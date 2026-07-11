// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
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
	mux  *http.ServeMux
	next atomic.Uint64
}

func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.mux.HandleFunc("/api/v1/", s.notFound)
	return s
}
func (s *Server) Handle(pattern string, handler http.Handler) { s.mux.Handle(pattern, s.wrap(handler)) }
func (s *Server) Handler() http.Handler                       { return s.wrap(s.mux) }
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, Error{Code: "not_found", Message: "The requested endpoint does not exist."})
}
func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("req_%d", s.next.Add(1))
		w.Header().Set("X-Request-ID", id)
		defer func() {
			if recover() != nil {
				WriteError(w, http.StatusInternalServerError, Error{Code: "internal_error", Message: "The server could not process the request."})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
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
