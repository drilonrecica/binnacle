// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func testKeyBytes() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i*7 + 3)
	}
	return key
}

func TestMasterKeyAcceptsOnlyHexOrBase64Of32Bytes(t *testing.T) {
	raw := testKeyBytes()
	accepted := map[string]string{
		"hex":            hex.EncodeToString(raw),
		"upper hex":      strings.ToUpper(hex.EncodeToString(raw)),
		"std base64":     base64.StdEncoding.EncodeToString(raw),
		"raw std base64": base64.RawStdEncoding.EncodeToString(raw),
		"url base64":     base64.URLEncoding.EncodeToString(raw),
		"raw url base64": base64.RawURLEncoding.EncodeToString(raw),
	}
	for name, key := range accepted {
		store, err := NewSecretStore(nil, key)
		if err != nil || !store.Available() {
			t.Fatalf("%s: err=%v available=%v", name, err, store.Available())
		}
	}
	rejected := map[string]string{
		"raw 32-byte ascii":    "0123456789abcdef0123456789abcdef",
		"raw 32-byte digits":   "01234567890123456789012345678901",
		"63 hex characters":    hex.EncodeToString(raw)[:63],
		"hex of 31 bytes":      hex.EncodeToString(raw[:31]),
		"base64 of 31 bytes":   base64.StdEncoding.EncodeToString(raw[:31]),
		"base64 of 33 bytes":   base64.StdEncoding.EncodeToString(append(raw, 1)),
		"passphrase":           "correct horse battery staple",
		"whitespace around":    " " + hex.EncodeToString(raw) + " ",
		"hex with inner space": hex.EncodeToString(raw)[:32] + " " + hex.EncodeToString(raw)[33:],
	}
	for name, key := range rejected {
		store, err := NewSecretStore(nil, key)
		if err == nil || store != nil {
			t.Fatalf("%s: accepted %q", name, key)
		}
		if !strings.Contains(err.Error(), "64 hexadecimal characters") || !strings.Contains(err.Error(), "base64") {
			t.Fatalf("%s: unclear error %q", name, err)
		}
	}
}

func TestMasterKeyEncodingsShareKeyMaterial(t *testing.T) {
	ctx := context.Background()
	manager, _ := setupDatabase(t)
	raw := testKeyBytes()
	hexStore, err := NewSecretStore(manager.DB(), hex.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err = hexStore.Put(ctx, "integration.key", []byte("value")); err != nil {
		t.Fatal(err)
	}
	base64Store, err := NewSecretStore(manager.DB(), base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := base64Store.Get(ctx, "integration.key")
	if err != nil || !bytes.Equal(got, []byte("value")) {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestSecretGetNamesUnconfiguredKeyVersion(t *testing.T) {
	ctx := context.Background()
	manager, _ := setupDatabase(t)
	store, err := NewSecretStore(manager.DB(), hex.EncodeToString(testKeyBytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put(ctx, "integration.key", []byte("value")); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = manager.DB().QueryRowContext(ctx, "SELECT key_version FROM encrypted_secrets WHERE key=?", "integration.key").Scan(&version); err != nil || version != store.KeyVersion() {
		t.Fatalf("stored version=%d store version=%d err=%v", version, store.KeyVersion(), err)
	}
	if _, err = manager.DB().ExecContext(ctx, "UPDATE encrypted_secrets SET key_version=2 WHERE key=?", "integration.key"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ctx, "integration.key")
	if err == nil || !strings.Contains(err.Error(), "key version 2") {
		t.Fatalf("err=%v, want the missing key version named", err)
	}
}
