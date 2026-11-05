// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// runUntilRunning starts Run in the background and waits for StateRunning.
func runUntilRunning(t *testing.T, a *Application, ctx context.Context) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for a.State() != StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("application did not reach running")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return result
}

// TestStartContextOutlivesSignal: the signal context must not cancel the
// components' work before their Stop runs, or persistence loses its queue
// and every component stops at once instead of in order (RUN-1).
func TestStartContextOutlivesSignal(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(name string) ComponentFuncs {
		var started context.Context
		return ComponentFuncs{
			Label: name,
			StartFunc: func(ctx context.Context) error {
				started = ctx
				return nil
			},
			StopFunc: func(context.Context) error {
				mu.Lock()
				defer mu.Unlock()
				state := "live"
				if started.Err() != nil {
					state = "cancelled"
				}
				order = append(order, name+":"+state)
				return nil
			},
		}
	}
	a := New(quietLogger(), record("store"), record("persistence"), record("http"))
	ctx, cancel := context.WithCancel(context.Background())
	result := runUntilRunning(t, a, ctx)
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "http:live persistence:live store:live"
	if got := strings.Join(order, " "); got != want {
		t.Fatalf("stop order and start-context state = %q, want %q", got, want)
	}
}

// TestStartContextCancelledDuringStartup: a signal while a component is
// still starting (a long migration) cancels its start context.
func TestStartContextCancelledDuringStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	slow := ComponentFuncs{Label: "migrate", StartFunc: func(start context.Context) error {
		cancel()
		select {
		case <-start.Done():
			return start.Err()
		case <-time.After(2 * time.Second):
			return errors.New("start context was not cancelled by the signal")
		}
	}}
	err := New(quietLogger(), slow).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want the start to end with context.Canceled", err)
	}
}

// TestShutdownBudgetFitsDockerGrace: Docker kills 10 s after SIGTERM.
func TestShutdownBudgetFitsDockerGrace(t *testing.T) {
	if ShutdownBudget >= 10*time.Second {
		t.Fatalf("ShutdownBudget = %s, want under Docker's 10 s stop grace", ShutdownBudget)
	}
	budgets := HTTPStopBudget + DefaultStopBudget
	if budgets >= ShutdownBudget {
		t.Fatalf("HTTP plus default budget %s leaves nothing for the rest of %s", budgets, ShutdownBudget)
	}
}

// TestStuckComponentIsAbandoned: a Stop that ignores its context is left
// after its budget and the components before it still stop.
func TestStuckComponentIsAbandoned(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	stoppedFirst := make(chan struct{})
	first := ComponentFuncs{Label: "store", StopFunc: func(context.Context) error { close(stoppedFirst); return nil }}
	stuck := ComponentFuncs{Label: "stuck", Budget: 50 * time.Millisecond, StopFunc: func(context.Context) error { <-release; return nil }}
	a := New(quietLogger(), first, stuck)
	ctx, cancel := context.WithCancel(context.Background())
	result := runUntilRunning(t, a, ctx)
	began := time.Now()
	cancel()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "stuck") {
			t.Fatalf("Run = %v, want the stuck component named", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown waited on a Stop that ignores its budget")
	}
	if elapsed := time.Since(began); elapsed > time.Second {
		t.Fatalf("shutdown took %s with a 50ms budget", elapsed)
	}
	select {
	case <-stoppedFirst:
	default:
		t.Fatal("the component before the stuck one was not stopped")
	}
}

// TestComponentBudgetCappedByOverall: no component gets more than what is
// left of the overall budget.
func TestComponentBudgetCappedByOverall(t *testing.T) {
	var got time.Duration
	long := ComponentFuncs{Label: "long", Budget: time.Hour, StopFunc: func(ctx context.Context) error {
		deadline, _ := ctx.Deadline()
		got = time.Until(deadline)
		return nil
	}}
	a := New(quietLogger(), long)
	a.budget = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	result := runUntilRunning(t, a, ctx)
	cancel()
	<-result
	if got <= 0 || got > 200*time.Millisecond {
		t.Fatalf("component deadline in %s, want at most the 200ms overall budget", got)
	}
}

// TestHTTPStopEndsStreams opens a stream that only ends with its request
// context, then stops the server. Server.Shutdown alone waits for it until
// the deadline; the cancelled base context ends it at once (RUN-1).
func TestHTTPStopEndsStreams(t *testing.T) {
	handlerDone := make(chan struct{})
	streaming := make(chan struct{})
	stream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(streaming)
		<-r.Context().Done()
	})
	h := NewHTTPServer("127.0.0.1:0", New(quietLogger()), Routes{API: stream})
	if err := h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	go func() {
		resp, err := http.Get("http://" + h.Addr().String() + "/api/v1/live")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-streaming:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), HTTPStopBudget)
	defer cancel()
	began := time.Now()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(began); elapsed > time.Second {
		t.Fatalf("Stop took %s; the open stream held shutdown", elapsed)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("stream handler kept running after Stop")
	}
}

func readyz(t *testing.T, h *HTTPServer) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /readyz: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, body
}

// TestReadyzReportsDatabaseAndCollectors (RUN-6).
func TestReadyzReportsDatabaseAndCollectors(t *testing.T) {
	database := "ok"
	application := New(quietLogger())
	h := NewHTTPServer("127.0.0.1:0", application, Routes{Ready: func(ctx context.Context) Readiness {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness probe has no deadline")
		}
		return Readiness{Database: database, Collectors: map[string]string{"docker": "degraded", "host": "healthy"}}
	}})
	if code, body := readyz(t, h); code != http.StatusServiceUnavailable || body["ready"] != false || body["state"] != string(StateStarting) {
		t.Fatalf("while starting: %d %v", code, body)
	}
	application.setState(StateRunning)
	code, body := readyz(t, h)
	if code != http.StatusOK || body["ready"] != true || body["database"] != "ok" {
		t.Fatalf("running with database ok: %d %v", code, body)
	}
	collectors, _ := body["collectors"].(map[string]any)
	if collectors["docker"] != "degraded" || collectors["host"] != "healthy" {
		t.Fatalf("collectors = %v, want their states; a degraded collector must not fail readiness", body["collectors"])
	}
	if _, ok := body["version"]; ok {
		t.Fatal("/readyz exposes the version")
	}
	database = "unavailable"
	if code, body := readyz(t, h); code != http.StatusServiceUnavailable || body["ready"] != false {
		t.Fatalf("database unavailable: %d %v", code, body)
	}
}
