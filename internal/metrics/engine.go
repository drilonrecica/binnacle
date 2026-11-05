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
	subscribers map[uint64]chan Snapshot
	live        map[uint64]chan LiveMessage
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
	return &Engine{maxEvents: maxEvents, subscribers: map[uint64]chan Snapshot{}, live: map[uint64]chan LiveMessage{}}
}

type LiveMessage struct {
	Snapshot *Snapshot
	Event    *Event
}
type Subscription struct {
	C      <-chan LiveMessage
	cancel func()
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
	for _, ch := range e.subscribers {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snapshot:
		default:
		}
	}
	for id, ch := range e.live {
		sendLive(ch, LiveMessage{Snapshot: snapshotPtr(clone(snapshot))})
		for _, event := range events {
			eventCopy := event
			if !sendLive(ch, LiveMessage{Event: &eventCopy}) {
				delete(e.live, id)
				close(ch)
				break
			}
		}
	}
}
func (e *Engine) Subscribe() *Subscription {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextSub++
	id := e.nextSub
	ch := make(chan LiveMessage, 33)
	e.live[id] = ch
	if e.snapshot.Sequence > 0 {
		sendLive(ch, LiveMessage{Snapshot: snapshotPtr(clone(e.snapshot))})
	}
	return &Subscription{C: ch, cancel: func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if c, ok := e.live[id]; ok {
			delete(e.live, id)
			close(c)
		}
	}}
}
func (e *Engine) PublishFilesystems(obs []FilesystemObservation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.filesystems = append([]FilesystemObservation(nil), obs...)
	e.snapshot.Filesystems = append([]FilesystemObservation(nil), obs...)
}
func (e *Engine) Snapshot() Snapshot { e.mu.RLock(); defer e.mu.RUnlock(); return clone(e.snapshot) }
func (e *Engine) SSEClients() int    { e.mu.RLock(); defer e.mu.RUnlock(); return len(e.live) }

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
func snapshotPtr(s Snapshot) *Snapshot { return &s }
func sendLive(ch chan LiveMessage, message LiveMessage) bool {
	if message.Snapshot != nil {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- message:
		default:
		}
		return true
	}
	select {
	case ch <- message:
		return true
	default:
		return false
	}
}
