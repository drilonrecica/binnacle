// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"github.com/drilonrecica/talos/internal/metrics"
	"github.com/drilonrecica/talos/internal/storage"
	"net/http"
	"strings"
)

func (s *Server) EnableResources(engine *metrics.Engine, auth Authorizer, store *storage.Manager) {
	s.Handle("/api/v1/resources", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil || !auth.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		snap := engine.Snapshot()
		if r.URL.Path == "/api/v1/resources" && r.URL.Query().Get("state") == "archived" {
			values, err := store.ArchivedResources(r.Context())
			if err != nil {
				WriteError(w, 500, Error{Code: "storage_error", Message: "Archived resources are unavailable."})
				return
			}
			WriteJSON(w, 200, values)
			return
		}
		if r.URL.Path != "/api/v1/resources" {
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/resources/")
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
	}))
}
