// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

// ADR 021 §9: live event frames carry the per-process sequence as their SSE
// id, so Last-Event-ID resumes from it, while the payload carries the ULID.
func TestLiveEventsResumeFromSequence(t *testing.T) {
	engine := metrics.NewEngine(8)
	engine.Publish(metrics.Snapshot{}, metrics.Event{Type: "first", Message: "one"}, metrics.Event{Type: "second", Message: "two"})
	events := engine.EventsAfter(0)
	server := New()
	server.EnableLive(engine, DemoAuthorizer(true), auth.NewProtection(16, auth.TrustedProxies{}), LiveOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/live", nil).WithContext(ctx)
	request.Header.Set("Last-Event-ID", strconv.FormatUint(uint64(events[0].Seq), 10))
	recorder, finished := serveStream(t, server.Handler(), request)
	if !finished(2 * time.Second) {
		t.Fatal("stream did not end with its context")
	}
	body := recorder.Body.String()
	want := "event: event\nid: " + strconv.FormatUint(uint64(events[1].Seq), 10) + "\ndata: "
	if !strings.Contains(body, want) || !strings.Contains(body, `"id":"`+events[1].ID+`"`) {
		t.Fatalf("resumed stream lacks the second event frame:\n%s", body)
	}
	if strings.Contains(body, `"type":"first"`) {
		t.Fatalf("resumed stream replayed an event before Last-Event-ID:\n%s", body)
	}
}

// resumeBody opens a stream on an engine holding three events and returns what
// it wrote before its context ended.
func resumeBody(t *testing.T, target string, header string) (string, []metrics.Event) {
	t.Helper()
	engine := metrics.NewEngine(8)
	engine.Publish(metrics.Snapshot{}, metrics.Event{Type: "first"}, metrics.Event{Type: "second"}, metrics.Event{Type: "third"})
	events := engine.EventsAfter(0)
	server := New()
	server.EnableLive(engine, DemoAuthorizer(true), auth.NewProtection(16, auth.TrustedProxies{}), LiveOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test"+strings.ReplaceAll(target, "{first}", strconv.FormatUint(uint64(events[0].Seq), 10)), nil).WithContext(ctx)
	if header != "" {
		request.Header.Set("Last-Event-ID", strings.ReplaceAll(header, "{second}", strconv.FormatUint(uint64(events[1].Seq), 10)))
	}
	recorder, finished := serveStream(t, server.Handler(), request)
	if !finished(2 * time.Second) {
		t.Fatal("stream did not end with its context")
	}
	return recorder.Body.String(), events
}

// UI-2: a client that opens a new EventSource cannot set headers, so it
// resumes with ?lastEventId= and the server replays from it.
func TestLiveResumesFromLastEventIDQuery(t *testing.T) {
	body, _ := resumeBody(t, "/api/v1/live?lastEventId={first}", "")
	if strings.Contains(body, `"type":"first"`) {
		t.Fatalf("query resume replayed an event at or before its cursor:\n%s", body)
	}
	if !strings.Contains(body, `"type":"second"`) || !strings.Contains(body, `"type":"third"`) {
		t.Fatalf("query resume did not replay the missed events:\n%s", body)
	}
}

// A browser's native retry reuses the original URL but sends the newer id in
// the header; the header must win.
func TestLiveResumeHeaderWinsOverQuery(t *testing.T) {
	body, _ := resumeBody(t, "/api/v1/live?lastEventId={first}", "{second}")
	if strings.Contains(body, `"type":"second"`) {
		t.Fatalf("stream resumed from the stale query instead of the header:\n%s", body)
	}
	if !strings.Contains(body, `"type":"third"`) {
		t.Fatalf("stream did not replay after the header id:\n%s", body)
	}
}

// Without a cursor nothing is replayed, and a malformed one is ignored.
func TestLiveWithoutCursorReplaysNothing(t *testing.T) {
	for _, target := range []string{"/api/v1/live", "/api/v1/live?lastEventId=abc"} {
		body, _ := resumeBody(t, target, "")
		if strings.Contains(body, "event: event") {
			t.Fatalf("%s replayed events without a valid cursor:\n%s", target, body)
		}
	}
}
