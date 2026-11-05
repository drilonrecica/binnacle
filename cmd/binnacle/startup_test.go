// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

func TestHealthcheckURLFollowsListenAddress(t *testing.T) {
	for listen, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"[::]:9001":      "http://127.0.0.1:9001/healthz",
		"127.0.0.1:7000": "http://127.0.0.1:7000/healthz",
		"[::1]:7001":     "http://[::1]:7001/healthz",
		"localhost:7002": "http://localhost:7002/healthz",
		"10.0.0.5:8080":  "http://10.0.0.5:8080/healthz",
	} {
		got, err := healthcheckURL(listen)
		if err != nil || got != want {
			t.Fatalf("healthcheckURL(%q)=%q err=%v, want %q", listen, got, err, want)
		}
	}
	if _, err := healthcheckURL("8080"); err == nil {
		t.Fatal("listen address without a port accepted")
	}
}

func TestHealthcheckDialsTheListenAddressAndTimesOut(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	}))
	defer ok.Close()
	if err := runHealthcheck(strings.TrimPrefix(ok.URL, "http://"), time.Second); err != nil {
		t.Fatalf("healthy server: %v", err)
	}
	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer hang.Close()
	defer close(release)
	started := time.Now()
	if err := runHealthcheck(strings.TrimPrefix(hang.URL, "http://"), 100*time.Millisecond); err == nil {
		t.Fatal("hanging server reported healthy")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("health check took %s, timeout not applied", elapsed)
	}
	if healthcheckTimeout != 3*time.Second {
		t.Fatalf("healthcheckTimeout=%s, want 3s", healthcheckTimeout)
	}
}

type fakeProber struct {
	access dockerapi.SocketAccess
	err    error
}

func (f fakeProber) ProbeSocketAccess(ctx context.Context) (dockerapi.SocketAccess, error) {
	if _, ok := ctx.Deadline(); !ok {
		return dockerapi.SocketUnknown, errors.New("probe without deadline")
	}
	return f.access, f.err
}

func TestProbeDockerSocketWarnsOnRawSocket(t *testing.T) {
	for _, tc := range []struct {
		prober fakeProber
		warn   bool
	}{
		{fakeProber{access: dockerapi.SocketRaw}, true},
		{fakeProber{access: dockerapi.SocketConstrained}, false},
		{fakeProber{access: dockerapi.SocketUnknown, err: errors.New("dial")}, true},
	} {
		var out bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&out, nil))
		var result atomic.Value
		probeDockerSocket(context.Background(), log, tc.prober, &result)
		if got := result.Load(); got != string(tc.prober.access) {
			t.Fatalf("stored %v, want %s", got, tc.prober.access)
		}
		if warned := strings.Contains(out.String(), `"level":"WARN"`); warned != tc.warn {
			t.Fatalf("access %s warned=%v log=%s", tc.prober.access, warned, out.String())
		}
		if tc.prober.access == dockerapi.SocketRaw && !strings.Contains(out.String(), "raw daemon socket") {
			t.Fatalf("raw socket warning unclear: %s", out.String())
		}
	}
}
