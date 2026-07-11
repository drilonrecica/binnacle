// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"github.com/drilonrecica/talos/internal/storage"
	"net/http"
	"time"
)

func (s *Server) EnableMetrics(store *storage.Manager, auth Authorizer) {
	s.Handle("/api/v1/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		if auth == nil || !auth.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		from, e := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if e != nil {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "A valid from timestamp is required."})
			return
		}
		to, e := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if e != nil || !from.Before(to) {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "The requested start time must be before the end time."})
			return
		}
		points, e := store.HostCPU(r.Context(), from, to, 1000)
		if e != nil {
			WriteError(w, 500, Error{Code: "storage_error", Message: "Metric history is unavailable."})
			return
		}
		WriteJSON(w, 200, map[string]any{"scope": "host", "from": from.UTC(), "to": to.UTC(), "resolution": "10s", "series": points})
	}))
}
