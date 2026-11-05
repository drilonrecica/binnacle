// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	authpkg "github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type SnapshotDecorator interface {
	Decorate(context.Context, metrics.Snapshot) metrics.Snapshot
}

func (s *Server) EnableResources(engine *metrics.Engine, auth Authorizer, store *storage.Manager, protection *authpkg.Protection, decorators ...SnapshotDecorator) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		if !requireAuth(w, r, auth) {
			return
		}
		if ok, retry := protection.AllowResources(r); !ok {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", maxRetry(retry)))
			WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many resource requests. Try again shortly.", Details: map[string]int{"retryAfterSeconds": maxRetry(retry)}})
			return
		}
		snap := engine.Snapshot()
		for _, decorator := range decorators {
			snap = decorator.Decorate(r.Context(), snap)
		}
		if r.URL.Path == "/api/v1/resources" && r.URL.Query().Get("state") == "archived" {
			// Paginated with the events convention (ADR 021 §6): limit 1..500,
			// default 100, and before=<id> for rows archived earlier than it.
			query := storage.ArchivedQuery{Limit: 100, BeforeID: r.URL.Query().Get("before")}
			if raw := r.URL.Query().Get("limit"); raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil || value < 1 || value > storage.MaxArchivedLimit {
					WriteError(w, 400, Error{Code: "invalid_request", Message: "limit must be between 1 and 500."})
					return
				}
				query.Limit = value
			}
			values, err := store.ArchivedResources(r.Context(), query)
			if err != nil {
				WriteError(w, 500, Error{Code: "storage_error", Message: "Archived resources are unavailable."})
				return
			}
			WriteJSON(w, 200, values)
			return
		}
		if r.URL.Path != "/api/v1/resources" {
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/resources/")
			if id == "" || strings.Contains(id, "/") {
				WriteError(w, 404, Error{Code: "not_found", Message: "Resource not found."})
				return
			}
			for _, v := range snap.Resources {
				if string(v.ID) == id {
					WriteJSON(w, 200, v)
					return
				}
			}
			if value, err := store.Resource(r.Context(), id); err == nil {
				WriteJSON(w, 200, value)
				return
			}
			WriteError(w, 404, Error{Code: "not_found", Message: "Resource not found."})
			return
		}
		WriteJSON(w, 200, snap.Resources)
	})
	s.Handle("/api/v1/resources", handler)
	// The trailing-slash pattern is what makes `/api/v1/resources/{id}` reachable.
	s.Handle("/api/v1/resources/", handler)
}
