// SPDX-License-Identifier: AGPL-3.0-only
package observe

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLatencySummaryPercentiles(t *testing.T) {
	var l Latency
	if s := l.Summary(); s.Count != 0 || s.P50 != 0 || s.Max != 0 {
		t.Fatalf("empty summary = %+v", s)
	}
	for i := 1; i <= 100; i++ {
		l.Observe(time.Duration(i) * time.Millisecond)
	}
	s := l.Summary()
	if s.Count != 100 || s.P50 != 50*time.Millisecond || s.P95 != 95*time.Millisecond || s.Max != 100*time.Millisecond || s.Last != 100*time.Millisecond {
		t.Fatalf("summary = %+v", s)
	}
}

func TestLatencyKeepsOnlyTheLatestWindow(t *testing.T) {
	var l Latency
	l.Observe(time.Hour)
	for range LatencyWindow {
		l.Observe(time.Millisecond)
	}
	s := l.Summary()
	if s.Max != time.Millisecond || s.Count != LatencyWindow+1 {
		t.Fatalf("an observation older than the window is still reported: %+v", s)
	}
}

func TestErrorLogRateLimitsEachMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	now := time.Unix(0, 0)
	e := &ErrorLog{Logger: func() *slog.Logger { return logger }, Interval: time.Minute, now: func() time.Time { return now }}
	e.Error("write failed", "attempt", 1)
	e.Error("write failed", "attempt", 2)
	e.Error("write failed", "attempt", 3)
	e.Error("other failure")
	now = now.Add(time.Minute)
	e.Error("write failed", "attempt", 4)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("logged %d lines, want 3:\n%s", len(lines), buf.String())
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatal(err)
	}
	if last["msg"] != "write failed" || last["attempt"] != 4.0 || last["suppressed"] != 2.0 || last["level"] != "ERROR" {
		t.Fatalf("resumed line = %v", last)
	}
	if !strings.Contains(lines[1], "other failure") {
		t.Fatalf("a different message was limited by the first: %s", lines[1])
	}
}

func TestErrorLogFallsBackToDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)
	(&ErrorLog{Logger: func() *slog.Logger { return nil }}).Warn("budget measurement failed")
	if !strings.Contains(buf.String(), "budget measurement failed") || !strings.Contains(buf.String(), "WARN") {
		t.Fatalf("default logger got %q", buf.String())
	}
}
