// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/observe"
)

// maxCarriedEvents bounds the events of lost batches kept for the next batch;
// older ones beyond it are counted in EventsLost.
const maxCarriedEvents = 128

// Reasons a batch was not stored, listed in a persistence_gap event.
const (
	gapQueueFull      = "queue_full"
	gapEmergencyPause = "emergency_pause"
	gapWriteFailed    = "write_failed"
)

// Persistence closes one sample window every Interval and schedules it into a
// bounded writer queue. A window is the aggregate of every tick the engine
// published since the previous one (COL-9, see metrics.Engine.PersistenceBatch).
// Storage failure never blocks collectors or the live Metrics Engine.
//
// A batch that is not stored (queue overflow, emergency pause, failed write)
// is not silent (EVT-6): its events wait for the next batch, and the next
// batch also carries one persistence_gap event spanning every consecutive
// lost batch, which the history API reports as a persistence_failure gap.
type Persistence struct {
	Engine         *metrics.Engine
	Store          *Manager
	Interval       time.Duration
	QueueLimit     int
	Dropped        atomic.Uint64
	EventsLost     atomic.Uint64
	QueueDepth     atomic.Int64
	LastWriteNanos atomic.Int64
	// WriteFailures counts batches whose write failed after its retries, and
	// writeLatency the duration of the latest writes, retries included (RUN-7).
	WriteFailures observe.Counter
	writeLatency  observe.Latency
	Paused        atomic.Bool
	cancel        context.CancelFunc
	// cancelWrites aborts the write in flight; Stop calls it at its deadline.
	cancelWrites context.CancelFunc
	// stopping is closed by Stop once no more batches are enqueued; the
	// writer then drains what is queued and returns.
	stopping chan struct{}
	tickWG   sync.WaitGroup
	writerWG sync.WaitGroup
	mu       sync.Mutex
	carry    carried
}

// PersistenceStopBudget is how long Stop may spend draining the queue before
// the database closes (RUN-1). Batches still queued at the deadline are
// counted as dropped.
const PersistenceStopBudget = 3 * time.Second

// queued is one batch in the writer queue; gapID is the id of the
// persistence_gap event it carries, "" for none.
type queued struct {
	batch metrics.PersistenceBatch
	gap   *batchGap
	gapID string
}

// carried is what lost batches leave for the next accepted one.
type carried struct {
	events []metrics.Event
	gap    *batchGap
}

// batchGap spans consecutive lost batches by their window end times.
type batchGap struct {
	From, To time.Time
	Batches  int
	Reasons  []string
}

func (g *batchGap) add(at time.Time, batches int, reasons ...string) {
	if g.Batches == 0 || at.Before(g.From) {
		g.From = at
	}
	if at.After(g.To) {
		g.To = at
	}
	g.Batches += batches
	for _, reason := range reasons {
		if !slices.Contains(g.Reasons, reason) {
			g.Reasons = append(g.Reasons, reason)
		}
	}
}

// event is the persistence_gap event of g, dated at its first lost window so
// it falls inside the gap the history API finds there. Its details are empty
// when they cannot be encoded (a time beyond year 9999); the event is kept.
func (g *batchGap) event() (metrics.Event, error) {
	details, err := json.Marshal(map[string]any{"from": g.From.UTC(), "to": g.To.UTC(), "batches": g.Batches, "reasons": g.Reasons})
	if err != nil {
		details = []byte("{}")
		err = fmt.Errorf("encode persistence gap details: %w", err)
	}
	return metrics.Event{
		ID:       metrics.NewEventID(),
		At:       g.From.UTC(),
		Type:     "persistence_gap",
		Severity: "warning",
		Message:  fmt.Sprintf("History was not stored for %d persistence window(s)", g.Batches),
		Details:  string(details),
	}, err
}

func NewPersistence(engine *metrics.Engine, store *Manager, interval time.Duration, limit int) *Persistence {
	return &Persistence{Engine: engine, Store: store, Interval: interval, QueueLimit: limit}
}

// take prepares an accepted batch: the events of lost batches go first, then
// its own, then the gap event of the lost batches.
func (p *Persistence) take(batch metrics.PersistenceBatch) queued {
	p.mu.Lock()
	defer p.mu.Unlock()
	item := queued{batch: batch}
	if len(p.carry.events) > 0 {
		item.batch.Events = append(p.carry.events, batch.Events...)
	}
	if p.carry.gap != nil {
		event, err := p.carry.gap.event()
		if err != nil && p.Store != nil {
			p.Store.errorLog().Warn("persistence gap event details were not encoded", "error", err)
		}
		item.gap, item.gapID = p.carry.gap, event.ID
		item.batch.Events = append(item.batch.Events, event)
	}
	p.carry = carried{}
	return item
}

// lose records a batch that will not be stored: its window joins the pending
// gap, together with the gap it was carrying, and its events wait for the
// next batch up to maxCarriedEvents.
func (p *Persistence) lose(item queued, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.carry.gap == nil {
		p.carry.gap = &batchGap{}
	}
	p.carry.gap.add(item.batch.Snapshot.At, 1, reason)
	if item.gap != nil {
		p.carry.gap.add(item.gap.From, item.gap.Batches, item.gap.Reasons...)
		p.carry.gap.add(item.gap.To, 0)
	}
	for _, event := range item.batch.Events {
		if event.ID != item.gapID {
			p.carry.events = append(p.carry.events, event)
		}
	}
	if excess := len(p.carry.events) - maxCarriedEvents; excess > 0 {
		p.EventsLost.Add(uint64(excess))
		p.carry.events = append([]metrics.Event(nil), p.carry.events[excess:]...)
	}
}

func (p *Persistence) Start(parent context.Context) error {
	if p.Interval <= 0 {
		p.Interval = 10 * time.Second
	}
	if p.QueueLimit < 1 {
		p.QueueLimit = 60
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	// Writes are not cancelled with the ticker: Stop drains the queue on this
	// context and cancels it only when its own deadline passes.
	writeCtx, cancelWrites := context.WithCancel(context.WithoutCancel(parent))
	p.cancelWrites = cancelWrites
	p.stopping = make(chan struct{})
	queue := make(chan queued, p.QueueLimit)
	enqueue := func() {
		batch := p.Engine.PersistenceBatch()
		if batch.Snapshot.Sequence == 0 {
			return
		}
		if p.Store != nil && p.Store.EmergencyPause() {
			p.Paused.Store(true)
			p.Dropped.Add(1)
			p.lose(queued{batch: batch}, gapEmergencyPause)
			return
		}
		p.Paused.Store(false)
		// This goroutine is the only sender: after dropping the oldest batch
		// there is room for the newest one.
		if len(queue) == cap(queue) {
			select {
			case old := <-queue:
				p.Dropped.Add(1)
				p.lose(old, gapQueueFull)
			default:
			}
		}
		item := p.take(batch)
		select {
		case queue <- item:
		default:
			p.Dropped.Add(1)
			p.lose(item, gapQueueFull)
		}
		p.QueueDepth.Store(int64(len(queue)))
	}
	p.tickWG.Add(1)
	go func() {
		defer p.tickWG.Done()
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		enqueue()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				enqueue()
			}
		}
	}()
	p.writerWG.Add(1)
	go func() {
		defer p.writerWG.Done()
		for {
			select {
			case <-p.stopping:
				p.drain(writeCtx, queue)
				return
			case item := <-queue:
				p.QueueDepth.Store(int64(len(queue)))
				p.write(writeCtx, item)
			}
		}
	}()
	return nil
}

// write stores one batch; a failure keeps its events and gap for the next.
func (p *Persistence) write(ctx context.Context, item queued) {
	started := time.Now()
	err := Retry(ctx, func() error { return p.Store.WriteBatch(ctx, item.batch) })
	elapsed := time.Since(started)
	p.LastWriteNanos.Store(elapsed.Nanoseconds())
	p.writeLatency.Observe(elapsed)
	if err == nil {
		return
	}
	p.Dropped.Add(1)
	p.lose(item, gapWriteFailed)
	if ctx.Err() == nil {
		p.WriteFailures.Inc()
		p.Store.errorLog().Error("persistence batch was not stored", "at", item.batch.Snapshot.At, "events", len(item.batch.Events), "error", err)
	}
}

// drain writes every queued batch until the queue is empty or ctx is done.
// Batches left at the deadline are counted as dropped and logged once.
func (p *Persistence) drain(ctx context.Context, queue chan queued) {
	stored := 0
	for {
		select {
		case item := <-queue:
			p.QueueDepth.Store(int64(len(queue)))
			if ctx.Err() != nil {
				p.Dropped.Add(1)
				p.lose(item, gapWriteFailed)
				continue
			}
			p.write(ctx, item)
			stored++
		default:
			if ctx.Err() != nil {
				p.Store.logger().Error("persistence queue not drained before the shutdown deadline", "dropped_total", p.Dropped.Load())
			} else if stored > 0 {
				p.Store.logger().Info("persistence queue drained", "batches", stored)
			}
			return
		}
	}
}

func (p *Persistence) Queue() int64                { return p.QueueDepth.Load() }
func (p *Persistence) DroppedCount() uint64        { return p.Dropped.Load() }
func (p *Persistence) EventsLostCount() uint64     { return p.EventsLost.Load() }
func (p *Persistence) WriteLatency() time.Duration { return time.Duration(p.LastWriteNanos.Load()) }

// WriteFailureCount is the number of batches whose write failed since start.
func (p *Persistence) WriteFailureCount() uint64 { return p.WriteFailures.Load() }

// WriteLatencySummary is the p50, p95 and max of the latest batch writes.
func (p *Persistence) WriteLatencySummary() observe.LatencySummary { return p.writeLatency.Summary() }
func (p *Persistence) Name() string                                { return "persistence" }
func (p *Persistence) StopBudget() time.Duration                   { return PersistenceStopBudget }

// Stop stops closing windows, then drains the queue within ctx before it
// returns, so the database is closed only after the last queued batch was
// written or the deadline passed (RUN-1).
func (p *Persistence) Stop(ctx context.Context) error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	p.tickWG.Wait()
	close(p.stopping)
	release := context.AfterFunc(ctx, p.cancelWrites)
	defer release()
	p.writerWG.Wait()
	p.cancelWrites()
	p.cancel = nil
	if ctx.Err() != nil {
		return fmt.Errorf("persistence drain: %w", ctx.Err())
	}
	return nil
}
