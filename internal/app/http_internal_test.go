// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHTTPServerConnectionLimits(t *testing.T) {
	h := NewHTTPServer("127.0.0.1:0", New(quietLogger()), Routes{})
	if h.server.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %s, want 10s", h.server.ReadHeaderTimeout)
	}
	if h.server.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %s, want 120s", h.server.IdleTimeout)
	}
	if h.server.MaxHeaderBytes <= 0 || h.server.MaxHeaderBytes > 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want a positive limit below the 1 MiB default", h.server.MaxHeaderBytes)
	}
}

func TestHealthzOmitsVersion(t *testing.T) {
	h := NewHTTPServer("127.0.0.1:0", New(quietLogger()), Routes{})
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /healthz: %v", err)
	}
	if _, ok := body["version"]; ok {
		t.Fatalf("/healthz exposes the version to unauthenticated callers: %s", rec.Body.String())
	}
	if body["state"] != string(StateStarting) {
		t.Fatalf("state = %v, want %q", body["state"], StateStarting)
	}
}

func TestServeErrorStopsApplication(t *testing.T) {
	application := New(quietLogger())
	h := NewHTTPServer("127.0.0.1:0", application, Routes{})
	application.Add(h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- application.Run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for application.State() != StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("application did not reach running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Closing the listener underneath Serve is a failure, not a shutdown.
	h.mu.Lock()
	ln := h.listener
	h.mu.Unlock()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "http server") {
			t.Fatalf("Run returned %v, want the http server error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run kept running after the HTTP server stopped serving")
	}
}

func TestFailBeforeRunIsKept(t *testing.T) {
	application := New(quietLogger())
	application.Fail(io.ErrUnexpectedEOF)
	application.Fail(io.ErrClosedPipe) // only the first failure is returned
	err := application.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("Run returned %v, want %v", err, io.ErrUnexpectedEOF)
	}
}
