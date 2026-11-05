// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Connection limits for the listener. Streaming routes (SSE, log follow) need
// unbounded write time, so there is no server-wide ReadTimeout or WriteTimeout;
// the root middleware sets per-request deadlines on every other route (SEC-7).
const (
	ReadHeaderTimeout = 10 * time.Second
	IdleTimeout       = 120 * time.Second
	MaxHeaderBytes    = 64 << 10
)

// Routes are the handlers mounted on the root mux. Middleware, when set, wraps
// the whole mux so the UI document, assets, /healthz and /metrics get the same
// security headers, deadlines and request logging as /api/v1 (SEC-6).
type Routes struct {
	API        http.Handler
	Static     http.Handler
	Metrics    http.Handler
	Middleware func(http.Handler) http.Handler
}

// HTTPServer serves the root mux, including the unauthenticated /healthz.
type HTTPServer struct {
	address  string
	app      *Application
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	done     chan struct{}
}

func NewHTTPServer(address string, application *Application, routes Routes) *HTTPServer {
	h := &HTTPServer{address: address, app: application}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.health)
	if routes.Metrics != nil {
		mux.Handle("/metrics", routes.Metrics)
	}
	if routes.API != nil {
		mux.Handle("/api/v1/", routes.API)
	}
	if routes.Static != nil {
		mux.Handle("/", routes.Static)
	}
	var handler http.Handler = mux
	if routes.Middleware != nil {
		handler = routes.Middleware(mux)
	}
	h.server = &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: ReadHeaderTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
	}
	return h
}

// Handler returns the root handler, middleware included.
func (h *HTTPServer) Handler() http.Handler { return h.server.Handler }

// Addr returns the bound listener address once Start has succeeded.
func (h *HTTPServer) Addr() net.Addr {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listener == nil {
		return nil
	}
	return h.listener.Addr()
}

func (h *HTTPServer) Start(context.Context) error {
	ln, err := net.Listen("tcp", h.address)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	h.mu.Lock()
	h.listener = ln
	h.done = done
	h.mu.Unlock()
	go func() {
		defer close(done)
		if err := h.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.fail(fmt.Errorf("http server: %w", err))
		}
	}()
	return nil
}

func (h *HTTPServer) fail(err error) {
	if h.app != nil {
		h.app.Fail(err)
	}
}

func (h *HTTPServer) Stop(ctx context.Context) error {
	err := h.server.Shutdown(ctx)
	h.mu.Lock()
	done := h.done
	h.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return err
}

// health reports only the lifecycle state. The exact version is not exposed
// to unauthenticated callers (SEC-21); diagnostics carry it behind a session.
func (h *HTTPServer) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state := StateStarting
	if h.app != nil {
		state = h.app.State()
	}
	status := http.StatusServiceUnavailable
	if state == StateRunning {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_ = json.NewEncoder(w).Encode(struct {
			State State `json:"state"`
		}{state})
	}
}
