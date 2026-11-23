// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const resetTestOldPassword = "correct horse battery staple"

// resetTestDatabase creates a migrated database with one admin in a fresh
// data directory, points the configuration at it and returns its path. The
// manager is closed again, as if the server were stopped.
func resetTestDatabase(t *testing.T, withAdmin bool) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("BINNACLE_DATA_DIR", dir)
	path := filepath.Join(dir, "binnacle.db")
	manager := storage.New(path, filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if withAdmin {
		if _, err := auth.NewCredentials(manager.DB()).CreateAdmin(ctx, "skipper", resetTestOldPassword); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// authenticate reports whether the password works against the database.
func authenticate(t *testing.T, path, password string) error {
	t.Helper()
	ctx := context.Background()
	db, closeDB, err := storage.OpenExisting(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB()
	_, err = auth.NewCredentials(db).Authenticate(ctx, "skipper", password)
	return err
}

func TestResetPasswordGeneratesAndPrintsAPassword(t *testing.T) {
	path := resetTestDatabase(t, true)
	var stdout, stderr bytes.Buffer
	if code := runResetPassword(nil, strings.NewReader(""), &stdout, &stderr, t.TempDir()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Administrator: skipper\n") {
		t.Fatalf("username missing from output:\n%s", out)
	}
	match := regexp.MustCompile(`(?m)^New password:  (\S{24})$`).FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("generated password missing from output:\n%s", out)
	}
	if err := authenticate(t, path, match[1]); err != nil {
		t.Fatalf("printed password does not work: %v", err)
	}
	if err := authenticate(t, path, resetTestOldPassword); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old password err=%v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%s", stderr.String())
	}
}

func TestResetPasswordFromStandardInput(t *testing.T) {
	path := resetTestDatabase(t, true)
	const chosen = "  spaces stay significant  "
	var stdout, stderr bytes.Buffer
	if code := runResetPassword([]string{"--password-stdin"}, strings.NewReader(chosen+"\r\n"), &stdout, &stderr, t.TempDir()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), chosen) {
		t.Fatal("a password given on standard input was echoed")
	}
	if err := authenticate(t, path, chosen); err != nil {
		t.Fatalf("chosen password does not work: %v", err)
	}
}

func TestResetPasswordFailures(t *testing.T) {
	cases := map[string]struct {
		args      []string
		stdin     string
		withAdmin bool
		want      int
		message   string
	}{
		"short password":     {args: []string{"--password-stdin"}, stdin: "short\n", withAdmin: true, want: 1, message: "12 to 128 characters"},
		"two lines":          {args: []string{"--password-stdin"}, stdin: "first line of password\nsecond line\n", withAdmin: true, want: 1, message: "single line"},
		"no administrator":   {withAdmin: false, want: 1, message: "first-run setup"},
		"unknown flag":       {args: []string{"--nope"}, withAdmin: true, want: 2, message: "Usage: binnacle reset-password"},
		"stray argument":     {args: []string{"admin"}, withAdmin: true, want: 2, message: "unexpected argument"},
		"oversized password": {args: []string{"--password-stdin"}, stdin: strings.Repeat("a", maxPasswordInput+1), withAdmin: true, want: 1, message: "too long"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := resetTestDatabase(t, tc.withAdmin)
			var stdout, stderr bytes.Buffer
			if code := runResetPassword(tc.args, strings.NewReader(tc.stdin), &stdout, &stderr, t.TempDir()); code != tc.want {
				t.Fatalf("exit=%d want %d, stderr=%s", code, tc.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.message) {
				t.Fatalf("stderr=%q, want it to mention %q", stderr.String(), tc.message)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout=%q", stdout.String())
			}
			if tc.withAdmin {
				if err := authenticate(t, path, resetTestOldPassword); err != nil {
					t.Fatalf("a failed reset changed the password: %v", err)
				}
			}
		})
	}
}

func TestResetPasswordMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BINNACLE_DATA_DIR", dir)
	var stdout, stderr bytes.Buffer
	if code := runResetPassword(nil, strings.NewReader(""), &stdout, &stderr, t.TempDir()); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "binnacle.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the command created a database: %v", err)
	}
}

// shortenHeadroomWait makes the memory guard give up quickly for the test.
func shortenHeadroomWait(t *testing.T, wait, poll time.Duration) {
	t.Helper()
	oldWait, oldPoll := resetHeadroomWait, resetHeadroomPoll
	resetHeadroomWait, resetHeadroomPoll = wait, poll
	t.Cleanup(func() { resetHeadroomWait, resetHeadroomPoll = oldWait, oldPoll })
}

func TestResetPasswordWaitsForMemoryTheServerReleases(t *testing.T) {
	shortenHeadroomWait(t, 30*time.Second, 10*time.Millisecond)
	path := resetTestDatabase(t, true)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("134217728\n"), 0600); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "memory.current")
	if err := os.WriteFile(current, []byte("100663296\n"), 0600); err != nil { // 96 MiB used
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		released <- os.WriteFile(current, []byte("52428800\n"), 0600) // 50 MiB used
	}()
	var stdout, stderr bytes.Buffer
	code := runResetPassword(nil, strings.NewReader(""), &stdout, &stderr, root)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Waiting for the server to release memory") {
		t.Fatalf("the wait was not announced: %q", stderr.String())
	}
	match := regexp.MustCompile(`(?m)^New password:  (\S{24})$`).FindStringSubmatch(stdout.String())
	if match == nil || authenticate(t, path, match[1]) != nil {
		t.Fatalf("password was not reset after the wait:\n%s", stdout.String())
	}
}

func TestResetPasswordRefusesWithoutMemoryHeadroom(t *testing.T) {
	shortenHeadroomWait(t, 60*time.Millisecond, 10*time.Millisecond)
	write := func(t *testing.T, limit, usage, stat string) string {
		t.Helper()
		root := t.TempDir()
		for name, value := range map[string]string{"memory.max": limit, "memory.current": usage, "memory.stat": stat} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(value+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	path := resetTestDatabase(t, true)

	var stdout, stderr bytes.Buffer
	tight := write(t, "134217728", "100663296", "anon 90000000\ninactive_file 1048576") // 128 MiB limit, 96 MiB used, 1 MiB reclaimable
	if code := runResetPassword(nil, strings.NewReader(""), &stdout, &stderr, tight); code != 1 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "one-off container") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	if err := authenticate(t, path, resetTestOldPassword); err != nil {
		t.Fatalf("a refused reset changed the password: %v", err)
	}

	for name, root := range map[string]string{
		"room left": write(t, "134217728", "41943040", "anon 40000000"),
		// 120 MiB used, but 60 MiB of it is page cache the kernel gives back.
		"reclaimable": write(t, "134217728", "125829120", "anon 60000000\ninactive_file 62914560\nactive_file 0"),
		"no limit":    write(t, "max", "100663296", ""),
	} {
		stdout.Reset()
		stderr.Reset()
		if code := runResetPassword(nil, strings.NewReader(""), &stdout, &stderr, root); code != 0 {
			t.Fatalf("%s: exit=%d stderr=%s", name, code, stderr.String())
		}
	}
}
