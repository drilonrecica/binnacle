// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestULIDEncodingAndMonotonicity(t *testing.T) {
	var g ulidGenerator
	at := time.UnixMilli(1_700_000_000_000)
	first := g.next(at)
	if len(first) != 26 || strings.Trim(first, crockford) != "" {
		t.Fatalf("id %q is not 26 Crockford characters", first)
	}
	// The first ten characters are the millisecond timestamp.
	if want := encodeULID(uint64(at.UnixMilli()), [10]byte{})[:10]; first[:10] != want {
		t.Fatalf("timestamp prefix %q, want %q", first[:10], want)
	}
	if got := encodeULID(1<<48-1, [10]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}); got != "7ZZZZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Fatalf("max ULID = %q", got)
	}
	previous := first
	// Same millisecond and a clock step back still sort strictly after.
	for _, now := range []time.Time{at, at, at.Add(-time.Second), at.Add(time.Millisecond)} {
		id := g.next(now)
		if id <= previous {
			t.Fatalf("id %q does not sort after %q", id, previous)
		}
		previous = id
	}
}

func TestIncrementOverflow(t *testing.T) {
	b := []byte{0xff, 0xff}
	if increment(b) || b[0] != 0 || b[1] != 0 {
		t.Fatalf("overflow not reported: %v", b)
	}
	b = []byte{0x00, 0xff}
	if !increment(b) || b[0] != 1 || b[1] != 0 {
		t.Fatalf("carry wrong: %v", b)
	}
}

// EVT-1: ids must not depend on the process's message sequence, which a new
// engine (a restart) starts again at 1.
func TestEventIDsAreUniqueAcrossEngines(t *testing.T) {
	seen := map[string]bool{}
	for range 2 {
		engine := NewEngine(8)
		engine.Publish(Snapshot{}, Event{Type: "container_restart"}, Event{Type: "container_oom"})
		for _, event := range engine.PersistenceBatch().Events {
			if event.ID == "" || seen[event.ID] {
				t.Fatalf("event id %q empty or reused", event.ID)
			}
			seen[event.ID] = true
		}
	}
}

func TestEngineAssignsSequenceAndKeepsGivenIDs(t *testing.T) {
	engine := NewEngine(8)
	sub := engine.Subscribe()
	defer sub.Close()
	engine.Publish(Snapshot{}, Event{ID: "given", Type: "a"}, Event{Type: "b"})
	live := []Event{<-sub.Events, <-sub.Events}
	if live[0].ID != "given" || live[1].ID == "" || live[0].Seq == 0 || live[1].Seq <= live[0].Seq {
		t.Fatalf("live events=%+v", live)
	}
	if after := engine.EventsAfter(live[0].Seq); len(after) != 1 || after[0].ID != live[1].ID {
		t.Fatalf("EventsAfter(%d)=%+v", live[0].Seq, after)
	}
}

// EVT-6: each persistence batch carries only the events published since the
// previous one, and events evicted before any batch took them are counted.
func TestPersistenceBatchCarriesOnlyNewEvents(t *testing.T) {
	engine := NewEngine(3)
	engine.Publish(Snapshot{}, Event{Type: "one"}, Event{Type: "two"})
	if got := engine.PersistenceBatch().Events; len(got) != 2 {
		t.Fatalf("first batch=%+v", got)
	}
	if got := engine.PersistenceBatch().Events; len(got) != 0 {
		t.Fatalf("second batch resent %+v", got)
	}
	engine.Publish(Snapshot{}, Event{Type: "three"})
	got := engine.PersistenceBatch().Events
	if len(got) != 1 || got[0].Type != "three" {
		t.Fatalf("third batch=%+v", got)
	}
	if engine.EventsDropped() != 0 {
		t.Fatalf("events already persisted counted as dropped: %d", engine.EventsDropped())
	}
	engine.Publish(Snapshot{}, Event{Type: "four"}, Event{Type: "five"}, Event{Type: "six"}, Event{Type: "seven"})
	if got := engine.EventsDropped(); got != 1 {
		t.Fatalf("EventsDropped=%d, want 1 (four left the ring unpersisted)", got)
	}
	got = engine.PersistenceBatch().Events
	if len(got) != 3 || got[0].Type != "five" {
		t.Fatalf("batch after eviction=%+v", got)
	}
}
