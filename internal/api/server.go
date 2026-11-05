// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
)

const MaxRequestBodyBytes int64 = 1 << 20

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
type errorEnvelope struct {
	Error Error `json:"error"`
}
type Server struct {
	mux            *http.ServeMux
	next           atomic.Uint64
	logger         *slog.Logger
	proxies        auth.TrustedProxies
	requestTimeout time.Duration
	writeGrace     time.Duration
	live           *liveHub
}

// RequestTimeout bounds every non-streaming request: the handler context is
// cancelled after it and the connection's read and write deadlines follow it
// (SEC-7). Streaming routes are exempt; see streamingRoute.
const RequestTimeout = 60 * time.Second

// WriteGrace lets a handler whose context just expired still write its error.
const WriteGrace = 5 * time.Second

func New() *Server {
	s := &Server{mux: http.NewServeMux(), logger: slog.Default(), requestTimeout: RequestTimeout, writeGrace: WriteGrace}
	s.mux.HandleFunc("/api/v1/", s.notFound)
	return s
}
func (s *Server) SetLogger(l *slog.Logger) {
	s.logger = l
	packageLog.Store(l)
}

// packageLog is the logger of the most recent SetLogger, for the package
// helpers (WriteJSON and friends) that have no Server.
var packageLog atomic.Pointer[slog.Logger]

// apiLogger is the app logger once SetLogger ran, slog.Default() before.
func apiLogger() *slog.Logger {
	if l := packageLog.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// logResponseWrite records a response body that did not reach the client.
// The status is already sent, so there is nothing to answer; it is almost
// always a client that went away, hence Debug.
func logResponseWrite(err error) {
	if err != nil {
		apiLogger().Debug("response body was not written", "error", err)
	}
}

// SetTrustedProxies supplies the proxy-aware Secure check that decides whether
// HSTS is sent; r.TLS alone never fires behind a TLS-terminating proxy (SEC-18).
// It also decides the client prefix in request logs.
func (s *Server) SetTrustedProxies(p auth.TrustedProxies) { s.proxies = p }

// Handle registers a route on the API mux. Routes are not wrapped one by one;
// Middleware wraps the whole tree once so each request is logged once.
func (s *Server) Handle(pattern string, handler http.Handler) { s.mux.Handle(pattern, handler) }

// Routes returns the bare API mux, for mounting under a root mux that is
// itself wrapped with Middleware.
func (s *Server) Routes() http.Handler { return s.mux }

// Handler returns the API mux wrapped with Middleware, for standalone use.
func (s *Server) Handler() http.Handler { return s.Middleware(s.mux) }
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, Error{Code: "not_found", Message: "The requested endpoint does not exist."})
}

// Middleware is the single wrapper for every HTTP response Binnacle sends: it
// assigns a request id, sets the security headers, bounds non-streaming
// requests, recovers panics and logs the request once (SEC-6, SEC-7, SEC-21).
func (s *Server) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := fmt.Sprintf("req_%d", s.next.Add(1))
		w.Header().Set("X-Request-ID", id)
		setSecurityHeaders(w, r, s.proxies.Secure(r))
		if !streamingRoute(r) && s.requestTimeout > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
			defer cancel()
			r = r.WithContext(ctx)
			s.setDeadlines(w, r, started.Add(s.requestTimeout))
		}
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.logger.Error("http handler panicked", slog.String("id", id), slog.String("path", r.URL.Path), slog.Any("panic", v))
				if !rec.wrote {
					WriteError(rec, http.StatusInternalServerError, Error{Code: "internal_error", Message: "The server could not process the request."})
				}
			}
			s.logRequest(r, id, rec.status, started)
		}()
		next.ServeHTTP(rec, r)
	})
}

func (s *Server) setDeadlines(w http.ResponseWriter, r *http.Request, deadline time.Time) {
	rc := http.NewResponseController(w)
	for _, err := range []error{rc.SetReadDeadline(deadline), rc.SetWriteDeadline(deadline.Add(s.writeGrace))} {
		if err != nil && !errors.Is(err, http.ErrNotSupported) {
			s.logger.Debug("request deadline not applied", slog.String("path", r.URL.Path), slog.Any("error", err))
		}
	}
}

func (s *Server) logRequest(r *http.Request, id string, status int, started time.Time) {
	level := slog.LevelInfo
	// Health probes and immutable assets are frequent and uninteresting.
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/assets/") {
		level = slog.LevelDebug
	}
	s.logger.LogAttrs(r.Context(), level, "http request",
		slog.String("id", id),
		slog.String("method", r.Method),
		slog.String("path", sanitizePath(r.URL)),
		slog.Int("status", status),
		slog.Duration("duration", time.Since(started)),
		slog.String("client", s.proxies.ClientPrefix(r)),
	)
}

// streamingRoute reports whether a request is a long-lived stream that must
// not get a request deadline: the SSE live feed and followed logs.
func streamingRoute(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/live":
		return true
	case "/api/v1/logs":
		return r.URL.Query().Get("follow") == "true"
	}
	return false
}

func setSecurityHeaders(w http.ResponseWriter, r *http.Request, secure bool) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", ContentSecurityPolicy)
	if secure {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	}
}

// ContentSecurityPolicy is sent on every response. The embedded UI loads only
// same-origin module scripts and stylesheets; Svelte sets inline style
// attributes, hence 'unsafe-inline' for styles only.
const ContentSecurityPolicy = "default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none';"

func sanitizePath(u *url.URL) string {
	values := u.Query()
	redact := []string{"token", "setup_token", "password", "csrf"}
	for _, key := range redact {
		if values.Has(key) {
			values.Set(key, "[REDACTED]")
		}
	}
	u = &url.URL{Path: u.Path, RawQuery: values.Encode()}
	return u.RequestURI()
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(status)
}
func (r *responseRecorder) Write(p []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the connection beneath.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	logResponseWrite(json.NewEncoder(w).Encode(value))
}
func WriteError(w http.ResponseWriter, status int, e Error) {
	WriteJSON(w, status, errorEnvelope{Error: e})
}
func DecodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("request body must contain one JSON value")
	}
	return nil
}
func UTC(t time.Time) time.Time               { return t.UTC() }
func Context(r *http.Request) context.Context { return r.Context() }
