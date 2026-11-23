// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/settings"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const (
	resetCommand = "reset-password"
	resetTimeout = 30 * time.Second
	// resetHeadroom is the free memory the command wants inside a limited
	// cgroup before it hashes. One Argon2 unit is 32 MiB, and with the Go
	// runtime and SQLite the command is OOM-killed under a 36 MiB limit and
	// completes under 38 MiB; the rest is margin. The command shares the
	// running server's cgroup under docker exec, so going over the limit
	// could get the server killed instead.
	resetHeadroom = 44 << 20
	// maxPasswordInput bounds what --password-stdin reads; ValidatePassword
	// rejects anything over 1024 bytes anyway.
	maxPasswordInput = 4096
)

// cgroupMemoryUsage reads the memory the process's cgroup cannot give back
// under root: current usage (cgroup v2 memory.current, then cgroup v1) less
// the inactive page cache, which the kernel reclaims before it kills anything.
func cgroupMemoryUsage(root string) (int64, bool) {
	for _, file := range []struct{ usage, stat, inactive string }{
		{"memory.current", "memory.stat", "inactive_file"},
		{filepath.Join("memory", "memory.usage_in_bytes"), filepath.Join("memory", "memory.stat"), "total_inactive_file"},
	} {
		raw, err := os.ReadFile(filepath.Join(root, file.usage))
		if err != nil {
			continue
		}
		usage, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil || usage < 0 {
			return 0, false
		}
		if stat, err := os.ReadFile(filepath.Join(root, file.stat)); err == nil {
			for _, line := range strings.Split(string(stat), "\n") {
				if name, value, ok := strings.Cut(line, " "); ok && name == file.inactive {
					if cache, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && cache > 0 && cache <= usage {
						usage -= cache
					}
					break
				}
			}
		}
		return usage, true
	}
	return 0, false
}

// resetHeadroomWait and resetHeadroomPoll bound how long the command waits
// for memory. A login attempt leaves the server holding one Argon2 unit until
// its next forced garbage collection, two to three minutes later, and someone
// locked out has usually just tried to log in. Variables so tests can shorten
// them.
var (
	resetHeadroomWait = 4 * time.Minute
	resetHeadroomPoll = 5 * time.Second
)

// resetHeadroomFree is the free memory in a limited cgroup, and false when
// there is no limit or usage cannot be read.
func resetHeadroomFree(root string) (free, limit int64, limited bool) {
	limit, ok := cgroupMemoryLimit(root)
	if !ok {
		return 0, 0, false
	}
	usage, ok := cgroupMemoryUsage(root)
	if !ok {
		return 0, 0, false
	}
	return limit - usage, limit, true
}

// waitForResetHeadroom holds the command back while a limited cgroup has too
// little room to hash in, and gives up after resetHeadroomWait. Without a
// limit it returns at once: the guard only exists to protect a server sharing
// the cgroup.
func waitForResetHeadroom(root string, progress io.Writer) error {
	deadline := time.Now().Add(resetHeadroomWait)
	announced := false
	for {
		free, limit, limited := resetHeadroomFree(root)
		if !limited || free >= resetHeadroom {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("only %d MiB of the container's %d MiB memory limit is free and hashing the password needs %d MiB; stop Binnacle and run this command in a one-off container on the same data volume instead", free>>20, limit>>20, int64(resetHeadroom)>>20)
		}
		if !announced {
			fmt.Fprintf(progress, "Waiting for the server to release memory before hashing the password (up to %s)...\n", resetHeadroomWait)
			announced = true
		}
		time.Sleep(resetHeadroomPoll)
	}
}

// readPassword reads one line from in for --password-stdin. Only the trailing
// line ending is removed, so spaces in a password stay significant.
func readPassword(in io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(in, maxPasswordInput+1))
	if err != nil {
		return "", fmt.Errorf("read password from standard input: %w", err)
	}
	if len(raw) > maxPasswordInput {
		return "", errors.New("password on standard input is too long")
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if strings.ContainsAny(password, "\r\n") {
		return "", errors.New("password on standard input must be a single line")
	}
	return password, nil
}

// runResetPassword implements `binnacle reset-password` and returns the exit
// code. It reads the same configuration as the server to find the database
// and works whether or not the server is running.
func runResetPassword(args []string, stdin io.Reader, stdout, stderr io.Writer, cgroup string) int {
	flags := flag.NewFlagSet(resetCommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	fromStdin := flags.Bool("password-stdin", false, "read the new password from standard input instead of generating one")
	disableMFA := flags.Bool("disable-mfa", false, "also remove two-factor authentication and its recovery codes")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: binnacle %s [--password-stdin] [--disable-mfa]\n\nSets a new password for the administrator and signs out every session.\nRun it on the host, for example: docker exec <container> binnacle %s\n\n", resetCommand, resetCommand)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument %q\n", flags.Arg(0))
		flags.Usage()
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "reset-password:", err)
		return 1
	}
	config, _, err := settings.Load()
	if err != nil {
		return fail(fmt.Errorf("configuration is invalid: %w", err))
	}
	var password string
	if *fromStdin {
		if password, err = readPassword(stdin); err != nil {
			return fail(err)
		}
	} else if password, err = auth.GeneratePassword(); err != nil {
		return fail(err)
	}
	if err = auth.ValidatePassword(password); err != nil {
		return fail(err)
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), resetTimeout)
	defer cancelOpen()
	db, closeDB, err := storage.OpenExisting(openCtx, config.Paths.DatabasePath)
	if err != nil {
		return fail(err)
	}
	defer closeDB()
	noAdmin := errors.New("no administrator exists yet; open Binnacle in a browser and complete first-run setup")
	// Checked before the wait so a database without an administrator fails
	// at once instead of after it.
	var users int
	if err = db.QueryRowContext(openCtx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		return fail(err)
	}
	if users == 0 {
		return fail(noAdmin)
	}
	if err = waitForResetHeadroom(cgroup, stderr); err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), resetTimeout)
	defer cancel()
	result, err := auth.ResetAdminPassword(ctx, db, password, *disableMFA)
	if errors.Is(err, auth.ErrNoAdmin) {
		return fail(noAdmin)
	}
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "Administrator: %s\n", result.User.Username)
	if *fromStdin {
		fmt.Fprintln(stdout, "The password was replaced with the one from standard input.")
	} else {
		fmt.Fprintf(stdout, "New password:  %s\n", password)
	}
	fmt.Fprintln(stdout, "Every session was signed out. API tokens were kept.")
	switch {
	case *disableMFA:
		fmt.Fprintln(stdout, "Two-factor authentication was removed.")
	case result.MFAEnabled:
		fmt.Fprintf(stdout, "Two-factor authentication is still on. If the authenticator and recovery codes are lost too, run this again with --disable-mfa.\n")
	}
	return 0
}
