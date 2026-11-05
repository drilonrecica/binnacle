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
