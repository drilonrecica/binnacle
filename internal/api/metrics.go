// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	authpkg "github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func (s *Server) EnableMetrics(store *storage.Manager, authz Authorizer, protection *authpkg.Protection) {
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				WriteError(w, http.StatusMethodNotAllowed, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
				return
			}
			if !requireAuth(w, r, authz) {
				return
			}
			if ok, retry := protection.AllowMetrics(r); !ok {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", maxRetry(retry)))
				WriteError(w, http.StatusTooManyRequests, Error{Code: "rate_limited", Message: "Too many metric queries. Try again shortly.", Details: map[string]int{"retryAfterSeconds": maxRetry(retry)}})
				return
			}
			next(w, r)
		}
	}
	s.Handle("/api/v1/metrics", guard(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, err := time.Parse(time.RFC3339, q.Get("from"))
		if err != nil {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "A valid from timestamp is required."})
			return
		}
		to, err := time.Parse(time.RFC3339, q.Get("to"))
		if err != nil {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "A valid to timestamp is required."})
			return
		}
		result, err := store.QueryMetrics(r.Context(), storage.MetricQuery{Scope: q.Get("scope"), ID: q.Get("id"), Metrics: parseMetrics(q.Get("metrics")), From: from, To: to})
		if err != nil {
			WriteError(w, 400, Error{Code: "invalid_metrics_query", Message: "The metrics request is invalid."})
			return
		}
		WriteJSON(w, http.StatusOK, result)
	}))
	// One bounded request feeds every row sparkline on the resource lists.
	s.Handle("/api/v1/metrics/sparklines", guard(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		window, ok := map[string]time.Duration{"": time.Hour, "1h": time.Hour, "3h": 3 * time.Hour, "6h": 6 * time.Hour}[q.Get("range")]
		if !ok {
			WriteError(w, 400, Error{Code: "invalid_time_range", Message: "range must be 1h, 3h, or 6h."})
			return
		}
		now := time.Now().UTC()
		result, err := store.ResourceSparklines(r.Context(), storage.SparklineQuery{Metrics: parseMetrics(q.Get("metrics")), From: now.Add(-window), To: now})
		if err != nil {
			WriteError(w, 400, Error{Code: "invalid_metrics_query", Message: "The sparkline request is invalid."})
			return
		}
		WriteJSON(w, http.StatusOK, result)
	}))
}

func parseMetrics(raw string) []storage.Metric {
	metrics := []storage.Metric{}
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			metrics = append(metrics, storage.Metric(value))
		}
	}
	return metrics
}

func maxRetry(d time.Duration) int {
	if d < time.Second {
		return 1
	}
	return int(d.Round(time.Second).Seconds())
}
