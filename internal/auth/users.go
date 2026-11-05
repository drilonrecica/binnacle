// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

var (
	ErrAdminExists        = errors.New("local admin already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type User struct {
	ID        string
	Username  string
	CreatedAt time.Time
}

type Credentials struct{ db *storage.DB }

func NewCredentials(db *storage.DB) *Credentials { return &Credentials{db: db} }
func (c *Credentials) SetDB(db *storage.DB)      { c.db = db }

func (c *Credentials) CreateAdmin(ctx context.Context, username, password string) (User, error) {
	if c == nil || c.db == nil {
		return User{}, errors.New("credential repository is unavailable")
	}
	username, err := ValidateUsername(username)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return User{}, err
	}
	if count != 0 {
		return User{}, ErrAdminExists
	}
	id, err := randomID()
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES(?,?,?,?,?)", id, username, hash, now.UnixMilli(), now.UnixMilli()); err != nil {
		if isUnique(err) {
			return User{}, ErrAdminExists
		}
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, CreatedAt: now}, nil
}

func (c *Credentials) Authenticate(ctx context.Context, username, password string) (User, error) {
	if c == nil || c.db == nil {
		return User{}, ErrInvalidCredentials
	}
	username, err := ValidateUsername(username)
	if err != nil {
		return User{}, rejectWithDummyHash(password)
	}
	var u User
	var created int64
	var hash string
	err = c.db.QueryRowContext(ctx, "SELECT id,username,password_hash,created_at FROM users WHERE username=?", username).Scan(&u.ID, &u.Username, &hash, &created)
	if err != nil {
		return User{}, rejectWithDummyHash(password)
	}
	ok, err := VerifyPassword(hash, password)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	u.CreatedAt = time.UnixMilli(created).UTC()
	return u, nil
}

// rejectWithDummyHash keeps the missing-user path as expensive as a real
// verification, and as subject to the hashing budget, so neither timing nor
// the busy signal reveals whether a username exists.
func rejectWithDummyHash(password string) error {
	if _, err := VerifyPassword(dummyPasswordHash, password); err != nil {
		return err
	}
	return ErrInvalidCredentials
}

// Exists reports whether username names a stored account. Login throttling
// uses it so only real accounts get lockout buckets. Database errors read as
// "unknown": the network bucket still applies and Authenticate fails anyway.
func (c *Credentials) Exists(ctx context.Context, username string) bool {
	if c == nil || c.db == nil {
		return false
	}
	username, err := ValidateUsername(username)
	if err != nil {
		return false
	}
	var one int
	return c.db.QueryRowContext(ctx, "SELECT 1 FROM users WHERE username=?", username).Scan(&one) == nil
}

// ChangePassword replaces the stored hash after the current password verifies.
// The new password must already satisfy ValidatePassword; the caller reports
// policy failures before paying for a hash. ErrHashingBusy is passed through so
// the client can retry instead of being told the password was wrong.
func (c *Credentials) ChangePassword(ctx context.Context, userID, current, next string) error {
	if c == nil || c.db == nil || userID == "" {
		return ErrInvalidCredentials
	}
	var hash string
	if err := c.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=?", userID).Scan(&hash); err != nil {
		return ErrInvalidCredentials
	}
	ok, err := VerifyPassword(hash, current)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	replacement, err := HashPassword(next)
	if err != nil {
		return err
	}
	result, err := c.db.ExecContext(ctx, "UPDATE users SET password_hash=?,updated_at=? WHERE id=?", replacement, time.Now().UTC().UnixMilli(), userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

func (c *Credentials) UserByID(ctx context.Context, id string) (User, error) {
	if c == nil || c.db == nil || id == "" {
		return User{}, ErrInvalidCredentials
	}
	var user User
	var created int64
	if err := c.db.QueryRowContext(ctx, "SELECT id,username,created_at FROM users WHERE id=?", id).Scan(&user.ID, &user.Username, &created); err != nil {
		return User{}, ErrInvalidCredentials
	}
	user.CreatedAt = time.UnixMilli(created).UTC()
	return user, nil
}

func (c *Credentials) Administrator(ctx context.Context) (User, error) {
	if c == nil || c.db == nil {
		return User{}, ErrInvalidCredentials
	}
	var user User
	var created int64
	err := c.db.QueryRowContext(ctx, "SELECT id,username,created_at FROM users ORDER BY created_at LIMIT 1").Scan(&user.ID, &user.Username, &created)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	user.CreatedAt = time.UnixMilli(created).UTC()
	return user, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random id: %w", err)
	}
	return "usr_" + hex.EncodeToString(b), nil
}

// This valid hash is only used to equalize invalid-username and absent-user
// work. It uses the current parameters so the dummy path costs one budget unit.
const dummyPasswordHash = "$argon2id$v=19$m=32768,t=3,p=4$MDEyMzQ1Njc4OWFiY2RlZg$rEs+e+stt14QPJl1KJ4hsMevadn33y2bi6V4TbGJjIA"

func isUnique(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE") || contains(err.Error(), "unique"))
}
func contains(s, want string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
