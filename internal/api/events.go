// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	authpkg "github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const maxEventRange = 7 * 24 * time.Hour

func (s *Server) EnableEvents(store *storage.Manager, auth Authorizer, protection *authpkg.Protection) {
	s.Handle("/api/v1/events", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		if !requireAuth(w, r, auth) {
			return
		}
		if ok, retry := protection.AllowEvents(r); !ok {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", maxRetry(retry)))
			WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many event queries. Try again shortly.", Details: map[string]int{"retryAfterSeconds": maxRetry(retry)}})
			return
		}
		q := r.URL.Query()
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour)
		if raw := q.Get("from"); raw != "" {
			var e error
			from, e = time.Parse(time.RFC3339, raw)
			if e != nil {
				WriteError(w, 400, Error{Code: "invalid_time_range", Message: "Invalid from timestamp."})
				return
			}
		}
		if raw := q.Get("to"); raw != "" {
			var e error
			to, e = time.Parse(time.RFC3339, raw)
			if e != nil || !from.Before(to) {
				WriteError(w, 400, Error{Code: "invalid_time_range", Message: "Invalid to timestamp."})
				return
			}
		}
		if to.Sub(from) > maxEventRange {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "Event queries are limited to 7 days."})
			return
		}
		severity := q.Get("severity")
		if severity != "" && severity != "info" && severity != "warning" && severity != "critical" {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "severity must be info, warning, or critical."})
			return
		}
		types := []string{}
		for _, value := range strings.Split(q.Get("type"), ",") {
			if value = strings.TrimSpace(value); value != "" {
				types = append(types, value)
			}
		}
		if len(types) > 32 {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "At most 32 event types can be requested."})
			return
		}
		limit := 100
		if raw := q.Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 500 {
				WriteError(w, 400, Error{Code: "invalid_request", Message: "limit must be between 1 and 500."})
				return
			}
			limit = value
		}
		v, e := store.QueryEvents(r.Context(), storage.EventQuery{From: from, To: to, ResourceID: q.Get("resource_id"), Severity: severity, Types: types, Limit: limit, BeforeID: q.Get("before")})
		if e != nil {
			WriteError(w, 500, Error{Code: "storage_error", Message: "Event history is unavailable."})
			return
		}
		WriteJSON(w, 200, v)
	}))
}
