// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/diagnostics"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

func (s *Server) EnableLogs(service *diagnostics.LogService, engine *metrics.Engine, sessions Authorizer) {
	s.Handle("/api/v1/logs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		if sessions == nil || !sessions.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return
		}
		request, err := logRequest(r, engine)
		if err != nil {
			WriteError(w, 400, Error{Code: "invalid_request", Message: err.Error()})
			return
		}
		if !request.Follow {
			result, readErr := service.Read(r.Context(), request, nil)
			if readErr != nil {
				WriteError(w, 502, Error{Code: "logs_unavailable", Message: "Container logs are unavailable."})
				return
			}
			WriteJSON(w, 200, result)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, 500, Error{Code: "stream_unavailable", Message: "Streaming is unavailable."})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		_, readErr := service.Read(r.Context(), request, func(entry diagnostics.LogEntry) error {
			payload, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, "event: log\ndata: %s\n\n", payload); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if readErr == nil {
			_, _ = fmt.Fprint(w, "event: end\ndata: {}\n\n")
			flusher.Flush()
		}
	}))
}

func logRequest(r *http.Request, engine *metrics.Engine) (diagnostics.LogRequest, error) {
	query := r.URL.Query()
	container, resource := strings.TrimSpace(query.Get("container")), strings.TrimSpace(query.Get("resource"))
	if (container == "") == (resource == "") {
		return diagnostics.LogRequest{}, fmt.Errorf("exactly one container or resource is required")
	}
	components := []string{container}
	if resource != "" {
		components = nil
		for _, candidate := range engine.Snapshot().Resources {
			if string(candidate.ID) != resource {
				continue
			}
			for _, component := range candidate.Components {
				components = append(components, string(component.ID))
			}
			break
		}
		if len(components) == 0 {
			return diagnostics.LogRequest{}, fmt.Errorf("resource has no active components")
		}
	}
	if len(components) > diagnostics.MaxLogComponents {
		return diagnostics.LogRequest{}, fmt.Errorf("resource exceeds the component limit")
	}
	limit := 0
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return diagnostics.LogRequest{}, fmt.Errorf("limit must be an integer")
		}
		limit = value
	}
	now := time.Now().UTC()
	from, to := now.Add(-5*time.Minute), now
	switch query.Get("range") {
	case "", "5m":
	case "30m":
		from = now.Add(-30 * time.Minute)
	case "1h":
		from = now.Add(-time.Hour)
	case "custom":
		var err error
		from, err = time.Parse(time.RFC3339, query.Get("from"))
		if err != nil {
			return diagnostics.LogRequest{}, fmt.Errorf("invalid from timestamp")
		}
		to, err = time.Parse(time.RFC3339, query.Get("to"))
		if err != nil || !from.Before(to) || to.Sub(from) > 24*time.Hour {
			return diagnostics.LogRequest{}, fmt.Errorf("custom range must be positive and no longer than 24 hours")
		}
	default:
		return diagnostics.LogRequest{}, fmt.Errorf("range must be 5m, 30m, 1h, or custom")
	}
	search := query.Get("search")
	if len(search) > 256 {
		return diagnostics.LogRequest{}, fmt.Errorf("search is too long")
	}
	return diagnostics.LogRequest{Components: components, From: from.UTC(), To: to.UTC(), Limit: limit, Search: search, Follow: query.Get("follow") == "true"}, nil
}
