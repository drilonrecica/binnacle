// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// Live stream defaults (RUN-5).
const (
	DefaultSSEInterval   = 2 * time.Second
	DefaultSSEMaxClients = 8
	// liveClientQueue is how many event frames one client may fall behind
	// before its stream is closed so it resumes with Last-Event-ID.
	liveClientQueue = 256
	// liveDecorateTimeout bounds the alert and Coolify queries behind one
	// snapshot's decoration.
	liveDecorateTimeout = 5 * time.Second
)

// LiveOptions configures the live stream. Zero values take the defaults.
type LiveOptions struct {
	// Interval is the shortest gap between two snapshot frames
	// (live.sse_interval). Snapshots published faster are coalesced.
	Interval time.Duration
	// MaxClients caps concurrent live streams (live.max_clients).
	MaxClients int
	Logger     *slog.Logger
}

func (o LiveOptions) withDefaults() LiveOptions {
	if o.Interval <= 0 {
		o.Interval = DefaultSSEInterval
	}
	if o.MaxClients <= 0 {
		o.MaxClients = DefaultSSEMaxClients
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// liveFrame is one encoded SSE payload, shared by every client. The id is the
// resume cursor: every event with a sequence at or below it was fanned out
// before this frame.
type liveFrame struct {
	kind string
	id   metrics.Sequence
	data []byte
}

// liveHub holds the only engine subscription behind the live stream. It
// decorates and encodes each snapshot once, at most once per interval, and
// fans the bytes out to every client; each event is encoded once as well.
// The hub runs only while at least one client is connected.
type liveHub struct {
	engine     *metrics.Engine
	decorators []SnapshotDecorator
	opts       LiveOptions

	mu      sync.Mutex
	clients map[*liveClient]struct{}
	latest  *liveFrame
	gen     uint64
	cancel  context.CancelFunc
}

func newLiveHub(engine *metrics.Engine, opts LiveOptions, decorators []SnapshotDecorator) *liveHub {
	return &liveHub{engine: engine, decorators: decorators, opts: opts.withDefaults(), clients: map[*liveClient]struct{}{}}
}

// Clients is the number of connected live streams.
func (h *liveHub) Clients() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// register adds a client, starting the hub for the first one. It refuses the
// client when MaxClients streams are already open.
func (h *liveHub) register() (*liveClient, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= h.opts.MaxClients {
		return nil, false
	}
	c := &liveClient{notify: make(chan struct{}, 1)}
	h.clients[c] = struct{}{}
	if h.latest != nil {
		c.setSnapshot(*h.latest)
	}
	if h.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		h.gen++
		h.cancel = cancel
		// Subscribe before the client sees its stream open, so nothing
		// published after registration can be missed.
		go h.run(ctx, h.gen, h.engine.Subscribe())
	}
	return c, true
}

// unregister removes a client and stops the hub after the last one leaves.
func (h *liveHub) unregister(c *liveClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	if len(h.clients) == 0 {
		h.stopLocked()
	}
}

func (h *liveHub) stopLocked() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.latest = nil
}

func (h *liveHub) run(ctx context.Context, gen uint64, sub *metrics.Subscription) {
	defer sub.Close()
	var (
		pending   *metrics.Snapshot
		lastSent  time.Time
		lastEvent metrics.Sequence
		timer     *time.Timer
		timerC    <-chan time.Time
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	event := func(e metrics.Event) bool {
		data, err := json.Marshal(e)
		if err != nil {
			h.opts.Logger.Error("live event encoding failed", "error", err, "seq", e.Seq)
			return true
		}
		lastEvent = e.Seq
		return h.broadcast(gen, liveFrame{kind: "event", id: e.Seq, data: data})
	}
	// flush sends the pending snapshot after every event published before it,
	// so the frame id can be the resume cursor.
	flush := func() bool {
		for {
			select {
			case e, ok := <-sub.Events:
				if !ok {
					return false
				}
				if !event(e) {
					return false
				}
				continue
			default:
			}
			break
		}
		snapshot := *pending
		pending = nil
		lastSent = time.Now()
		decorateCtx, cancel := context.WithTimeout(ctx, liveDecorateTimeout)
		for _, decorator := range h.decorators {
			snapshot = decorator.Decorate(decorateCtx, snapshot)
		}
		cancel()
		data, err := json.Marshal(snapshot)
		if err != nil {
			h.opts.Logger.Error("live snapshot encoding failed", "error", err, "seq", snapshot.Sequence)
			return true
		}
		return h.broadcast(gen, liveFrame{kind: "snapshot", id: max(snapshot.Sequence, lastEvent), data: data})
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub.Events:
			if !ok || !event(e) {
				h.end(gen)
				return
			}
		case s, ok := <-sub.Snapshots:
			if !ok {
				h.end(gen)
				return
			}
			pending = &s
			if wait := h.opts.Interval - time.Since(lastSent); wait > 0 {
				if timerC == nil {
					timer = time.NewTimer(wait)
					timerC = timer.C
				}
				continue
			}
			if !flush() {
				h.end(gen)
				return
			}
		case <-timerC:
			timerC = nil
			if pending != nil && !flush() {
				h.end(gen)
				return
			}
		}
	}
}

// broadcast hands frame to every client. It reports false once this run is
// no longer the hub's current one.
func (h *liveHub) broadcast(gen uint64, frame liveFrame) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gen != gen || h.cancel == nil {
		return false
	}
	if frame.kind == "snapshot" {
		h.latest = &frame
	}
	for c := range h.clients {
		if frame.kind == "snapshot" {
			c.setSnapshot(frame)
		} else {
			c.pushEvent(frame)
		}
	}
	return true
}

// end runs when the engine closed the hub's subscription (it fell too far
// behind): every client stream closes and reconnects with Last-Event-ID.
func (h *liveHub) end(gen uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gen != gen {
		return
	}
	h.opts.Logger.Warn("live fan-out fell behind the engine; closing live streams so clients resume")
	for c := range h.clients {
		c.drop()
	}
	h.stopLocked()
}

// liveClient is one stream's queue: events in order, and the latest snapshot
// in its own slot, so coalescing snapshots never drops an event.
type liveClient struct {
	mu       sync.Mutex
	events   []liveFrame
	snapshot *liveFrame
	closed   bool
	notify   chan struct{}
}

func (c *liveClient) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *liveClient) setSnapshot(frame liveFrame) {
	c.mu.Lock()
	c.snapshot = &frame
	c.mu.Unlock()
	c.signal()
}

func (c *liveClient) pushEvent(frame liveFrame) {
	c.mu.Lock()
	if len(c.events) >= liveClientQueue {
		c.closed = true
		c.events = nil
	} else if !c.closed {
		c.events = append(c.events, frame)
	}
	c.mu.Unlock()
	c.signal()
}

func (c *liveClient) drop() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.signal()
}

// take returns the queued frames in write order and reports whether the
// stream must close. The snapshot goes after every event at or below its id.
func (c *liveClient) take() ([]liveFrame, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, true
	}
	events, snapshot := c.events, c.snapshot
	c.events, c.snapshot = nil, nil
	if snapshot == nil {
		return events, false
	}
	out := make([]liveFrame, 0, len(events)+1)
	placed := false
	for _, e := range events {
		if !placed && e.id > snapshot.id {
			out = append(out, *snapshot)
			placed = true
		}
		out = append(out, e)
	}
	if !placed {
		out = append(out, *snapshot)
	}
	return out, false
}
