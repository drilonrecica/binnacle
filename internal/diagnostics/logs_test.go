// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"context"
	"strings"
	"testing"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

type fakeLogs struct{ lines []string }

func (f fakeLogs) ReadLogs(ctx context.Context, _ string, _ dockerapi.LogOptions, emit func(string, string) error) error {
	for _, line := range f.lines {
		if err := emit("stdout", line); err != nil {
			return err
		}
	}
	return nil
}

func TestLogServiceRedactsCompleteSecretValues(t *testing.T) {
	service, err := NewLogService(fakeLogs{}, 5000, 1<<20, []string{`session-[a-z]+`})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := "-----BEGIN PRIVATE KEY-----\nprivate material\n-----END PRIVATE KEY-----"
	tests := map[string]string{
		`{"password":"correct horse battery staple"}`:         `{"password":"[REDACTED]"}`,
		`secret = 'two words with \'escaped\' quotes'`:        `secret = '[REDACTED]'`,
		`{"api_key": "value with \"escaped\" quotes"}`:        `{"api_key": "[REDACTED]"}`,
		`password=correct horse battery staple`:               `password=[REDACTED]`,
		`token=abc123; user=admin`:                            `token=[REDACTED]; user=admin`,
		`Authorization: Basic dXNlcjpwYXNz`:                   `Authorization: Basic [REDACTED]`,
		`postgres://user:correct-horse@example.test/database`: `postgres://user:[REDACTED]@example.test/database`,
		`request session-secret completed`:                    `request [REDACTED] completed`,
		privateKey:                                            `[REDACTED PRIVATE KEY]`,
	}
	for input, want := range tests {
		if got := service.Redact(input); got != want {
			t.Errorf("Redact(%q)=%q, want %q", input, got, want)
		}
		if strings.Contains(service.Redact(input), "correct horse") {
			t.Errorf("Redact(%q) retained secret text", input)
		}
	}
}

func TestLogServiceRedactsBeforeLiteralSearchAndPreservesMessage(t *testing.T) {
	service, err := NewLogService(fakeLogs{[]string{
		`2026-01-02T03:04:05Z {"level":"error","token":"secret"}`,
		`Authorization: Bearer abc123`,
	}}, 5000, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Read(context.Background(), LogRequest{Components: []string{"container"}, Limit: 10, Search: "[REDACTED]"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries=%d", len(result.Entries))
	}
	if result.Entries[0].Severity != "error" {
		t.Fatalf("severity=%q", result.Entries[0].Severity)
	}
	if result.Entries[0].Message != `{"level":"error","token":"[REDACTED]"}` {
		t.Fatalf("message=%q", result.Entries[0].Message)
	}
}

func TestLogServiceReportsTruncation(t *testing.T) {
	service, _ := NewLogService(fakeLogs{[]string{"one", "two", "three"}}, 5000, 1<<20, nil)
	result, err := service.Read(context.Background(), LogRequest{Components: []string{"container"}, Limit: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Entries) != 2 {
		t.Fatalf("result=%+v", result)
	}
}

func TestLogServiceRedactsPrivateKeySpreadAcrossLines(t *testing.T) {
	service, err := NewLogService(fakeLogs{[]string{
		`2026-01-02T03:04:05Z starting`,
		`2026-01-02T03:04:06Z key: -----BEGIN RSA PRIVATE KEY-----`,
		`2026-01-02T03:04:06Z MIIEowIBAAKCAQEAsecretmaterial`,
		`2026-01-02T03:04:06Z moresecretmaterial==`,
		`2026-01-02T03:04:06Z -----END RSA PRIVATE KEY----- loaded`,
		`2026-01-02T03:04:07Z ready`,
	}}, 5000, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Read(context.Background(), LogRequest{Components: []string{"container"}, Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, entry := range result.Entries {
		messages = append(messages, entry.Message)
	}
	want := []string{"starting", "key: [REDACTED PRIVATE KEY]", "loaded", "ready"}
	if strings.Join(messages, "|") != strings.Join(want, "|") {
		t.Fatalf("messages=%q, want %q", messages, want)
	}
}

func TestLogServiceClosesUnterminatedPrivateKeyAfterBound(t *testing.T) {
	lines := []string{"-----BEGIN PRIVATE KEY-----"}
	for i := 0; i < maxPEMLines+5; i++ {
		lines = append(lines, "material")
	}
	lines = append(lines, "after")
	service, _ := NewLogService(fakeLogs{lines}, 5000, 1<<20, nil)
	result, err := service.Read(context.Background(), LogRequest{Components: []string{"container"}, Limit: 5000}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(result.Entries); n != 1+5+1 {
		t.Fatalf("entries=%d: an unterminated block must stop hiding lines after %d", n, maxPEMLines)
	}
	if last := result.Entries[len(result.Entries)-1].Message; last != "after" {
		t.Fatalf("last=%q", last)
	}
}

func TestLogServiceStripsANSIEscapesBeforeRedaction(t *testing.T) {
	service, _ := NewLogService(fakeLogs{[]string{
		"\x1b[31mERROR\x1b[0m password=\x1b[1mhunter2\x1b[0m",
		"\x1b]0;title\x07plain \x1b(Bline",
	}}, 5000, 1<<20, nil)
	result, err := service.Read(context.Background(), LogRequest{Components: []string{"container"}, Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Entries[0]; got.Message != "ERROR password=[REDACTED]" || got.Severity != "error" {
		t.Fatalf("entry=%+v", got)
	}
	if got := result.Entries[1].Message; got != "plain line" {
		t.Fatalf("message=%q", got)
	}
}
