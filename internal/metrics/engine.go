// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"sync"
	"time"
)

type Engine struct {
	mu          sync.RWMutex
	snapshot    Snapshot
	events      []Event
	filesystems []FilesystemObservation
	maxEvents   int
	live        map[uint64]*liveSub
	nextSub     uint64
	nextMessage Sequence
	// persistedSeq is the high-water mark of events handed to persistence:
	// each PersistenceBatch carries only events after it (EVT-6).
	persistedSeq Sequence
	// eventsDropped counts events that left the ring before persistence
	// took them.
	eventsDropped uint64
	// window accumulates the ticks published since the last PersistenceBatch.
	window window
	// trends holds the latest time-to-full estimates (SPEC §P3-12), set by
	// the estimator every few minutes and attached to every snapshot.
	trends trendSet
}

// trendSet is one estimator run: the memory estimate and one per mount key.
type trendSet struct {
	set         bool
	windowHours int
	memory      Trend
	filesystems map[string]Trend
}

// filesystem returns the estimate of mountKey, unknown when the estimator has
// none for it (a new mount, or before the first run).
func (t trendSet) filesystem(mountKey string) Trend {
	if v, ok := t.filesystems[mountKey]; ok {
		return v
	}
	return UnknownTrend(t.window())
}

func (t trendSet) memoryTrend() Trend {
	if t.set {
		return t.memory
	}
	return UnknownTrend(t.window())
}

func (t trendSet) window() int {
	if t.windowHours > 0 {
		return t.windowHours
	}
	return DefaultTrendWindowHours
}

// withTrends returns a copy of obs carrying the current estimates.
func (t trendSet) withTrends(obs []FilesystemObservation) []FilesystemObservation {
	out := append([]FilesystemObservation(nil), obs...)
	for i := range out {
		out[i].Trend = t.filesystem(out[i].MountKey)
	}
	return out
}

// SetTrends replaces the time-to-full estimates attached to snapshots. The
// estimator calls it after each run; filesystems is keyed by mount key.
func (e *Engine) SetTrends(windowHours int, memory Trend, filesystems map[string]Trend) {
	e.mu.Lock()
	defer e.mu.Unlock()
	copied := make(map[string]Trend, len(filesystems))
	for k, v := range filesystems {
		copied[k] = v
	}
	e.trends = trendSet{set: true, windowHours: windowHours, memory: memory, filesystems: copied}
	e.filesystems = e.trends.withTrends(e.filesystems)
	e.snapshot.Filesystems = append([]FilesystemObservation(nil), e.filesystems...)
	e.snapshot.Host.MemoryTrend = e.trends.memoryTrend()
}

// PersistenceBatch returns the persistence window ending now and starts the
// next one: the aggregate of every tick published since the previous call
// (COL-9), so history and rollups see bursts between two batches.
func (e *Engine) PersistenceBatch() PersistenceBatch {
	e.mu.Lock()
	defer e.mu.Unlock()
	batch := e.window.drain(clone(e.snapshot))
	for _, event := range e.events {
		if event.Seq > e.persistedSeq {
			batch.Events = append(batch.Events, event)
		}
	}
	if n := len(e.events); n > 0 {
		e.persistedSeq = max(e.persistedSeq, e.events[n-1].Seq)
	}
	batch.Filesystems = append([]FilesystemObservation(nil), e.filesystems...)
	return batch
}

func NewEngine(maxEvents int) *Engine {
	if maxEvents < 1 {
		maxEvents = 128
	}
	return &Engine{maxEvents: maxEvents, live: map[uint64]*liveSub{}}
}

// LiveEventBuffer is how many published events a subscriber may fall behind
// before the engine closes its subscription.
const LiveEventBuffer = 256

// Subscription receives the engine's snapshots and events (RUN-5).
//
// Snapshots and events travel in separate channels so that coalescing
// snapshots never discards an event: Snapshots holds at most the latest
// snapshot, a newer one replacing an unread one, while Events carries every
// event in sequence order. A subscriber more than LiveEventBuffer events
// behind is closed instead (both channels), so it can resume from the ring
// with EventsAfter rather than silently losing events.
//
// Every event with a sequence below a snapshot's is sent to Events before
// that snapshot reaches Snapshots.
type Subscription struct {
	Snapshots <-chan Snapshot
	Events    <-chan Event
	cancel    func()
}

type liveSub struct {
	snapshots chan Snapshot
	events    chan Event
}

func (s *liveSub) close() {
	close(s.snapshots)
	close(s.events)
}

func (s *Subscription) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}
func (e *Engine) Start(context.Context) error { return nil }
func (e *Engine) Stop(context.Context) error  { return nil }
func (e *Engine) Publish(snapshot Snapshot, events ...Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextMessage++
	snapshot.Sequence = e.nextMessage
	if snapshot.At.IsZero() {
		snapshot.At = time.Now().UTC()
	}
	snapshot.At = snapshot.At.UTC()
	snapshot.Host.MemoryTrend = e.trends.memoryTrend()
	snapshot.Resources = append([]ResourceSnapshot(nil), snapshot.Resources...)
	snapshot.Collectors = copyCollectors(snapshot.Collectors)
	snapshot.Filesystems = append([]FilesystemObservation(nil), e.filesystems...)
	e.snapshot = snapshot
	e.window.observe(snapshot)
	events = append([]Event(nil), events...)
	for index := range events {
		e.nextMessage++
		events[index].Seq = e.nextMessage
		if events[index].ID == "" {
			events[index].ID = NewEventID()
		}
		events[index].At = events[index].At.UTC()
		e.events = append(e.events, events[index])
	}
	if excess := len(e.events) - e.maxEvents; excess > 0 {
		for _, event := range e.events[:excess] {
			if event.Seq > e.persistedSeq {
				e.eventsDropped++
			}
		}
		e.events = append([]Event(nil), e.events[excess:]...)
	}
	for id, sub := range e.live {
		replaceSnapshot(sub.snapshots, clone(snapshot))
		for _, event := range events {
			select {
			case sub.events <- event:
				continue
			default:
			}
			delete(e.live, id)
			sub.close()
			break
		}
	}
}
func (e *Engine) Subscribe() *Subscription {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextSub++
	id := e.nextSub
	sub := &liveSub{snapshots: make(chan Snapshot, 1), events: make(chan Event, LiveEventBuffer)}
	e.live[id] = sub
	if e.snapshot.Sequence > 0 {
		replaceSnapshot(sub.snapshots, clone(e.snapshot))
	}
	return &Subscription{Snapshots: sub.snapshots, Events: sub.events, cancel: func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if s, ok := e.live[id]; ok {
			delete(e.live, id)
			s.close()
		}
	}}
}
func (e *Engine) PublishFilesystems(obs []FilesystemObservation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.filesystems = e.trends.withTrends(obs)
	e.snapshot.Filesystems = append([]FilesystemObservation(nil), e.filesystems...)
}
func (e *Engine) Snapshot() Snapshot { e.mu.RLock(); defer e.mu.RUnlock(); return clone(e.snapshot) }

// Subscribers is the number of open subscriptions.
func (e *Engine) Subscribers() int { e.mu.RLock(); defer e.mu.RUnlock(); return len(e.live) }

// EventsDropped is the number of events evicted from the ring before a
// persistence batch took them; they reached the live stream but not history.
func (e *Engine) EventsDropped() uint64 { e.mu.RLock(); defer e.mu.RUnlock(); return e.eventsDropped }

// EventsAfter returns the ring's events with a sequence after seq, for
// Last-Event-ID resumption.
func (e *Engine) EventsAfter(seq Sequence) []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []Event{}
	for _, v := range e.events {
		if v.Seq > seq {
			out = append(out, v)
		}
	}
	return out
}
func clone(s Snapshot) Snapshot {
	s.Resources = append([]ResourceSnapshot(nil), s.Resources...)
	for index := range s.Resources {
		s.Resources[index].Components = append([]ResourceComponent(nil), s.Resources[index].Components...)
	}
	s.Collectors = copyCollectors(s.Collectors)
	s.Filesystems = append([]FilesystemObservation(nil), s.Filesystems...)
	return s
}
func copyCollectors(in map[string]CollectorHealth) map[string]CollectorHealth {
	out := map[string]CollectorHealth{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// replaceSnapshot puts s in the one-slot channel ch, discarding an unread
// older snapshot. Only the engine sends on ch, under e.mu, so the second
// send cannot block.
func replaceSnapshot(ch chan Snapshot, s Snapshot) {
	select {
	case <-ch:
	default:
	}
	ch <- s
}
