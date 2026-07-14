// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/drilonrecica/binnacle/internal/auth"
)

type AuthorizationDecision struct {
	Allowed       bool
	Status        int
	Code, Message string
}
type DecisionAuthorizer interface {
	Decision(*http.Request) AuthorizationDecision
}
type ScopedAuthorizer struct {
	Sessions          *auth.Sessions
	Tokens            *auth.APITokenRepository
	Scope             auth.APIScope
	TokenPathPrefixes []string
}

func (a ScopedAuthorizer) Authorize(r *http.Request) bool { return a.Decision(r).Allowed }
func (a ScopedAuthorizer) Decision(r *http.Request) AuthorizationDecision {
	header := r.Header.Get("Authorization")
	if header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return unauthorizedToken()
		}
		allowedPath := len(a.TokenPathPrefixes) == 0
		for _, prefix := range a.TokenPathPrefixes {
			allowedPath = allowedPath || strings.HasPrefix(r.URL.Path, prefix)
		}
		if !allowedPath {
			return unauthorizedToken()
		}
		err := a.Tokens.Authenticate(r.Context(), parts[1], a.Scope)
		if errors.Is(err, auth.ErrAPIScopeInsufficient) {
			return AuthorizationDecision{Status: 403, Code: "insufficient_scope", Message: "The API token does not grant the required scope."}
		}
		if err != nil {
			return unauthorizedToken()
		}
		return AuthorizationDecision{Allowed: true}
	}
	if a.Sessions != nil && a.Sessions.Authorize(r) {
		return AuthorizationDecision{Allowed: true}
	}
	return AuthorizationDecision{Status: 401, Code: "unauthorized", Message: "Authentication is required."}
}
func unauthorizedToken() AuthorizationDecision {
	return AuthorizationDecision{Status: 401, Code: "invalid_token", Message: "A valid API token is required."}
}

func requireAuth(w http.ResponseWriter, r *http.Request, a Authorizer) bool {
	if a == nil {
		WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
		return false
	}
	if detailed, ok := a.(DecisionAuthorizer); ok {
		decision := detailed.Decision(r)
		if decision.Allowed {
			return true
		}
		WriteError(w, decision.Status, Error{Code: decision.Code, Message: decision.Message})
		return false
	}
	if !a.Authorize(r) {
		WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
		return false
	}
	return true
}
