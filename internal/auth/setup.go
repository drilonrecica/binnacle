// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

var (
	ErrSetupUnavailable = errors.New("setup is unavailable")
	ErrSetupToken       = errors.New("setup token is invalid")
)

type SetupService struct {
	db        *storage.DB
	now       func() time.Time
	tokenTTL  time.Duration
	tokenFile string
	// Logger receives cleanup failures; nil falls back to slog.Default().
	Logger *slog.Logger
}

// SetupTokenFileName is the file inside the runtime directory that holds a
// generated loopback setup token while first-run setup is open.
const SetupTokenFileName = "setup-token"

func NewSetupService(db *storage.DB) *SetupService {
	return &SetupService{db: db, now: func() time.Time { return time.Now().UTC() }, tokenTTL: 24 * time.Hour}
}

// SetupTokenFromEnvironment reads the operator-provided setup token. A file
// source is intended for Docker secrets and takes precedence only when the
// direct value is absent; configuring both is rejected to avoid ambiguity.
func SetupTokenFromEnvironment() (string, error) {
	return EnvironmentSecret("BINNACLE_SETUP_TOKEN")
}
func (s *SetupService) SetDB(db *storage.DB) { s.db = db }

// SetTokenFile sets where a generated loopback token is written. The file is
// created 0600 and removed once setup is claimed or disabled.
func (s *SetupService) SetTokenFile(path string) { s.tokenFile = path }

// TokenFile reports where a generated loopback token is written.
func (s *SetupService) TokenFile() string { return s.tokenFile }

// Disable permanently records that first-run setup has completed. It is used
// by non-browser bootstrap paths as well as the browser claim transaction.
func (s *SetupService) Disable(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrSetupUnavailable
	}
	now := s.now().UTC().UnixMilli()
	_, err := s.db.ExecContext(ctx, `INSERT INTO setup_state(id,token_hash,expires_at,claimed_at,created_at)
		VALUES(1,NULL,NULL,?,?) ON CONFLICT(id) DO UPDATE SET token_hash=NULL,claimed_at=COALESCE(setup_state.claimed_at,excluded.claimed_at)`, now, now)
	if err != nil {
		return err
	}
	return s.removeTokenFile()
}

// Initialize arms first-run setup while no user exists. A public listener
// must be supplied an operator token out of band; a loopback-only listener
// without one gets a generated token written to the token file. While setup is
// unclaimed and no user exists, every start re-arms it: the configured token
// gets a fresh expiry and a loopback install gets a fresh generated token, so
// an expired unclaimed token never blocks setup permanently. The returned
// value is the generated token, empty when the operator supplied one or setup
// is closed.
func (s *SetupService) Initialize(ctx context.Context, listenAddress, configuredToken string) (generated string, err error) {
	if s == nil || s.db == nil {
		return "", ErrSetupUnavailable
	}
	var users int
	if err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		return "", err
	}
	if users > 0 {
		return "", s.removeTokenFile()
	}
	var claimed sql.NullInt64
	var expires int64
	exists := true
	err = s.db.QueryRowContext(ctx, "SELECT claimed_at,COALESCE(expires_at,0) FROM setup_state WHERE id=1").Scan(&claimed, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		exists = false
	} else if err != nil {
		return "", err
	}
	if exists && claimed.Valid {
		return "", s.removeTokenFile()
	}
	now := s.now().UTC()
	token := strings.TrimSpace(configuredToken)
	if token == "" {
		if publicListener(listenAddress) {
			if exists && now.UnixMilli() < expires {
				// A token supplied on an earlier start is still valid.
				return "", nil
			}
			return "", fmt.Errorf("public setup requires BINNACLE_SETUP_TOKEN")
		}
		if s.tokenFile == "" {
			return "", fmt.Errorf("loopback setup requires a token file location")
		}
		if token, err = randomSetupToken(); err != nil {
			return "", err
		}
		generated = token
	}
	if len(token) < 32 {
		return "", fmt.Errorf("setup token must be at least 32 characters")
	}
	if generated == "" {
		if err = s.removeTokenFile(); err != nil {
			return "", err
		}
	} else if err = writeTokenFile(s.tokenFile, token); err != nil {
		return "", fmt.Errorf("write setup token file: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO setup_state(id,token_hash,expires_at,created_at) VALUES(1,?,?,?)
		ON CONFLICT(id) DO UPDATE SET token_hash=excluded.token_hash,expires_at=excluded.expires_at WHERE setup_state.claimed_at IS NULL`,
		setupHash(token), now.Add(s.tokenTTL).UnixMilli(), now.UnixMilli())
	if err != nil {
		return "", err
	}
	return generated, nil
}

// writeTokenFile replaces path atomically with a file readable only by the
// Binnacle user. The temporary file is created 0600 with O_EXCL so the token
// is never exposed through a wider mode, whatever the umask.
func writeTokenFile(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(token + "\n"); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return os.Rename(tmp, path)
}

func (s *SetupService) removeTokenFile() error {
	if s.tokenFile == "" {
		return nil
	}
	if err := os.Remove(s.tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove setup token file: %w", err)
	}
	return nil
}

func (s *SetupService) Available(ctx context.Context) bool {
	if s == nil || s.db == nil {
		return false
	}
	var token string
	var expires int64
	var claimed sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(token_hash,''),COALESCE(expires_at,0),claimed_at FROM setup_state WHERE id=1").Scan(&token, &expires, &claimed); err != nil {
		return false
	}
	return token != "" && !claimed.Valid && s.now().UTC().UnixMilli() < expires
}

func (s *SetupService) Verify(ctx context.Context, token string) error {
	if s == nil || s.db == nil {
		return ErrSetupUnavailable
	}
	var expected string
	var expires int64
	var claimed sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(token_hash,''),COALESCE(expires_at,0),claimed_at FROM setup_state WHERE id=1").Scan(&expected, &expires, &claimed); err != nil {
		return ErrSetupUnavailable
	}
	if expected == "" || claimed.Valid || s.now().UTC().UnixMilli() >= expires || subtle.ConstantTimeCompare([]byte(expected), []byte(setupHash(token))) != 1 {
		return ErrSetupToken
	}
	return nil
}

// Claim creates the administrator. Availability and the token are checked
// before any Argon2 work so an unauthenticated caller cannot spend hashing
// memory; the transaction re-checks both so concurrent claims stay exclusive.
func (s *SetupService) Claim(ctx context.Context, token, username, password string) (User, error) {
	if s == nil || s.db == nil {
		return User{}, ErrSetupUnavailable
	}
	username, err := ValidateUsername(username)
	if err != nil {
		return User{}, err
	}
	if err = ValidatePassword(password); err != nil {
		return User{}, err
	}
	if err = s.Verify(ctx, token); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var expected string
	var expires int64
	var claimed sql.NullInt64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(token_hash,''),COALESCE(expires_at,0),claimed_at FROM setup_state WHERE id=1").Scan(&expected, &expires, &claimed); err != nil {
		return User{}, ErrSetupUnavailable
	}
	now := s.now().UTC()
	if expected == "" || claimed.Valid || now.UnixMilli() >= expires || subtle.ConstantTimeCompare([]byte(expected), []byte(setupHash(token))) != 1 {
		return User{}, ErrSetupToken
	}
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
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES(?,?,?,?,?)", id, username, hash, now.UnixMilli(), now.UnixMilli()); err != nil {
		return User{}, err
	}
	r, err := tx.ExecContext(ctx, "UPDATE setup_state SET claimed_at=?,token_hash=NULL WHERE id=1 AND claimed_at IS NULL", now.UnixMilli())
	if err != nil {
		return User{}, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if n != 1 {
		return User{}, ErrSetupUnavailable
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	// The token is spent. A file that cannot be removed now is removed on the
	// next start (a user exists by then), so the claim itself still succeeds.
	if err := s.removeTokenFile(); err != nil {
		fallbackLogger(s.Logger).Warn("spent setup token file was not removed; it is removed at the next start", "error", err)
	}
	return User{ID: id, Username: username, CreatedAt: now}, nil
}

func publicListener(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return true
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}
func randomSetupToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func setupHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
