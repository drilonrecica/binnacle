// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
)

func (s *Server) EnableAPITokens(repo *auth.APITokenRepository, sessions *auth.Sessions) {
	userID := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		session, err := sessions.Authenticate(r.Context(), auth.TokenFromRequest(r))
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return "", false
		}
		return session.UserID, true
	}
	s.Handle("/api/v1/api-tokens", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := userID(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			values, err := repo.List(r.Context(), id)
			if err != nil {
				WriteError(w, 500, Error{Code: "storage_error", Message: "API tokens are unavailable."})
				return
			}
			WriteJSON(w, 200, map[string]any{"tokens": values, "scopes": []auth.APIScope{auth.ScopeServerRead, auth.ScopeResourcesRead, auth.ScopeMetricsRead, auth.ScopeEventsRead, auth.ScopeIncidentsRead}})
		case http.MethodPost:
			if !sessions.ValidCSRF(r) {
				WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
				return
			}
			var body struct {
				Name      string          `json:"name"`
				Scopes    []auth.APIScope `json:"scopes"`
				ExpiresAt string          `json:"expiresAt,omitempty"`
			}
			if DecodeJSON(r, &body) != nil {
				WriteError(w, 400, Error{Code: "invalid_request", Message: "API token configuration is invalid."})
				return
			}
			var expiry *time.Time
			if body.ExpiresAt != "" {
				value, err := time.Parse(time.RFC3339, body.ExpiresAt)
				if err != nil {
					WriteError(w, 400, Error{Code: "invalid_expiry", Message: "Token expiry must be an RFC 3339 timestamp."})
					return
				}
				expiry = &value
			}
			token, plaintext, err := repo.Create(r.Context(), id, body.Name, body.Scopes, expiry)
			if err != nil {
				WriteError(w, 400, Error{Code: "token_invalid", Message: err.Error()})
				return
			}
			WriteJSON(w, 201, map[string]any{"token": token, "plaintext": plaintext})
		default:
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET and POST are supported."})
		}
	}))
	s.Handle("/api/v1/api-tokens/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := userID(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodDelete {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only DELETE is supported."})
			return
		}
		if !sessions.ValidCSRF(r) {
			WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
			return
		}
		tokenID := strings.TrimPrefix(r.URL.Path, "/api/v1/api-tokens/")
		if tokenID == "" || strings.Contains(tokenID, "/") {
			WriteError(w, 404, Error{Code: "not_found", Message: "API token not found."})
			return
		}
		if err := repo.Revoke(r.Context(), id, tokenID); err != nil {
			WriteError(w, 404, Error{Code: "not_found", Message: "API token not found."})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
