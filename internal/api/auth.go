// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"net/http"
	"time"

	"github.com/drilonrecica/talos/internal/auth"
)

func (s *Server) EnableAuth(credentials *auth.Credentials, sessions *auth.Sessions, limiter *auth.Limiter, proxies auth.TrustedProxies) {
	limited := func(w http.ResponseWriter, r *http.Request, username string) bool {
		ok, _ := limiter.Allow("login-ip:"+proxies.ClientPrefix(r), auth.BucketPolicy{Capacity: 10, Refill: time.Minute})
		ok2, _ := limiter.Allow("login-user:"+username, auth.BucketPolicy{Capacity: 5, Refill: 5 * time.Minute})
		if ok && ok2 {
			return true
		}
		w.Header().Set("Retry-After", "60")
		WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many login attempts. Try again later."})
		return false
	}
	s.Handle("/api/v1/auth/login", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if DecodeJSON(r, &body) != nil || !limited(w, r, body.Username) {
			return
		}
		user, err := credentials.Authenticate(r.Context(), body.Username, body.Password)
		if err != nil {
			WriteError(w, 401, Error{Code: "invalid_credentials", Message: "Invalid username or password."})
			return
		}
		token, csrf, session, err := sessions.IssueWithCSRF(r.Context(), user.ID)
		if err != nil {
			WriteError(w, 500, Error{Code: "session_error", Message: "Could not start session."})
			return
		}
		secure := proxies.Secure(r)
		auth.SetSessionCookie(w, token, secure, session.ExpiresAt)
		auth.SetCSRFCookie(w, csrf, secure)
		w.WriteHeader(http.StatusNoContent)
	}))
	s.Handle("/api/v1/auth/logout", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		if !sessions.ValidCSRF(r) {
			WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
			return
		}
		_ = sessions.Revoke(r.Context(), auth.TokenFromRequest(r))
		auth.ClearSessionCookie(w, proxies.Secure(r))
		w.WriteHeader(http.StatusNoContent)
	}))
	s.Handle("/api/v1/auth/logout-all", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		if !sessions.ValidCSRF(r) {
			WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
			return
		}
		session, err := sessions.Authenticate(r.Context(), auth.TokenFromRequest(r))
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		_ = sessions.RevokeAll(r.Context(), session.UserID)
		auth.ClearSessionCookie(w, proxies.Secure(r))
		w.WriteHeader(http.StatusNoContent)
	}))
}
