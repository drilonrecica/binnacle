// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"errors"
	"testing"
)

type failingClient struct{ followClient }

func (failingClient) List(context.Context) ([]Container, error) {
	return nil, errors.New("daemon unreachable")
}

// RUN-7: every failed Docker call is counted; a call the caller cancelled
// before it got a slot is not a Docker failure.
func TestLimitedCountsFailedCalls(t *testing.T) {
	limited := New(&failingClient{}, 1)
	for range 2 {
		if _, err := limited.List(context.Background()); err == nil {
			t.Fatal("List succeeded")
		}
	}
	if _, err := limited.Inspect(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	limited.sem <- struct{}{} // every slot taken
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := limited.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if got := limited.FailureCount(); got != 2 {
		t.Fatalf("failures=%d, want 2", got)
	}
}
