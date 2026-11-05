// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

func jsonLoggedServer(buf io.Writer) *Server {
	s := New()
	s.SetLogger(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return s
}

func requestLines(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if entry["msg"] == "http request" {
			out = append(out, entry)
		}
	}
	return out
}

// Each request is logged exactly once, through the configured logger, with the
// client network prefix rather than the address (SEC-21).
func TestRequestLoggedOnceWithClientPrefix(t *testing.T) {
	var buf syncBuffer
	s := jsonLoggedServer(&buf)
	s.Handle("/api/v1/ping", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping?token=secret", nil)
	req.RemoteAddr = "192.0.2.55:41234"
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)

	lines := requestLines(t, buf.String())
	if len(lines) != 1 {
		t.Fatalf("logged %d request lines, want 1:\n%s", len(lines), buf.String())
	}
	if got := lines[0]["client"]; got != "192.0.2.0/24" {
		t.Errorf("client = %v, want 192.0.2.0/24", got)
	}
	if got := lines[0]["status"]; got != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want 418", got)
	}
	if strings.Contains(buf.String(), "192.0.2.55") || strings.Contains(buf.String(), "secret") {
		t.Errorf("log leaks the client address or a token: %s", buf.String())
	}
}

func TestIPv6ClientPrefix(t *testing.T) {
	var buf syncBuffer
	s := jsonLoggedServer(&buf)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	req.RemoteAddr = "[2001:db8:1234:5678::9]:443"
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	lines := requestLines(t, buf.String())
	if len(lines) != 1 || lines[0]["client"] != "2001:db8:1234:5600::/56" {
		t.Fatalf("unexpected request log: %s", buf.String())
	}
}

func TestNonStreamingRoutesGetDeadline(t *testing.T) {
	s := New()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	seen := map[string]bool{}
	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Context().Deadline()
		seen[r.URL.RequestURI()] = ok
	})
	s.Handle("/api/v1/session", record)
	s.Handle("/api/v1/live", record)
	s.Handle("/api/v1/logs", record)
	for uri, want := range map[string]bool{
		"/api/v1/session":          true,
		"/api/v1/logs":             true,
		"/api/v1/logs?follow=true": false,
		"/api/v1/live":             false,
	} {
		s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, uri, nil))
		if seen[uri] != want {
			t.Errorf("%s: deadline set = %v, want %v", uri, seen[uri], want)
		}
	}
}

// Over a real connection the write deadline cuts off a handler that overruns
// its budget, while a streaming route keeps writing past it.
func TestWriteDeadlineOnRealConnection(t *testing.T) {
	s := New()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if testing.Short() {
		t.Skip("slow deadline test")
	}
	s.requestTimeout = 50 * time.Millisecond
	s.writeGrace = 50 * time.Millisecond
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(s.requestTimeout + s.writeGrace + 200*time.Millisecond)
		_, _ = io.WriteString(w, "late")
	})
	s.Handle("/api/v1/slow", slow)
	s.Handle("/api/v1/live", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "stream")
	}))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/v1/slow")
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) == "late" {
			t.Fatal("handler wrote after its write deadline")
		}
	}
	resp, err = srv.Client().Get(srv.URL + "/api/v1/live")
	if err != nil {
		t.Fatalf("streaming route failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "stream" {
		t.Fatalf("streaming route body = %q", body)
	}
}

func TestPanicRecoveredAndLogged(t *testing.T) {
	var buf syncBuffer
	s := jsonLoggedServer(&buf)
	s.Handle("/api/v1/boom", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(buf.String(), "http handler panicked") {
		t.Fatalf("panic not logged: %s", buf.String())
	}
	if lines := requestLines(t, buf.String()); len(lines) != 1 || lines[0]["status"] != float64(500) {
		t.Fatalf("request log after panic: %s", buf.String())
	}
}
