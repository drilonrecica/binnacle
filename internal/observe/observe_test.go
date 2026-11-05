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

func TestLatencyHistogramIsCumulativeSinceStart(t *testing.T) {
	var l Latency
	l.Observe(500 * time.Microsecond) // le 1 ms
	l.Observe(time.Millisecond)       // a bound belongs to its own bucket
	l.Observe(45 * time.Millisecond)  // le 50 ms
	l.Observe(time.Minute)            // beyond the last bound
	// Older than the window, the histogram still counts it.
	for range LatencyWindow {
		l.Observe(3 * time.Millisecond) // le 5 ms
	}
	s := l.Summary()
	at := func(bound time.Duration) uint64 {
		for i, b := range HistogramBounds {
			if b == bound {
				return s.Buckets[i]
			}
		}
		t.Fatalf("no bucket for %s", bound)
		return 0
	}
	if got := at(time.Millisecond); got != 2 {
		t.Fatalf("le 1ms = %d, want 2", got)
	}
	if got := at(2 * time.Millisecond); got != 2 {
		t.Fatalf("le 2ms = %d, want 2", got)
	}
	if got := at(5 * time.Millisecond); got != 2+LatencyWindow {
		t.Fatalf("le 5ms = %d, want %d", got, 2+LatencyWindow)
	}
	if got := at(50 * time.Millisecond); got != 3+LatencyWindow {
		t.Fatalf("le 50ms = %d", got)
	}
	if got := at(30 * time.Second); got != 3+LatencyWindow {
		t.Fatalf("le 30s = %d", got)
	}
	if last := s.Buckets[len(s.Buckets)-1]; last != s.Count || s.Count != 4+LatencyWindow {
		t.Fatalf("+Inf bucket = %d, count = %d", last, s.Count)
	}
	want := 500*time.Microsecond + time.Millisecond + 45*time.Millisecond + time.Minute + LatencyWindow*3*time.Millisecond
	if s.Sum != want {
		t.Fatalf("sum = %s, want %s", s.Sum, want)
	}
}
