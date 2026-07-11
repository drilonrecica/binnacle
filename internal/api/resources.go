// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"github.com/drilonrecica/talos/internal/metrics"
	"net/http"
	"strings"
)

func (s *Server) EnableResources(engine *metrics.Engine, auth Authorizer) {
	s.Handle("/api/v1/resources", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil || !auth.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		snap := engine.Snapshot()
		if r.URL.Path != "/api/v1/resources" {
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/resources/")
			for _, v := range snap.Resources {
				if string(v.ID) == id {
					WriteJSON(w, 200, v)
					return
				}
			}
			WriteError(w, 404, Error{Code: "not_found", Message: "Resource not found."})
			return
		}
		WriteJSON(w, 200, snap.Resources)
	}))
}
