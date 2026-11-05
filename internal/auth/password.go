// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Memory is expressed in KiB as the argon2 package wants
// it. Releases before the 32 MiB change wrote 64 MiB hashes; those keep
// verifying (passwordMemoryMax) but cost two budget units each.
const (
	passwordMemory    uint32 = 32 * 1024
	passwordMemoryMax uint64 = 64 * 1024
	passwordTime      uint32 = 3
	passwordKeySize   uint32 = 32

	// hashingUnits is the whole-process budget for concurrent Argon2 work, in
	// units of passwordMemory. Two units keep the peak at 64 MiB inside the
	// packaged 128 MiB container limit.
	hashingUnits = 2
)

// ErrHashingBusy reports that the hashing budget is fully in use. Callers
// answer 429 with a short Retry-After instead of queueing memory-heavy work.
var ErrHashingBusy = errors.New("password hashing is busy")

// hashBudget bounds how much Argon2 memory is live at once. A plain counter
// under a mutex lets a 64 MiB legacy hash take both units atomically.
type hashBudget struct {
	mu    sync.Mutex
	used  int
	peak  int
	limit int
}

var passwordHashing = &hashBudget{limit: hashingUnits}

func (b *hashBudget) acquire(units int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if units < 1 || b.used+units > b.limit {
		return false
	}
	b.used += units
	if b.used > b.peak {
		b.peak = b.used
	}
	return true
}

func (b *hashBudget) release(units int) {
	b.mu.Lock()
	b.used -= units
	b.mu.Unlock()
}

// hashingUnitsFor rounds a memory parameter up to whole budget units.
func hashingUnitsFor(memoryKiB uint64) int {
	unit := uint64(passwordMemory)
	return int((memoryKiB + unit - 1) / unit)
}

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
	units := hashingUnitsFor(uint64(passwordMemory))
	if !passwordHashing.acquire(units) {
		return "", ErrHashingBusy
	}
	key := argon2.IDKey([]byte(password), salt, passwordTime, passwordMemory, p, passwordKeySize)
	passwordHashing.release(units)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", passwordMemory, passwordTime, p,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the stored PHC value. A
// malformed or oversized hash is simply false; ErrHashingBusy means nothing was
// checked and the caller should ask the client to retry.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, nil
	}
	params := map[string]uint64{}
	for _, item := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return false, nil
		}
		value, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return false, nil
		}
		params[pair[0]] = value
	}
	m, t, p := params["m"], params["t"], params["p"]
	if m == 0 || m > passwordMemoryMax || t == 0 || t > 10 || p == 0 || p > 16 {
		return false, nil
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, nil
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 || len(expected) > 64 || len(salt) < 8 || len(salt) > 64 {
		return false, nil
	}
	units := hashingUnitsFor(m)
	if !passwordHashing.acquire(units) {
		return false, ErrHashingBusy
	}
	actual := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(expected)))
	passwordHashing.release(units)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
