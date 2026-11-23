// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

// ErrNoAdmin reports that the database has no administrator to recover.
var ErrNoAdmin = errors.New("no administrator exists")

// ResetResult describes the account after ResetAdminPassword.
type ResetResult struct {
	User User
	// MFAEnabled is whether TOTP is still required at login.
	MFAEnabled bool
}

// GeneratePassword returns a random 24-character password with 144 bits of
// entropy, in the URL-safe base64 alphabet so it survives copy and paste.
func GeneratePassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ResetAdminPassword replaces the administrator's password without knowing
// the current one and revokes every session of the account. It is the
// operator's way back in after a lost password, reached only through the
// reset-password command by someone who can already read and write the
// database file; no HTTP route calls it.
//
// With disableMFA it also removes the TOTP enrollment and recovery codes, for
// an operator who lost the authenticator as well. API tokens and preferences
// are kept either way. Everything happens in one transaction, so a failure
// leaves the account as it was.
func ResetAdminPassword(ctx context.Context, db *storage.DB, password string, disableMFA bool) (ResetResult, error) {
	if db == nil {
		return ResetResult{}, errors.New("credential repository is unavailable")
	}
	// The hash is computed before the transaction so Argon2 never runs while
	// the write lock is held against a running server.
	hash, err := HashPassword(password)
	if err != nil {
		return ResetResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ResetResult{}, err
	}
	defer tx.Rollback()
	var result ResetResult
	var created int64
	var key sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT id,username,created_at,totp_enabled,totp_secret_key FROM users ORDER BY created_at LIMIT 1").
		Scan(&result.User.ID, &result.User.Username, &created, &result.MFAEnabled, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return ResetResult{}, ErrNoAdmin
	}
	if err != nil {
		return ResetResult{}, err
	}
	result.User.CreatedAt = time.UnixMilli(created).UTC()
	now := time.Now().UTC().UnixMilli()
	if _, err = tx.ExecContext(ctx, "UPDATE users SET password_hash=?,updated_at=? WHERE id=?", hash, now, result.User.ID); err != nil {
		return ResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE user_id=?", now, result.User.ID); err != nil {
		return ResetResult{}, err
	}
	if disableMFA {
		// The same writes as MFA.Disable. The encrypted seed is deleted by
		// key, which needs no master key, so the reset also works when the
		// master key is the thing that was lost.
		if _, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", result.User.ID); err != nil {
			return ResetResult{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE users SET totp_enabled=0,totp_secret_key=NULL,totp_last_step=0,mfa_changed_at=? WHERE id=?", now, result.User.ID); err != nil {
			return ResetResult{}, err
		}
		if key.Valid {
			if _, err = tx.ExecContext(ctx, "DELETE FROM encrypted_secrets WHERE key=?", key.String); err != nil {
				return ResetResult{}, err
			}
		}
		result.MFAEnabled = false
	}
	if err = tx.Commit(); err != nil {
		return ResetResult{}, err
	}
	return result, nil
}
