// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	TOTPPeriod            = 30 * time.Second
	pendingEnrollmentTTL  = 10 * time.Minute
	maxPendingEnrollments = 64
)

var ErrMFAInvalid = errors.New("authentication code is invalid")

type Enrollment struct {
	Seed      string    `json:"seed"`
	URI       string    `json:"uri"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type pendingEnrollment struct {
	seed    []byte
	expires time.Time
}
type MFA struct {
	db          *sql.DB
	credentials *Credentials
	secrets     *SecretStore
	sessions    *Sessions
	now         func() time.Time
	mu          sync.Mutex
	pending     map[string]pendingEnrollment
}

func NewMFA(db *sql.DB, credentials *Credentials, secrets *SecretStore, sessions *Sessions) *MFA {
	return &MFA{db: db, credentials: credentials, secrets: secrets, sessions: sessions, now: func() time.Time { return time.Now().UTC() }, pending: map[string]pendingEnrollment{}}
}
func (m *MFA) SetDB(db *sql.DB) { m.db = db }
func (m *MFA) Enabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := m.db.QueryRowContext(ctx, "SELECT totp_enabled FROM users WHERE id=?", userID).Scan(&enabled)
	return enabled, err
}
func (m *MFA) Begin(ctx context.Context, user User, password string) (Enrollment, error) {
	if !m.secrets.Available() {
		return Enrollment{}, ErrMasterKeyMissing
	}
	if _, err := m.credentials.Authenticate(ctx, user.Username, password); err != nil {
		return Enrollment{}, ErrInvalidCredentials
	}
	seed := make([]byte, 20)
	if _, err := rand.Read(seed); err != nil {
		return Enrollment{}, err
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, value := range m.pending {
		if !now.Before(value.expires) {
			delete(m.pending, id)
		}
	}
	if _, ok := m.pending[user.ID]; !ok && len(m.pending) >= maxPendingEnrollments {
		return Enrollment{}, errors.New("too many pending MFA enrollments")
	}
	expires := now.Add(pendingEnrollmentTTL)
	m.pending[user.ID] = pendingEnrollment{seed: seed, expires: expires}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(seed)
	label := "Binnacle:" + user.Username
	values := url.Values{"secret": {encoded}, "issuer": {"Binnacle"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return Enrollment{Seed: encoded, URI: "otpauth://totp/" + url.PathEscape(label) + "?" + values.Encode(), ExpiresAt: expires}, nil
}
func (m *MFA) Confirm(ctx context.Context, userID, code string) ([]string, error) {
	m.mu.Lock()
	pending, ok := m.pending[userID]
	if !ok || !m.now().Before(pending.expires) {
		ok = false
		delete(m.pending, userID)
	}
	m.mu.Unlock()
	if !ok {
		return nil, ErrMFAInvalid
	}
	if !VerifyTOTP(pending.seed, code, m.now()) {
		return nil, ErrMFAInvalid
	}
	m.mu.Lock()
	current, stillPending := m.pending[userID]
	if !stillPending || !bytes.Equal(current.seed, pending.seed) || !m.now().Before(current.expires) {
		m.mu.Unlock()
		return nil, ErrMFAInvalid
	}
	delete(m.pending, userID)
	m.mu.Unlock()
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	key := "totp." + userID
	if err = m.secrets.Put(ctx, key, pending.seed); err != nil {
		return nil, err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		_ = m.secrets.Delete(ctx, key)
		return nil, err
	}
	defer tx.Rollback()
	now := m.now().UnixMilli()
	if _, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", userID); err == nil {
		for _, hash := range hashes {
			if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_codes(user_id,code_hash,created_at) VALUES(?,?,?)", userID, hash, now); err != nil {
				break
			}
		}
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE users SET totp_enabled=1,totp_secret_key=?,mfa_changed_at=?,updated_at=? WHERE id=?", key, now, now, userID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = m.secrets.Delete(ctx, key)
		return nil, err
	}
	_ = m.sessions.RevokeAll(ctx, userID)
	return codes, nil
}
func (m *MFA) Verify(ctx context.Context, userID, code string) error {
	enabled, err := m.Enabled(ctx, userID)
	if err != nil {
		return ErrMFAInvalid
	}
	if !enabled {
		return nil
	}
	var key string
	if err = m.db.QueryRowContext(ctx, "SELECT COALESCE(totp_secret_key,'') FROM users WHERE id=?", userID).Scan(&key); err != nil || key == "" {
		return ErrMFAInvalid
	}
	seed, err := m.secrets.Get(ctx, key)
	if err == nil && VerifyTOTP(seed, code, m.now()) {
		return nil
	}
	hash := recoveryHash(code)
	if hash == "" {
		return ErrMFAInvalid
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrMFAInvalid
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE recovery_codes SET used_at=? WHERE user_id=? AND code_hash=? AND used_at IS NULL", m.now().UnixMilli(), userID, hash)
	if err != nil {
		return ErrMFAInvalid
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrMFAInvalid
	}
	if tx.Commit() != nil {
		return ErrMFAInvalid
	}
	return nil
}
func (m *MFA) Disable(ctx context.Context, user User, password, code string) error {
	if _, err := m.credentials.Authenticate(ctx, user.Username, password); err != nil {
		return ErrInvalidCredentials
	}
	if err := m.Verify(ctx, user.ID, code); err != nil {
		return ErrMFAInvalid
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var key sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT totp_secret_key FROM users WHERE id=?", user.ID).Scan(&key); err != nil {
		return err
	}
	now := m.now().UnixMilli()
	if _, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", user.ID); err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE users SET totp_enabled=0,totp_secret_key=NULL,mfa_changed_at=?,updated_at=? WHERE id=?", now, now, user.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return err
	}
	if key.Valid {
		_ = m.secrets.Delete(ctx, key.String)
	}
	_ = m.sessions.RevokeAll(ctx, user.ID)
	return nil
}

func TOTP(seed []byte, at time.Time) string {
	counter := uint64(at.Unix() / 30)
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)
	mac := hmac.New(sha1.New, seed)
	_, _ = mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000)
}
func VerifyTOTP(seed []byte, code string, at time.Time) bool {
	if len(code) != 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	for step := -1; step <= 1; step++ {
		expected := TOTP(seed, at.Add(time.Duration(step)*TOTPPeriod))
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}
func newRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 10)
	hashes := make([]string, 10)
	for i := range codes {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		codes[i] = encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:]
		hashes[i] = recoveryHash(codes[i])
	}
	return codes, hashes, nil
}
func recoveryHash(code string) string {
	normalized := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	if len(normalized) != 16 {
		return ""
	}
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized); err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
