// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/sweep"
)

// domainInput is the body of POST and PATCH on sweep domains. Pointers
// distinguish an omitted field from a zero value, so a PATCH changes only
// what it names.
type domainInput struct {
	Domain          string `json:"domain"`
	IntervalSeconds *int   `json:"intervalSeconds"`
	Enabled         *bool  `json:"enabled"`
}

// apply overlays the input on a domain; the domain name is normalized the
// way a user would type it (any case, maybe a trailing dot).
func (v domainInput) apply(d sweep.Domain) sweep.Domain {
	if v.Domain != "" {
		d.Domain = sweep.NormalizeDomain(v.Domain)
	}
	if v.IntervalSeconds != nil {
		d.Interval = time.Duration(*v.IntervalSeconds) * time.Second
	}
	if v.Enabled != nil {
		d.Enabled = *v.Enabled
	}
	return d
}

// EnableSweep serves the domain sweep (ADR 028) under /api/v1/sweep/domains.
// defaultInterval is the interval a new domain gets when the body names none.
func (s *Server) EnableSweep(repo *sweep.Repository, scheduler *sweep.Scheduler, defaultInterval time.Duration, a Authorizer, sessions *auth.Sessions, protections ...*auth.Protection) {
	if defaultInterval <= 0 {
		defaultInterval = sweep.DefaultInterval
	}
	s.Handle("/api/v1/sweep/domains", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if a == nil || !a.Authorize(r) {
				WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
				return
			}
			if len(protections) > 0 {
				if ok, retry := protections[0].AllowResources(r); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(maxRetry(retry)))
					WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many sweep requests."})
					return
				}
			}
			limit, offset := sweepPagination(r)
			v, err := repo.List(r.Context(), limit, offset)
			if err != nil {
				WriteError(w, 500, Error{Code: "storage_error", Message: "Swept domains are unavailable."})
				return
			}
			WriteJSON(w, 200, v)
		case http.MethodPost:
			if !authorizeMutation(w, r, a, sessions) {
				return
			}
			var body domainInput
			if DecodeJSON(r, &body) != nil {
				WriteError(w, 400, Error{Code: "invalid_request", Message: "Invalid domain definition."})
				return
			}
			d := body.apply(sweep.Domain{ID: apiID("domain_"), Interval: defaultInterval, Enabled: true})
			if err := d.Validate(); err != nil {
				WriteError(w, 400, Error{Code: "invalid_request", Message: err.Error()})
				return
			}
			if err := repo.Create(r.Context(), d); err != nil {
				writeSweepError(w, err, "The domain could not be added.")
				return
			}
			now := time.Now().UTC()
			d.CreatedAt, d.UpdatedAt = now, now
			WriteJSON(w, 201, d)
		default:
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Method not allowed."})
		}
	}))
	s.Handle("/api/v1/sweep/domains/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/sweep/domains/")
		run := strings.HasSuffix(path, "/run")
		id := strings.TrimSuffix(path, "/run")
		if id == "" || strings.Contains(id, "/") {
			WriteError(w, 404, Error{Code: "not_found", Message: "Domain not found."})
			return
		}
		if run {
			if r.Method != http.MethodPost {
				WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
				return
			}
			if !authorizeMutation(w, r, a, sessions) {
				return
			}
			result, err := scheduler.RunNow(r.Context(), id)
			if errors.Is(err, sql.ErrNoRows) {
				WriteError(w, 404, Error{Code: "not_found", Message: "Domain not found."})
				return
			}
			if err != nil {
				WriteError(w, 400, Error{Code: "sweep_failed", Message: "The domain could not be swept: " + err.Error()})
				return
			}
			WriteJSON(w, 200, result)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if a == nil || !a.Authorize(r) {
				WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
				return
			}
			v, err := repo.Get(r.Context(), id)
			if err != nil {
				WriteError(w, 404, Error{Code: "not_found", Message: "Domain not found."})
				return
			}
			WriteJSON(w, 200, v)
		case http.MethodPatch:
			if !authorizeMutation(w, r, a, sessions) {
				return
			}
			existing, err := repo.Get(r.Context(), id)
			if err != nil {
				WriteError(w, 404, Error{Code: "not_found", Message: "Domain not found."})
				return
			}
			var body domainInput
			if DecodeJSON(r, &body) != nil {
				WriteError(w, 400, Error{Code: "invalid_request", Message: "Invalid domain update."})
				return
			}
			updated := body.apply(existing.Domain)
			if err = updated.Validate(); err != nil {
				WriteError(w, 400, Error{Code: "invalid_request", Message: err.Error()})
				return
			}
			if err = repo.Update(r.Context(), updated); err != nil {
				writeSweepError(w, err, "The domain could not be updated.")
				return
			}
			entry, err := repo.Get(r.Context(), id)
			if err != nil {
				WriteError(w, 500, Error{Code: "storage_error", Message: "The domain could not be read back."})
				return
			}
			WriteJSON(w, 200, entry)
		case http.MethodDelete:
			if !authorizeMutation(w, r, a, sessions) {
				return
			}
			if err := repo.Delete(r.Context(), id); err != nil {
				writeSweepError(w, err, "The domain could not be removed.")
				return
			}
			w.WriteHeader(204)
		default:
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Method not allowed."})
		}
	}))
}

// writeSweepError maps repository errors to a response: a duplicate domain
// is a conflict, a missing one is not found, anything else is the fallback.
func writeSweepError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, sweep.ErrDuplicate):
		WriteError(w, 409, Error{Code: "domain_exists", Message: "This domain is already swept."})
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, 404, Error{Code: "not_found", Message: "Domain not found."})
	default:
		WriteError(w, 500, Error{Code: "storage_error", Message: fallback})
	}
}

// sweepPagination reads limit and offset; the sweep is a short list, so the
// default page holds every domain of a typical deployment.
func sweepPagination(r *http.Request) (int, int) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 || limit > 500 {
		limit = 200
	}
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return limit, offset
}
