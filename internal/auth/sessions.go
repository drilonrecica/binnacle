// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

const SessionCookieName = "binnacle_session"

var ErrSessionInvalid = errors.New("session is invalid")

// Session lifetime ceilings (SEC-19). Configuration above them is refused by
// settings validation and clamped here as well, so no code path can issue a
// longer session. Proxy sessions are shorter because the proxy's identity
// assertion is checked only when the session is bootstrapped; a day bounds how
// long a revoked upstream identity keeps working.
const (
	MaxSessionIdleTimeout      = 7 * 24 * time.Hour
	MaxSessionAbsoluteLifetime = 90 * 24 * time.Hour
	ProxySessionLifetime       = 24 * time.Hour
)

// SessionTouchInterval is how often an active session's last_seen_at and idle
// expiry are written (STO-4). Requests inside the interval reuse the stored
// row, so a polling tab costs one write a minute instead of one per request;
// the idle expiry then lags real activity by at most the interval.
const SessionTouchInterval = time.Minute

type SessionConfig struct {
	IdleTimeout      time.Duration
	AbsoluteLifetime time.Duration
}

func (c SessionConfig) valid() bool { return c.IdleTimeout > 0 && c.AbsoluteLifetime >= c.IdleTimeout }

// capped returns the configuration with both lifetimes held under the ceilings.
func (c SessionConfig) capped() SessionConfig {
	c.IdleTimeout = min(c.IdleTimeout, MaxSessionIdleTimeout)
	c.AbsoluteLifetime = min(c.AbsoluteLifetime, MaxSessionAbsoluteLifetime)
	return c
}

type Session struct {
	UserID          string
	CreatedAt       time.Time
	LastSeenAt      time.Time
	ExpiresAt       time.Time
	AbsoluteExpires time.Time
	AuthMethod      string
	AuthSubject     string
}

type Sessions struct {
	db      *storage.DB
	now     func() time.Time
	mu      sync.RWMutex
	cfg     SessionConfig
	proxies TrustedProxies
}

func NewSessions(db *storage.DB, cfg SessionConfig) *Sessions {
	return &Sessions{db: db, cfg: cfg, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Sessions) SetDB(db *storage.DB)                     { s.db = db }
func (s *Sessions) SetTrustedProxies(proxies TrustedProxies) { s.proxies = proxies }
func (s *Sessions) SetConfig(cfg SessionConfig) {
	if cfg.valid() {
		s.mu.Lock()
		s.cfg = cfg
		s.mu.Unlock()
	}
}
func (s *Sessions) config() SessionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.capped()
}

func (s *Sessions) Issue(ctx context.Context, userID string) (string, Session, error) {
	token, _, session, err := s.issue(ctx, userID, "local", "")
	return token, session, err
}

// IssueWithCSRF returns the only plaintext copy of the anti-CSRF token.
func (s *Sessions) IssueWithCSRF(ctx context.Context, userID string) (string, string, Session, error) {
	return s.issue(ctx, userID, "local", "")
}

// IssueForProxy records the proxy-asserted subject the session was mapped
// from and bounds the session by ProxySessionLifetime.
func (s *Sessions) IssueForProxy(ctx context.Context, userID, subject string) (string, string, Session, error) {
	return s.issue(ctx, userID, "proxy", subject)
}
func (s *Sessions) issue(ctx context.Context, userID, authMethod, authSubject string) (string, string, Session, error) {
	if s == nil || s.db == nil || userID == "" {
		return "", "", Session{}, ErrSessionInvalid
	}
	cfg := s.config()
	if !cfg.valid() {
		return "", "", Session{}, ErrSessionInvalid
	}
	token, err := randomToken()
	if err != nil {
		return "", "", Session{}, err
	}
	csrf, err := NewCSRFToken()
	if err != nil {
		return "", "", Session{}, err
	}
	now := s.now().UTC()
	lifetime := cfg.AbsoluteLifetime
	switch authMethod {
	case "local":
	case "proxy":
		lifetime = min(lifetime, ProxySessionLifetime)
	default:
		return "", "", Session{}, ErrSessionInvalid
	}
	absolute := now.Add(lifetime)
	session := Session{UserID: userID, CreatedAt: now, LastSeenAt: now, ExpiresAt: minTime(now.Add(cfg.IdleTimeout), absolute), AbsoluteExpires: absolute, AuthMethod: authMethod, AuthSubject: authSubject}
	if _, err = s.db.ExecContext(ctx, "INSERT INTO sessions(id_hash,user_id,created_at,last_seen_at,expires_at,absolute_expires_at,revoked_at,csrf_hash,auth_method,auth_subject) VALUES(?,?,?,?,?,?,NULL,?,?,?)", tokenHash(token), userID, now.UnixMilli(), now.UnixMilli(), session.ExpiresAt.UnixMilli(), absolute.UnixMilli(), CSRFHash(csrf), authMethod, nullString(authSubject)); err != nil {
		return "", "", Session{}, err
	}
	return token, csrf, session, nil
}

// Authorize satisfies API authentication hooks without exposing session tokens.
func (s *Sessions) Authorize(r *http.Request) bool {
	_, err := s.Authenticate(r.Context(), TokenFromRequest(r))
	return err == nil
}

// Recheck reports whether the request's session is still valid without
// counting the check as activity. Open streams call it on every heartbeat so a
// logout, an MFA change or an expiry closes them (SEC-12), while a tab left
// open still idles out.
func (s *Sessions) Recheck(r *http.Request) bool {
	return s.Check(r.Context(), TokenFromRequest(r)) == nil
}
func (s *Sessions) Actor(r *http.Request) (string, bool) {
	var id, method string
	var subject sql.NullString
	err := s.db.QueryRowContext(r.Context(), "SELECT user_id,auth_method,auth_subject FROM sessions WHERE id_hash=? AND revoked_at IS NULL", tokenHash(TokenFromRequest(r))).Scan(&id, &method, &subject)
	if err != nil {
		return "", false
	}
	if method == "proxy" && subject.Valid {
		return "proxy:" + subject.String, true
	}
	return id, true
}

func (s *Sessions) ValidCSRF(r *http.Request) bool {
	if !SameOrigin(r, s.proxies) {
		return false
	}
	token := TokenFromRequest(r)
	if token == "" || s == nil || s.db == nil {
		return false
	}
	var expected string
	if err := s.db.QueryRowContext(r.Context(), "SELECT COALESCE(csrf_hash,'') FROM sessions WHERE id_hash=? AND revoked_at IS NULL", tokenHash(token)).Scan(&expected); err != nil {
		return false
	}
	return expected != "" && ValidCSRF(r, expected)
}

// Authenticate validates a session token and counts the call as activity,
// sliding the idle expiry forward. The slide is written at most once per
// touch interval; inside it the stored session is returned unchanged.
func (s *Sessions) Authenticate(ctx context.Context, token string) (Session, error) {
	cfg := s.config()
	if !cfg.valid() {
		return Session{}, ErrSessionInvalid
	}
	session, hash, err := s.load(ctx, token)
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	if now.Sub(session.LastSeenAt) < touchInterval(cfg) {
		return session, nil
	}
	session.LastSeenAt = now
	session.ExpiresAt = minTime(now.Add(cfg.IdleTimeout), session.AbsoluteExpires)
	if _, err = s.db.ExecContext(ctx, "UPDATE sessions SET last_seen_at=?,expires_at=? WHERE id_hash=? AND revoked_at IS NULL", now.UnixMilli(), session.ExpiresAt.UnixMilli(), hash); err != nil {
		return Session{}, err
	}
	return session, nil
}

// touchInterval is SessionTouchInterval, shortened to half the idle timeout so
// a short configured timeout cannot expire a session that is in use.
func touchInterval(cfg SessionConfig) time.Duration {
	return min(SessionTouchInterval, cfg.IdleTimeout/2)
}

// Check validates a session token without touching its idle expiry.
func (s *Sessions) Check(ctx context.Context, token string) error {
	_, _, err := s.load(ctx, token)
	return err
}

// load reads a session and rejects revoked or expired ones. An expired row is
// marked revoked so later lookups are cheap and Cleanup removes it.
func (s *Sessions) load(ctx context.Context, token string) (Session, string, error) {
	if s == nil || s.db == nil || token == "" {
		return Session{}, "", ErrSessionInvalid
	}
	hash := tokenHash(token)
	var session Session
	var created, seen, expires, absolute int64
	var revoked sql.NullInt64
	var subject sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT user_id,created_at,last_seen_at,expires_at,absolute_expires_at,revoked_at,auth_method,auth_subject FROM sessions WHERE id_hash=?", hash).Scan(&session.UserID, &created, &seen, &expires, &absolute, &revoked, &session.AuthMethod, &subject)
	if err != nil || revoked.Valid {
		return Session{}, "", ErrSessionInvalid
	}
	session.CreatedAt, session.LastSeenAt = time.UnixMilli(created).UTC(), time.UnixMilli(seen).UTC()
	session.ExpiresAt, session.AbsoluteExpires = time.UnixMilli(expires).UTC(), time.UnixMilli(absolute).UTC()
	if subject.Valid {
		session.AuthSubject = subject.String
	}
	now := s.now().UTC()
	if !now.Before(session.ExpiresAt) || !now.Before(session.AbsoluteExpires) {
		_, _ = s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE id_hash=?", now.UnixMilli(), hash)
		return Session{}, "", ErrSessionInvalid
	}
	return session, hash, nil
}

func (s *Sessions) Revoke(ctx context.Context, token string) error {
	if s == nil || s.db == nil || token == "" {
		return ErrSessionInvalid
	}
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE id_hash=?", s.now().UTC().UnixMilli(), tokenHash(token))
	return err
}

func (s *Sessions) RevokeAll(ctx context.Context, userID string) error {
	if s == nil || s.db == nil || userID == "" {
		return ErrSessionInvalid
	}
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE user_id=?", s.now().UTC().UnixMilli(), userID)
	return err
}

// RevokeOthers ends every session of the user except the one identified by
// token, so a password change signs out other devices and keeps the caller in.
func (s *Sessions) RevokeOthers(ctx context.Context, userID, token string) error {
	if s == nil || s.db == nil || userID == "" || token == "" {
		return ErrSessionInvalid
	}
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE user_id=? AND id_hash<>?", s.now().UTC().UnixMilli(), userID, tokenHash(token))
	return err
}

func (s *Sessions) Cleanup(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrSessionInvalid
	}
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	cutoff := s.now().UTC().UnixMilli()
	r, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE rowid IN (SELECT rowid FROM sessions WHERE expires_at<? OR absolute_expires_at<? OR (revoked_at IS NOT NULL AND revoked_at<?) LIMIT ?)", cutoff, cutoff, cutoff, limit)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

func (s *Sessions) Start(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for {
					n, err := s.Cleanup(ctx, 500)
					if err != nil || n < 500 {
						break
					}
				}
			}
		}
	}()
	return nil
}
func (s *Sessions) Stop(context.Context) error { return nil }

func SetSessionCookie(w http.ResponseWriter, token string, secure bool, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expires.UTC()})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func TokenFromRequest(r *http.Request) string {
	// Bearer credentials and browser sessions are deliberately exclusive. A
	// caller that supplies Authorization must never fall back to ambient cookies.
	if r.Header.Get("Authorization") != "" {
		return ""
	}
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
