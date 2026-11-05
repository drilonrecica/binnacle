// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ErrEngineUnverified is wrapped by every call the Gate refuses because no
// supported Docker Engine has answered the version probe yet.
var ErrEngineUnverified = errors.New("Docker Engine not verified")

// Gate backoff bounds: the first retry after GateInitialBackoff, doubling up
// to GateMaxBackoff.
const (
	GateInitialBackoff = 5 * time.Second
	GateMaxBackoff     = 5 * time.Minute
)

// Gate keeps the version requirement fail-closed without failing startup
// (RUN-3). Until the Engine reports a supported release, every call except
// the version probe itself is refused with an error that wraps
// ErrEngineUnverified and names the reason, so the collector reports Docker
// degraded with that reason while the UI, history and checks keep running.
// The probe repeats with backoff; once it passes the gate stays open.
type Gate struct {
	Client Client
	Logger *slog.Logger
	// OnOpen runs once, on the probe goroutine, when the gate opens.
	OnOpen func(context.Context)

	mu     sync.RWMutex
	open   bool
	reason string
	opened chan struct{}
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// initial and max override the backoff bounds in tests.
	initial, max time.Duration
}

// NewGate wraps client. The gate is closed until Start's probe succeeds.
func NewGate(client Client, log *slog.Logger) *Gate {
	if log == nil {
		log = slog.Default()
	}
	return &Gate{Client: client, Logger: log, reason: "Docker Engine version not yet probed", opened: make(chan struct{})}
}

func (g *Gate) Name() string { return "docker-gate" }

// Start probes once synchronously, bounded by the probe timeout, and leaves a
// retry loop running when that probe fails. It never returns an error for an
// unsupported or unreachable Engine.
func (g *Gate) Start(ctx context.Context) error {
	ctx, g.cancel = context.WithCancel(ctx)
	if g.probe(ctx) {
		return nil
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.retry(ctx)
	}()
	return nil
}

// Stop ends the retry loop. It does not close the client; the collector
// closes it after its own loops stopped (ADR 022 §7).
func (g *Gate) Stop(context.Context) error {
	if g.cancel != nil {
		g.cancel()
	}
	g.wg.Wait()
	return nil
}

func (g *Gate) retry(ctx context.Context) {
	initial, max := g.initial, g.max
	if initial <= 0 {
		initial = GateInitialBackoff
	}
	if max <= 0 {
		max = GateMaxBackoff
	}
	delay := initial
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if g.probe(ctx) {
			return
		}
		delay = min(delay*2, max)
	}
}

// probe runs one bounded version check and opens the gate when it passes.
func (g *Gate) probe(ctx context.Context) bool {
	err := RequireSupportedEngine(ctx, g.Client)
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		g.mu.Lock()
		changed := g.reason != err.Error()
		g.reason = err.Error()
		g.mu.Unlock()
		if changed {
			g.Logger.Warn("Docker collection paused until a supported Engine is reachable; retrying with backoff", "reason", err.Error(), "minimum", MinimumEngineVersion)
		}
		return false
	}
	g.mu.Lock()
	g.open, g.reason = true, ""
	close(g.opened)
	g.mu.Unlock()
	g.Logger.Info("Docker Engine version verified")
	if g.OnOpen != nil {
		g.OnOpen(ctx)
	}
	return true
}

// Open reports whether a supported Engine has been verified.
func (g *Gate) Open() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.open
}

// Opened is closed when the gate opens.
func (g *Gate) Opened() <-chan struct{} { return g.opened }

// Reason is why the gate is closed, "" once it is open.
func (g *Gate) Reason() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.reason
}

func (g *Gate) check() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.open {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrEngineUnverified, g.reason)
}

func (g *Gate) List(ctx context.Context) ([]Container, error) {
	if err := g.check(); err != nil {
		return nil, err
	}
	return g.Client.List(ctx)
}
func (g *Gate) Inspect(ctx context.Context, id string) (Inspect, error) {
	if err := g.check(); err != nil {
		return Inspect{}, err
	}
	return g.Client.Inspect(ctx, id)
}
func (g *Gate) Stats(ctx context.Context, id string) (Stats, error) {
	if err := g.check(); err != nil {
		return Stats{}, err
	}
	return g.Client.Stats(ctx, id)
}

// Events waits for the gate to open before subscribing, so the event
// stream never connects to an unverified Engine. The returned channel closes
// when ctx ends, as the Engine's does.
func (g *Gate) Events(ctx context.Context) <-chan Event {
	if g.check() == nil {
		return g.Client.Events(ctx)
	}
	out := make(chan Event)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-g.opened:
		}
		for event := range g.Client.Events(ctx) {
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Version always reaches the Engine: it is how the gate itself is probed and
// what diagnostics report.
func (g *Gate) Version(ctx context.Context) (Version, error) { return g.Client.Version(ctx) }

func (g *Gate) Diagnostics(ctx context.Context) (Diagnostics, error) {
	if err := g.check(); err != nil {
		return Diagnostics{}, err
	}
	return g.Client.Diagnostics(ctx)
}

func (g *Gate) ReadLogs(ctx context.Context, id string, options LogOptions, emit func(string, string) error) error {
	if err := g.check(); err != nil {
		return err
	}
	logs, ok := g.Client.(LogClient)
	if !ok {
		return fmt.Errorf("docker log access is unavailable")
	}
	return logs.ReadLogs(ctx, id, options, emit)
}

func (g *Gate) Close() error {
	if closer, ok := g.Client.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
