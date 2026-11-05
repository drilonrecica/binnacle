// SPDX-License-Identifier: AGPL-3.0-only

// Package observe holds the bounded self-observability primitives the
// background workers share (RUN-7): failure counters, latency windows and a
// rate-limited error log. Nothing here leaves the process; the values are
// read by monitor-health, the System settings page and the optional
// Prometheus endpoint.
package observe

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Counter counts events since the process started.
type Counter struct{ n atomic.Uint64 }

func (c *Counter) Inc()         { c.n.Add(1) }
func (c *Counter) Load() uint64 { return c.n.Load() }
func (c *Counter) Add(n uint64) { c.n.Add(n) }

// LatencyWindow is how many recent observations a Latency keeps. The
// percentiles describe the latest operations, not the whole uptime, so a slow
// hour long ago does not hide a healthy present (and the reverse).
const LatencyWindow = 256

// Latency keeps the durations of the latest LatencyWindow operations. Its
// memory is fixed; Observe is O(1) and Summary sorts at most LatencyWindow
// values.
type Latency struct {
	mu      sync.Mutex
	samples [LatencyWindow]time.Duration
	next    int
	size    int
	count   uint64
	last    time.Duration
}

// Observe records one operation's duration.
func (l *Latency) Observe(d time.Duration) {
	if d < 0 {
		d = 0
	}
	l.mu.Lock()
	l.samples[l.next] = d
	l.next = (l.next + 1) % LatencyWindow
	if l.size < LatencyWindow {
		l.size++
	}
	l.count++
	l.last = d
	l.mu.Unlock()
}

// Since observes the time elapsed since started.
func (l *Latency) Since(started time.Time) { l.Observe(time.Since(started)) }

// LatencySummary is the nearest-rank p50, p95 and max of the latest window,
// the latest duration and the number of operations since start. Count is 0
// until the first observation.
type LatencySummary struct {
	Count         uint64
	Last          time.Duration
	P50, P95, Max time.Duration
}

// Summary returns the current percentiles.
func (l *Latency) Summary() LatencySummary {
	l.mu.Lock()
	values := slices.Clone(l.samples[:l.size])
	summary := LatencySummary{Count: l.count, Last: l.last}
	l.mu.Unlock()
	if len(values) == 0 {
		return summary
	}
	slices.Sort(values)
	summary.P50 = rank(values, 0.50)
	summary.P95 = rank(values, 0.95)
	summary.Max = values[len(values)-1]
	return summary
}

// rank is the nearest-rank percentile of sorted values.
func rank(sorted []time.Duration, q float64) time.Duration {
	i := int(q*float64(len(sorted))+0.999999) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// DefaultLogInterval is how often one message may be logged by an ErrorLog.
const DefaultLogInterval = time.Minute

// maxLogKeys bounds the messages an ErrorLog tracks. Messages are constants
// in the code, so the bound is never reached in practice; past it a message
// is logged without limiting rather than dropped.
const maxLogKeys = 256

// ErrorLog writes worker errors through the app logger, at most once per
// Interval for each message. A failing worker retries every tick, so without
// the limit one broken disk or Docker daemon would flood the log; with it the
// first failure is logged at once and the next one, after Interval, carries
// the number of identical messages suppressed in between. The failure
// counters still count every occurrence.
type ErrorLog struct {
	// Logger resolves the app logger at log time, so a component whose Logger
	// field is set after construction is covered. Nil uses slog.Default().
	Logger   func() *slog.Logger
	Interval time.Duration
	now      func() time.Time
	mu       sync.Mutex
	keys     map[string]*logState
}

type logState struct {
	last       time.Time
	suppressed uint64
}

// Error logs msg at error level, rate limited by msg.
func (e *ErrorLog) Error(msg string, args ...any) { e.Log(slog.LevelError, msg, args...) }

// Warn logs msg at warning level, rate limited by msg.
func (e *ErrorLog) Warn(msg string, args ...any) { e.Log(slog.LevelWarn, msg, args...) }

// Log writes msg at level unless the same message was written less than
// Interval ago; a suppressed message is counted and reported on the next
// write as "suppressed".
func (e *ErrorLog) Log(level slog.Level, msg string, args ...any) {
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	interval := e.Interval
	if interval <= 0 {
		interval = DefaultLogInterval
	}
	e.mu.Lock()
	if e.keys == nil {
		e.keys = map[string]*logState{}
	}
	state, ok := e.keys[msg]
	if !ok && len(e.keys) < maxLogKeys {
		state = &logState{}
		e.keys[msg] = state
	}
	var suppressed uint64
	if state != nil {
		if !state.last.IsZero() && now.Sub(state.last) < interval {
			state.suppressed++
			e.mu.Unlock()
			return
		}
		suppressed, state.suppressed, state.last = state.suppressed, 0, now
	}
	e.mu.Unlock()
	if suppressed > 0 {
		args = append(args, "suppressed", suppressed)
	}
	logger := slog.Default()
	if e.Logger != nil {
		if l := e.Logger(); l != nil {
			logger = l
		}
	}
	logger.Log(context.Background(), level, msg, args...)
}
