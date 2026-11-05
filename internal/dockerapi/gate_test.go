// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// versionedClient reports a version that the test can change, or an error.
type versionedClient struct {
	mu      sync.Mutex
	version string
	err     error
	probes  atomic.Int32
	lists   atomic.Int32
}

func (c *versionedClient) set(version string, err error) {
	c.mu.Lock()
	c.version, c.err = version, err
	c.mu.Unlock()
}
func (c *versionedClient) List(context.Context) ([]Container, error) {
	c.lists.Add(1)
	return []Container{{ID: "c1"}}, nil
}
func (c *versionedClient) Inspect(context.Context, string) (Inspect, error) { return Inspect{}, nil }
func (c *versionedClient) Stats(context.Context, string) (Stats, error)     { return Stats{}, nil }
func (c *versionedClient) Events(ctx context.Context) <-chan Event {
	out := make(chan Event, 1)
	out <- Event{ID: "c1", Action: "start"}
	go func() { <-ctx.Done(); close(out) }()
	return out
}
func (c *versionedClient) Version(context.Context) (Version, error) {
	c.probes.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	return Version{EngineVersion: c.version}, c.err
}
func (c *versionedClient) Diagnostics(context.Context) (Diagnostics, error) {
	return Diagnostics{}, nil
}

func quietGate(client Client) *Gate {
	g := NewGate(client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.initial, g.max = 10*time.Millisecond, 40*time.Millisecond
	return g
}

// TestGateStartsDegradedAndRetries: an unsupported Engine no longer fails
// startup (RUN-3); calls are refused with the reason until a supported
// release answers, then the gate opens.
func TestGateStartsDegradedAndRetries(t *testing.T) {
	client := &versionedClient{version: "28.0.0"}
	gate := quietGate(client)
	var opened atomic.Int32
	gate.OnOpen = func(context.Context) { opened.Add(1) }
	if err := gate.Start(context.Background()); err != nil {
		t.Fatalf("Start failed on an unsupported Engine: %v", err)
	}
	defer gate.Stop(context.Background())

	_, err := gate.List(context.Background())
	if !errors.Is(err, ErrEngineUnverified) || !strings.Contains(err.Error(), "28.0.0") || !strings.Contains(err.Error(), MinimumEngineVersion) {
		t.Fatalf("List on a closed gate = %v, want ErrEngineUnverified naming the version and the minimum", err)
	}
	if client.lists.Load() != 0 {
		t.Fatal("a call reached the unsupported Engine")
	}
	if err := gate.ReadLogs(context.Background(), "c1", LogOptions{}, func(string, string) error { return nil }); !errors.Is(err, ErrEngineUnverified) {
		t.Fatalf("ReadLogs on a closed gate = %v", err)
	}
	if _, err := gate.Inspect(context.Background(), "c1"); !errors.Is(err, ErrEngineUnverified) {
		t.Fatalf("Inspect on a closed gate = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for client.probes.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("probed %d times, want retries with backoff", client.probes.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	client.set("29.5.1", nil)
	select {
	case <-gate.Opened():
	case <-time.After(2 * time.Second):
		t.Fatal("gate did not open after the Engine was upgraded")
	}
	if _, err := gate.List(context.Background()); err != nil {
		t.Fatalf("List on an open gate: %v", err)
	}
	for deadline := time.Now().Add(time.Second); opened.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	if opened.Load() != 1 || gate.Reason() != "" {
		t.Fatalf("OnOpen ran %d times, reason %q", opened.Load(), gate.Reason())
	}
}

// TestGateUnreachableEngine: a daemon that does not answer is a reason, not
// a startup failure.
func TestGateUnreachableEngine(t *testing.T) {
	client := &versionedClient{err: errors.New("dial unix /var/run/docker.sock: connect: no such file or directory")}
	gate := quietGate(client)
	if err := gate.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.Contains(gate.Reason(), "no such file") {
		t.Fatalf("reason = %q", gate.Reason())
	}
	stopped := make(chan struct{})
	go func() { _ = gate.Stop(context.Background()); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not end the retry loop")
	}
}

// TestGateEventsWaitForOpen: the event stream does not connect before the
// Engine is verified, and delivers once it is.
func TestGateEventsWaitForOpen(t *testing.T) {
	client := &versionedClient{version: "28.0.0"}
	gate := quietGate(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gate.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer gate.Stop(context.Background())
	events := gate.Events(ctx)
	select {
	case event := <-events:
		t.Fatalf("event %v delivered from an unverified Engine", event)
	case <-time.After(30 * time.Millisecond):
	}
	client.set("29.6.0", nil)
	select {
	case event := <-events:
		if event.ID != "c1" {
			t.Fatalf("event = %v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event after the gate opened")
	}
}
