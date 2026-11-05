// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"sync"
	"testing"
)

// RUN-5: a newer snapshot replaces an unread one, but every queued event
// stays queued. The old shared channel drained one message, often an event,
// to make room for each snapshot.
func TestSnapshotsCoalesceWithoutDroppingEvents(t *testing.T) {
	engine := NewEngine(64)
	sub := engine.Subscribe()
	defer sub.Close()
	for range 10 {
		engine.Publish(Snapshot{}, Event{Type: "tick"})
	}
	if n := len(sub.Snapshots); n != 1 {
		t.Fatalf("%d snapshots queued, want the latest only", n)
	}
	if latest := <-sub.Snapshots; latest.Sequence != engine.Snapshot().Sequence {
		t.Fatalf("queued snapshot %d, latest is %d", latest.Sequence, engine.Snapshot().Sequence)
	}
	var previous Sequence
	for range 10 {
		event := <-sub.Events
		if event.Seq <= previous {
			t.Fatalf("event %d after %d", event.Seq, previous)
		}
		previous = event.Seq
	}
	if n := len(sub.Events); n != 0 {
		t.Fatalf("%d unexpected events", n)
	}
}

// A subscriber that falls more than LiveEventBuffer events behind is closed
// rather than silently skipping events, so it can resume from the ring.
func TestSlowSubscriberIsClosed(t *testing.T) {
	engine := NewEngine(8)
	sub := engine.Subscribe()
	defer sub.Close()
	for range LiveEventBuffer + 1 {
		engine.Publish(Snapshot{}, Event{Type: "tick"})
	}
	if engine.Subscribers() != 0 {
		t.Fatalf("slow subscriber still registered")
	}
	received := 0
	for range sub.Events {
		received++
	}
	if received != LiveEventBuffer {
		t.Fatalf("received %d buffered events, want %d", received, LiveEventBuffer)
	}
	if _, ok := <-sub.Snapshots; ok {
		if _, ok := <-sub.Snapshots; ok {
			t.Fatal("snapshot channel left open")
		}
	}
}

// Publishers, subscribers and closes race; run under -race. Every reader sees
// events in strictly increasing order with no gap until its channel closes,
// and every event below a snapshot's sequence arrives before that snapshot.
func TestEnginePublishSubscribeRace(t *testing.T) {
	engine := NewEngine(32)
	var publishers, readers sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for range 200 {
				engine.Publish(Snapshot{}, Event{Type: "a"}, Event{Type: "b"})
			}
		}()
	}
	errs := make(chan string, 64)
	for reader := range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				sub := engine.Subscribe()
				var last Sequence
				for received := 0; received < 50+reader*10; {
					select {
					case event, ok := <-sub.Events:
						if !ok {
							received = 1 << 30
							continue
						}
						if last != 0 && event.Seq <= last {
							errs <- "event order regressed"
						}
						last = event.Seq
						received++
					case snapshot, ok := <-sub.Snapshots:
						if !ok {
							received = 1 << 30
							continue
						}
						// Drain what is already queued: it must hold every
						// event below this snapshot.
						for drained := false; !drained; {
							select {
							case event, ok := <-sub.Events:
								if !ok {
									drained = true
									continue
								}
								last = event.Seq
							default:
								drained = true
							}
						}
						// Each Publish is a snapshot followed by its two events,
						// so the event just before this snapshot is Sequence-1.
						if last != 0 && last < snapshot.Sequence-1 {
							errs <- "snapshot overtook its earlier events"
						}
					case <-stop:
						sub.Close()
						return
					}
				}
				sub.Close()
			}
		}()
	}
	publishers.Wait()
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if engine.Subscribers() != 0 {
		t.Fatalf("%d subscriptions leaked", engine.Subscribers())
	}
}
