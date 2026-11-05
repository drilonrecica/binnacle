// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

// followingLogs emits one line per component, then follows until ctx ends,
// as Docker does for a followed container. It records the options it saw.
type followingLogs struct {
	mu      sync.Mutex
	options []dockerapi.LogOptions
}

func (f *followingLogs) ReadLogs(ctx context.Context, id string, options dockerapi.LogOptions, emit func(string, string) error) error {
	f.mu.Lock()
	f.options = append(f.options, options)
	f.mu.Unlock()
	if err := emit("stdout", "2026-07-11T12:00:00Z started "+id); err != nil {
		return err
	}
	if !options.Follow {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

// RUN-6: a followed resource streams every component at once, and no follow
// is bounded by Until.
func TestFollowReadsEveryComponentWithoutUntil(t *testing.T) {
	docker := &followingLogs{}
	service, err := NewLogService(docker, 5000, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	seen := map[string]bool{}
	all := make(chan struct{})
	done := make(chan error, 1)
	request := LogRequest{Components: []string{"web", "worker", "db"}, From: time.Now().Add(-time.Hour), To: time.Now(), Follow: true}
	go func() {
		_, err := service.Read(ctx, request, func(entry LogEntry) error {
			mu.Lock()
			defer mu.Unlock()
			seen[entry.Component] = true
			if len(seen) == 3 {
				close(all)
			}
			return nil
		})
		done <- err
	}()
	select {
	case <-all:
	case <-time.After(2 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("components streamed: %v; the first follow blocked the rest", seen)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Read = %v, want context.Canceled when the caller ends the follow", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not end with its context")
	}
	docker.mu.Lock()
	defer docker.mu.Unlock()
	for _, options := range docker.options {
		if !options.Until.IsZero() {
			t.Fatalf("follow passed Until=%s; a follow bounded by now ends immediately", options.Until)
		}
	}
}
