// SPDX-License-Identifier: AGPL-3.0-only
package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(hash, "correct horse battery staple"); err != nil || !ok {
		t.Fatalf("valid password ok=%v err=%v", ok, err)
	}
	if ok, err := VerifyPassword(hash, "wrong password"); err != nil || ok {
		t.Fatalf("invalid password ok=%v err=%v", ok, err)
	}
}

func TestCredentialValidation(t *testing.T) {
	if _, err := ValidateUsername("Admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUsername("1admin"); err == nil {
		t.Fatal("invalid username accepted")
	}
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
}
