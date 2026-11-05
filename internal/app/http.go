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
	// HTTPStopBudget bounds Shutdown: streams end at once through the base
	// context, other requests get this long to finish (RUN-1).
	HTTPStopBudget = 3 * time.Second
	// ReadinessTimeout bounds the database probe behind /readyz.
	ReadinessTimeout = 2 * time.Second
)

// Readiness is what the readiness probe reports besides the lifecycle state:
// "ok" or "unavailable" for the database, and the state word of every
// collector (healthy, degraded, down, unknown). Reasons are left out because
// /readyz is unauthenticated (SEC-21); the UI shows them behind a session.
type Readiness struct {
	Database   string            `json:"database"`
	Collectors map[string]string `json:"collectors"`
}

// Routes are the handlers mounted on the root mux. Middleware, when set, wraps
// the whole mux so the UI document, assets, /healthz and /metrics get the same
// security headers, deadlines and request logging as /api/v1 (SEC-6).
type Routes struct {
	API        http.Handler
	Static     http.Handler
	Metrics    http.Handler
	Middleware func(http.Handler) http.Handler
	// Ready reports database and collector state for /readyz. When nil the
	// database is reported "unknown" and only the lifecycle state counts.
	Ready func(context.Context) Readiness
}

// HTTPServer serves the root mux, including the unauthenticated /healthz
// (liveness) and /readyz (readiness).
//
// Every request context derives from a base context that Shutdown cancels
// through RegisterOnShutdown. Server.Shutdown alone only waits for active
// connections to go idle, which a live stream or a followed log never does,
// so without it shutdown used to wait out its whole budget (RUN-1).
type HTTPServer struct {
	address    string
	app        *Application
	server     *http.Server
	ready      func(context.Context) Readiness
	listener   net.Listener
	mu         sync.Mutex
	done       chan struct{}
	base       context.Context
	cancelBase context.CancelFunc
}

func NewHTTPServer(address string, application *Application, routes Routes) *HTTPServer {
	base, cancelBase := context.WithCancel(context.Background())
	h := &HTTPServer{address: address, app: application, ready: routes.Ready, base: base, cancelBase: cancelBase}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.health)
	mux.HandleFunc("/readyz", h.readiness)
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
		BaseContext:       func(net.Listener) context.Context { return base },
	}
	h.server.RegisterOnShutdown(cancelBase)
	return h
}

func (h *HTTPServer) Name() string              { return "http" }
func (h *HTTPServer) StopBudget() time.Duration { return HTTPStopBudget }

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

// Stop closes the listener, cancels every request context so streams end,
// and waits for in-flight requests within ctx. Connections still open at the
// deadline are closed.
func (h *HTTPServer) Stop(ctx context.Context) error {
	err := h.server.Shutdown(ctx)
	h.cancelBase()
	if err != nil {
		err = errors.Join(err, h.server.Close())
	}
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

// readiness answers 200 while the application is running and the database
// answers a query, 503 otherwise. Collectors are reported but never make
// Binnacle unready: a degraded collector is shown in the UI, not hidden
// behind a failing probe (degrade explicitly).
func (h *HTTPServer) readiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state := StateStarting
	if h.app != nil {
		state = h.app.State()
	}
	report := Readiness{Database: "unknown", Collectors: map[string]string{}}
	if h.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), ReadinessTimeout)
		report = h.ready(ctx)
		cancel()
		if report.Collectors == nil {
			report.Collectors = map[string]string{}
		}
	}
	ready := state == StateRunning && (h.ready == nil || report.Database == "ok")
	status := http.StatusServiceUnavailable
	if ready {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_ = json.NewEncoder(w).Encode(struct {
			Ready bool  `json:"ready"`
			State State `json:"state"`
			Readiness
		}{ready, state, report})
	}
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
