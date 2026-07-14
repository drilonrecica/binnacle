// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	SecretAlgorithm  = "AES-256-GCM"
	SecretKeyVersion = 1
)

var (
	ErrMasterKeyMissing = errors.New("master encryption key is not configured")
	ErrSecretNotFound   = errors.New("encrypted secret was not found")
)

type SecretStatus struct {
	Configured bool   `json:"configured"`
	Algorithm  string `json:"algorithm,omitempty"`
	KeyVersion int    `json:"keyVersion,omitempty"`
}

type SecretStore struct {
	db   *sql.DB
	aead cipher.AEAD
}

func (s *SecretStore) Available() bool { return s != nil && s.aead != nil }

func (s *SecretStore) SetDB(db *sql.DB) { s.db = db }

func NewSecretStore(db *sql.DB, encodedKey string) (*SecretStore, error) {
	if encodedKey == "" {
		return &SecretStore{db: db}, nil
	}
	key, err := decodeMasterKey(encodedKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretStore{db: db, aead: aead}, nil
}

func decodeMasterKey(value string) ([]byte, error) {
	if len(value) == 32 {
		return []byte(value), nil
	}
	if len(value) == 64 {
		if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	return nil, errors.New("master encryption key must encode exactly 32 bytes")
}

func (s *SecretStore) Put(ctx context.Context, key string, plaintext []byte) error {
	if s == nil || s.db == nil || s.aead == nil {
		return ErrMasterKeyMissing
	}
	if key == "" || len(plaintext) == 0 {
		return errors.New("secret key and value are required")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate secret nonce: %w", err)
	}
	ciphertext := s.aead.Seal(nil, nonce, plaintext, []byte(key))
	_, err := s.db.ExecContext(ctx, `INSERT INTO encrypted_secrets(key,ciphertext,nonce,algorithm,key_version,updated_at)
		VALUES(?,?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET ciphertext=excluded.ciphertext,nonce=excluded.nonce,algorithm=excluded.algorithm,key_version=excluded.key_version,updated_at=excluded.updated_at`,
		key, ciphertext, nonce, SecretAlgorithm, SecretKeyVersion, time.Now().UTC().UnixMilli())
	return err
}

func (s *SecretStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.db == nil || s.aead == nil {
		return nil, ErrMasterKeyMissing
	}
	var ciphertext, nonce []byte
	var algorithm string
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT ciphertext,nonce,algorithm,key_version FROM encrypted_secrets WHERE key=?", key).Scan(&ciphertext, &nonce, &algorithm, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	if algorithm != SecretAlgorithm || version != SecretKeyVersion {
		return nil, errors.New("encrypted secret uses an unsupported algorithm or key version")
	}
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(key))
	if err != nil {
		return nil, errors.New("encrypted secret could not be decrypted")
	}
	return plaintext, nil
}

func (s *SecretStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.db == nil {
		return errors.New("secret repository is unavailable")
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM encrypted_secrets WHERE key=?", key)
	return err
}

func (s *SecretStore) Status(ctx context.Context, key string) (SecretStatus, error) {
	if s == nil || s.db == nil {
		return SecretStatus{}, errors.New("secret repository is unavailable")
	}
	var status SecretStatus
	err := s.db.QueryRowContext(ctx, "SELECT 1,algorithm,key_version FROM encrypted_secrets WHERE key=?", key).Scan(&status.Configured, &status.Algorithm, &status.KeyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return SecretStatus{}, nil
	}
	return status, err
}
