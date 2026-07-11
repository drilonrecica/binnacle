// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemory  uint32 = 64 * 1024
	passwordTime    uint32 = 3
	passwordKeySize uint32 = 32
)

// ValidateUsername keeps the one local identity deliberately unsurprising.
func ValidateUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) < 3 || len(username) > 32 {
		return "", fmt.Errorf("username must be 3 to 32 characters")
	}
	for i, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			if i == 0 && !(r >= 'a' && r <= 'z') {
				return "", fmt.Errorf("username must start with a letter")
			}
			continue
		}
		return "", fmt.Errorf("username contains unsupported characters")
	}
	return username, nil
}

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) > 1024 {
		return fmt.Errorf("password is invalid")
	}
	if n := utf8.RuneCountInString(password); n < 12 || n > 128 {
		return fmt.Errorf("password must be 12 to 128 characters")
	}
	return nil
}

func parallelism() uint8 {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return uint8(n)
}

// HashPassword returns a self-describing, versioned Argon2id PHC value.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	p := parallelism()
	key := argon2.IDKey([]byte(password), salt, passwordTime, passwordMemory, p, passwordKeySize)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", passwordMemory, passwordTime, p,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	params := map[string]uint64{}
	for _, item := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return false
		}
		value, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return false
		}
		params[pair[0]] = value
	}
	m, t, p := params["m"], params["t"], params["p"]
	if m == 0 || t == 0 || p == 0 || p > 255 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
