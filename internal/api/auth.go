// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
)

func (s *Server) EnableAuth(credentials *auth.Credentials, sessions *auth.Sessions, protection *auth.Protection, options ...any) {
	var mfa *auth.MFA
	var proxy *auth.ProxyAuthenticator
	for _, option := range options {
		switch value := option.(type) {
		case *auth.MFA:
			mfa = value
		case *auth.ProxyAuthenticator:
			proxy = value
		}
	}
	proxies := protection.Proxies()
	exists := func(r *http.Request) func(string) bool {
		return func(username string) bool { return credentials.Exists(r.Context(), username) }
	}
	limited := func(w http.ResponseWriter, r *http.Request, username string, known func(string) bool) bool {
		ok, retry := protection.AllowLogin(r, username, known)
		if ok {
			return true
		}
		seconds := maxRetry(retry)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
		WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many login attempts. Try again later.", Details: map[string]int{"retryAfterSeconds": seconds}})
		return false
	}
	s.Handle("/api/v1/auth/login", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		if proxy != nil && !proxy.AllowsLocal() {
			WriteError(w, 404, Error{Code: "not_found", Message: "Local login is disabled."})
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Code     string `json:"code,omitempty"`
		}
		if DecodeJSON(r, &body) != nil {
			WriteError(w, http.StatusBadRequest, Error{Code: "invalid_request", Message: "A username and password are required."})
			return
		}
		if !limited(w, r, body.Username, exists(r)) {
			return
		}
		user, err := credentials.Authenticate(r.Context(), body.Username, body.Password)
		if err == nil && mfa != nil {
			err = mfa.Verify(r.Context(), user.ID, body.Code)
		}
		if errors.Is(err, auth.ErrHashingBusy) {
			writeHashingBusy(w)
			return
		}
		if err != nil {
			WriteError(w, 401, Error{Code: "invalid_credentials", Message: "Invalid username, password, or authentication code."})
			return
		}
		if previous := auth.TokenFromRequest(r); previous != "" {
			_ = sessions.Revoke(r.Context(), previous)
		}
		token, csrf, session, err := sessions.IssueWithCSRF(r.Context(), user.ID)
		if err != nil {
			WriteError(w, 500, Error{Code: "session_error", Message: "Could not start session."})
			return
		}
		secure := proxies.Secure(r)
		auth.SetSessionCookie(w, token, secure, session.AbsoluteExpires)
		auth.SetCSRFCookie(w, csrf, secure, session.AbsoluteExpires)
		w.WriteHeader(http.StatusNoContent)
	}))
	s.Handle("/api/v1/auth/session", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		session, err := sessions.Authenticate(r.Context(), auth.TokenFromRequest(r))
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		user, err := credentials.UserByID(r.Context(), session.UserID)
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		WriteJSON(w, 200, map[string]any{
			"user":              map[string]string{"id": user.ID, "username": user.Username},
			"expiresAt":         session.ExpiresAt.UTC().Format(time.RFC3339),
			"absoluteExpiresAt": session.AbsoluteExpires.UTC().Format(time.RFC3339),
			"authMethod":        session.AuthMethod,
		})
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
		auth.ClearCSRFCookie(w, proxies.Secure(r))
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
		auth.ClearCSRFCookie(w, proxies.Secure(r))
		w.WriteHeader(http.StatusNoContent)
	}))
	// Password change (SEC-21): same-origin, CSRF, the current password, and
	// the login throttle so the stored hash cannot be brute-forced from a
	// stolen session. Other sessions are revoked; the caller stays signed in.
	s.Handle("/api/v1/auth/password", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		if !sessions.ValidCSRF(r) {
			WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
			return
		}
		token := auth.TokenFromRequest(r)
		session, err := sessions.Authenticate(r.Context(), token)
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return
		}
		user, err := credentials.UserByID(r.Context(), session.UserID)
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return
		}
		if !limited(w, r, user.Username, func(string) bool { return true }) {
			return
		}
		var body struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if DecodeJSON(r, &body) != nil {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "The current and new passwords are required."})
			return
		}
		if err = auth.ValidatePassword(body.NewPassword); err != nil {
			WriteError(w, 400, Error{Code: "password_policy", Message: "The new " + err.Error() + "."})
			return
		}
		err = credentials.ChangePassword(r.Context(), user.ID, body.CurrentPassword, body.NewPassword)
		switch {
		case errors.Is(err, auth.ErrHashingBusy):
			writeHashingBusy(w)
			return
		case errors.Is(err, auth.ErrInvalidCredentials):
			WriteError(w, 401, Error{Code: "invalid_credentials", Message: "The current password is incorrect."})
			return
		case err != nil:
			WriteError(w, 500, Error{Code: "password_change_failed", Message: "The password could not be changed."})
			return
		}
		if err = sessions.RevokeOthers(r.Context(), user.ID, token); err != nil {
			s.logger.Error("revoke other sessions after password change", "error", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func (s *Server) EnableMFA(mfa *auth.MFA, credentials *auth.Credentials, sessions *auth.Sessions, protection *auth.Protection) {
	current := func(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
		session, err := sessions.Authenticate(r.Context(), auth.TokenFromRequest(r))
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return auth.User{}, false
		}
		user, err := credentials.UserByID(r.Context(), session.UserID)
		if err != nil {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "A browser session is required."})
			return auth.User{}, false
		}
		return user, true
	}
	csrf := func(w http.ResponseWriter, r *http.Request) bool {
		if !sessions.ValidCSRF(r) {
			WriteError(w, 403, Error{Code: "csrf_invalid", Message: "A valid CSRF token is required."})
			return false
		}
		return true
	}
	s.Handle("/api/v1/auth/mfa", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		user, ok := current(w, r)
		if !ok {
			return
		}
		enabled, err := mfa.Enabled(r.Context(), user.ID)
		if err != nil {
			WriteError(w, 500, Error{Code: "mfa_unavailable", Message: "MFA status is unavailable."})
			return
		}
		WriteJSON(w, 200, map[string]bool{"enabled": enabled})
	}))
	s.Handle("/api/v1/auth/mfa/enroll", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		user, ok := current(w, r)
		if !ok || !csrf(w, r) {
			return
		}
		if allowed, retry := protection.AllowLogin(r, user.Username, func(string) bool { return true }); !allowed {
			seconds := maxRetry(retry)
			w.Header().Set("Retry-After", fmt.Sprint(seconds))
			WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many authentication attempts."})
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if DecodeJSON(r, &body) != nil {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "Current password is required."})
			return
		}
		enrollment, err := mfa.Begin(r.Context(), user, body.Password)
		if errors.Is(err, auth.ErrHashingBusy) {
			writeHashingBusy(w)
			return
		}
		if err != nil {
			WriteError(w, 400, Error{Code: "mfa_enrollment_failed", Message: "MFA enrollment could not be started."})
			return
		}
		WriteJSON(w, 200, enrollment)
	}))
	s.Handle("/api/v1/auth/mfa/confirm", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		user, ok := current(w, r)
		if !ok || !csrf(w, r) {
			return
		}
		var body struct {
			Code string `json:"code"`
		}
		if DecodeJSON(r, &body) != nil {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "Authentication code is required."})
			return
		}
		codes, err := mfa.Confirm(r.Context(), user.ID, body.Code)
		if err != nil {
			WriteError(w, 400, Error{Code: "mfa_confirmation_failed", Message: "Authentication code is invalid or expired."})
			return
		}
		WriteJSON(w, 200, map[string]any{"recoveryCodes": codes})
	}))
	s.Handle("/api/v1/auth/mfa/disable", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only POST is supported."})
			return
		}
		user, ok := current(w, r)
		if !ok || !csrf(w, r) {
			return
		}
		var body struct {
			Password string `json:"password"`
			Code     string `json:"code"`
		}
		if DecodeJSON(r, &body) != nil {
			WriteError(w, 400, Error{Code: "invalid_request", Message: "Password and authentication code are required."})
			return
		}
		if err := mfa.Disable(r.Context(), user, body.Password, body.Code); err != nil {
			if errors.Is(err, auth.ErrHashingBusy) {
				writeHashingBusy(w)
				return
			}
			WriteError(w, 401, Error{Code: "invalid_credentials", Message: "Invalid password or authentication code."})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// hashingRetrySeconds is how long a client waits when both password-hashing
// slots are busy. One Argon2id run takes well under a second, so one second
// is enough for a slot to free up without the UI feeling stuck.
const hashingRetrySeconds = 1

// writeHashingBusy answers 429 when the process-wide Argon2 budget is in use.
// It is deliberately distinct from rate_limited: the client did nothing wrong
// and may retry almost immediately.
func writeHashingBusy(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", hashingRetrySeconds))
	WriteError(w, http.StatusTooManyRequests, Error{Code: "server_busy", Message: "The server is busy. Try again in a moment.", Details: map[string]int{"retryAfterSeconds": hashingRetrySeconds}})
}
