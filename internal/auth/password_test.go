// SPDX-License-Identifier: AGPL-3.0-only
package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("invalid password accepted")
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
