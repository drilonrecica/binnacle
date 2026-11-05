// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

// followClient answers collection calls at once; ReadLogs follows until its
// context ends, as an open log view does.
type followClient struct{ following chan struct{} }

func (f *followClient) List(context.Context) ([]Container, error) {
	return []Container{{ID: "c1"}}, nil
}
func (f *followClient) Inspect(context.Context, string) (Inspect, error) { return Inspect{}, nil }
func (f *followClient) Stats(context.Context, string) (Stats, error)     { return Stats{}, nil }
func (f *followClient) Events(context.Context) <-chan Event              { return nil }
func (f *followClient) Version(context.Context) (Version, error)         { return Version{}, nil }
func (f *followClient) Diagnostics(context.Context) (Diagnostics, error) { return Diagnostics{}, nil }
func (f *followClient) ReadLogs(ctx context.Context, _ string, _ LogOptions, _ func(string, string) error) error {
	f.following <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

// RUN-2: open log follows hold only the log semaphore, so collection keeps
// its slots, and the number of concurrent log reads stays bounded.
func TestLogFollowsDoNotStarveCollection(t *testing.T) {
	client := &followClient{following: make(chan struct{}, LogConcurrency+1)}
	limited := New(client, LogConcurrency)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, LogConcurrency+1)
	for range LogConcurrency + 1 {
		go func() { done <- limited.ReadLogs(ctx, "c1", LogOptions{Follow: true}, nil) }()
	}
	for range LogConcurrency {
		<-client.following
	}

	call, callCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer callCancel()
	if _, err := limited.List(call); err != nil {
		t.Fatalf("List with %d open follows: %v", LogConcurrency, err)
	}
	if _, err := limited.Stats(call, "c1"); err != nil {
		t.Fatalf("Stats with %d open follows: %v", LogConcurrency, err)
	}
	select {
	case <-client.following:
		t.Fatalf("more than %d log reads ran at once", LogConcurrency)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	for range LogConcurrency + 1 {
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("log read ended with %v", err)
		}
	}
}
