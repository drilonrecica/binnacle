// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"net/http"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

func (s *Server) EnableCurrent(engine *metrics.Engine, auth Authorizer) {
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
				return
			}
			if !requireAuth(w, r, auth) {
				return
			}
			next(w, r)
		}
	}
	// The server view carries the filesystems too, each with its role and
	// time-to-full trend, beside the host's memory trend (SPEC §P3-12).
	s.Handle("/api/v1/server", guard(func(w http.ResponseWriter, r *http.Request) {
		snap := engine.Snapshot()
		filesystems := snap.Filesystems
		if filesystems == nil {
			filesystems = []metrics.FilesystemObservation{}
		}
		WriteJSON(w, 200, struct {
			At          string                          `json:"at"`
			Boot        metrics.BootIdentity            `json:"bootIdentity"`
			Host        metrics.HostObservation         `json:"host"`
			Filesystems []metrics.FilesystemObservation `json:"filesystems"`
		}{snap.At.UTC().Format("2006-01-02T15:04:05Z07:00"), snap.BootIdentity, snap.Host, filesystems})
	}))
	s.Handle("/api/v1/collector-health", guard(func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, engine.Snapshot().Collectors) }))
	// Per-mount filesystem usage drives the filesystem and inode alert
	// families; exposing it keeps the Host page consistent with alerting.
	s.Handle("/api/v1/filesystems", guard(func(w http.ResponseWriter, r *http.Request) {
		values := engine.Snapshot().Filesystems
		if values == nil {
			values = []metrics.FilesystemObservation{}
		}
		WriteJSON(w, 200, values)
	}))
}
