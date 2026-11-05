// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

type sseFrame struct {
	kind string
	id   uint64
	data string
}

// openLive connects to the live stream and returns its frames. The stream
// closes with the test.
func openLive(t *testing.T, url string) (*http.Response, <-chan sseFrame) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/live", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan sseFrame, 1024)
	if response.StatusCode != http.StatusOK {
		close(frames)
		return response, frames
	}
	go func() {
		defer close(frames)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		var frame sseFrame
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				frame.kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				frame.id, _ = strconv.ParseUint(strings.TrimPrefix(line, "id: "), 10, 64)
			case strings.HasPrefix(line, "data: "):
				frame.data = strings.TrimPrefix(line, "data: ")
			case line == "" && frame.kind != "":
				frames <- frame
				frame = sseFrame{}
			}
		}
	}()
	return response, frames
}

func nextFrame(t *testing.T, frames <-chan sseFrame, kind string) sseFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("stream ended while waiting for %s", kind)
			}
			if frame.kind == kind {
				return frame
			}
		case <-deadline:
			t.Fatalf("no %s frame", kind)
		}
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

type countingDecorator struct{ calls atomic.Int64 }

func (d *countingDecorator) Decorate(_ context.Context, s metrics.Snapshot) metrics.Snapshot {
	d.calls.Add(1)
	s.Resources = append(s.Resources, metrics.ResourceSnapshot{ID: "decorated"})
	return s
}

func liveServer(t *testing.T, engine *metrics.Engine, opts LiveOptions, decorators ...SnapshotDecorator) (*Server, *httptest.Server) {
	t.Helper()
	server := New()
	server.EnableLive(engine, DemoAuthorizer(true), auth.NewProtection(64, auth.TrustedProxies{}), opts, decorators...)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return server, ts
}

// RUN-5: concurrent streams are capped; the next one gets a clear 503 with
// Retry-After, and a freed slot is usable again.
func TestLiveStreamCapsConcurrentClients(t *testing.T) {
	engine := metrics.NewEngine(8)
	server, ts := liveServer(t, engine, LiveOptions{MaxClients: 2})
	ctx, cancel := context.WithCancel(context.Background())
	first, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/live", nil)
	firstResponse, err := http.DefaultClient.Do(first)
	if err != nil {
		t.Fatal(err)
	}
	defer firstResponse.Body.Close()
	openLive(t, ts.URL)
	waitFor(t, "two clients", func() bool { return server.LiveClients() == 2 })

	refused, _ := openLive(t, ts.URL)
	if refused.StatusCode != http.StatusServiceUnavailable || refused.Header.Get("Retry-After") == "" {
		t.Fatalf("third stream: status %d, Retry-After %q", refused.StatusCode, refused.Header.Get("Retry-After"))
	}
	var body struct {
		Error Error `json:"error"`
	}
	if err := json.NewDecoder(refused.Body).Decode(&body); err != nil || body.Error.Code != "live_capacity" {
		t.Fatalf("third stream body %+v, %v", body, err)
	}
	refused.Body.Close()

	cancel()
	waitFor(t, "a freed slot", func() bool { return server.LiveClients() == 1 })
	if again, _ := openLive(t, ts.URL); again.StatusCode != http.StatusOK {
		t.Fatalf("stream after a slot freed: status %d", again.StatusCode)
	}
}

// RUN-5: each snapshot is decorated and encoded once, however many clients
// are connected.
func TestLiveSnapshotDecoratedOncePerSnapshot(t *testing.T) {
	engine := metrics.NewEngine(8)
	decorator := &countingDecorator{}
	server, ts := liveServer(t, engine, LiveOptions{Interval: time.Millisecond}, decorator)
	streams := make([]<-chan sseFrame, 4)
	for i := range streams {
		_, streams[i] = openLive(t, ts.URL)
	}
	waitFor(t, "four clients", func() bool { return server.LiveClients() == 4 })
	engine.Publish(metrics.Snapshot{})
	sequence := engine.Snapshot().Sequence
	for _, frames := range streams {
		frame := nextFrame(t, frames, "snapshot")
		var snapshot metrics.Snapshot
		if err := json.Unmarshal([]byte(frame.data), &snapshot); err != nil || snapshot.Sequence != sequence || len(snapshot.Resources) != 1 {
			t.Fatalf("snapshot frame %q (%v)", frame.data, err)
		}
	}
	if calls := decorator.calls.Load(); calls != 1 {
		t.Fatalf("decorated %d times for one snapshot and four clients", calls)
	}
}

// RUN-5: snapshots published faster than live.sse_interval are coalesced to
// the latest, while every event still arrives, in order, with frame ids that
// never go backwards.
func TestLiveHonoursIntervalWithoutDroppingEvents(t *testing.T) {
	engine := metrics.NewEngine(64)
	engine.Publish(metrics.Snapshot{})
	server, ts := liveServer(t, engine, LiveOptions{Interval: 300 * time.Millisecond})
	_, frames := openLive(t, ts.URL)
	nextFrame(t, frames, "snapshot")
	waitFor(t, "a client", func() bool { return server.LiveClients() == 1 })
	for range 10 {
		engine.Publish(metrics.Snapshot{}, metrics.Event{Type: "tick"})
	}
	latest := engine.Snapshot().Sequence
	events, snapshots := 0, 0
	var lastID uint64
	deadline := time.After(2 * time.Second)
	for events < 10 || snapshots < 1 {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatal("stream ended")
			}
			if frame.id < lastID {
				t.Fatalf("frame id %d after %d", frame.id, lastID)
			}
			lastID = frame.id
			switch frame.kind {
			case "event":
				events++
			case "snapshot":
				snapshots++
				var snapshot metrics.Snapshot
				_ = json.Unmarshal([]byte(frame.data), &snapshot)
				if snapshot.Sequence != latest {
					t.Fatalf("coalesced snapshot %d, want latest %d", snapshot.Sequence, latest)
				}
			}
		case <-deadline:
			t.Fatalf("received %d events and %d snapshots", events, snapshots)
		}
	}
	if events != 10 || snapshots != 1 {
		t.Fatalf("received %d events and %d snapshots, want 10 and 1", events, snapshots)
	}
	if lastID < uint64(latest) {
		t.Fatalf("resume cursor %d is behind the snapshot %d", lastID, latest)
	}
}

// A client's queue writes a snapshot after the events at or below its id,
// and closes instead of dropping events when it overflows.
func TestLiveClientQueue(t *testing.T) {
	c := &liveClient{notify: make(chan struct{}, 1)}
	c.pushEvent(liveFrame{kind: "event", id: 3})
	c.setSnapshot(liveFrame{kind: "snapshot", id: 4})
	c.pushEvent(liveFrame{kind: "event", id: 6})
	c.setSnapshot(liveFrame{kind: "snapshot", id: 5})
	frames, closed := c.take()
	if closed || len(frames) != 3 || frames[0].id != 3 || frames[1].kind != "snapshot" || frames[1].id != 5 || frames[2].id != 6 {
		t.Fatalf("frames %+v closed %v", frames, closed)
	}
	for i := range liveClientQueue + 1 {
		c.pushEvent(liveFrame{kind: "event", id: metrics.Sequence(10 + i)})
	}
	if _, closed := c.take(); !closed {
		t.Fatal("overflowing client not closed")
	}
}

// A Last-Event-ID from before a restart is above the new sequence; events
// published afterwards must still be sent, not skipped as already seen.
func TestLiveStaleLastEventIDDoesNotSkipEvents(t *testing.T) {
	engine := metrics.NewEngine(8)
	server, ts := liveServer(t, engine, LiveOptions{Interval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/live", nil)
	request.Header.Set("Last-Event-ID", "5000")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	waitFor(t, "a client", func() bool { return server.LiveClients() == 1 })
	engine.Publish(metrics.Snapshot{}, metrics.Event{Type: "after_restart"})
	scanner := bufio.NewScanner(response.Body)
	deadline := time.AfterFunc(3*time.Second, cancel)
	defer deadline.Stop()
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"type":"after_restart"`) {
			return
		}
	}
	t.Fatal("event after a restart was skipped")
}
