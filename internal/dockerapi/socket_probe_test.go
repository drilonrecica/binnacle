// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveDocker answers /_ping like a daemon and /images/json with status.
func serveDocker(t *testing.T, status int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.55")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			if status != http.StatusOK {
				http.Error(w, http.StatusText(status), status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return path
}

func TestProbeSocketAccessClassifiesDaemonAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		want   SocketAccess
	}{
		"raw socket answers":       {http.StatusOK, SocketRaw},
		"proxy forbids":            {http.StatusForbidden, SocketConstrained},
		"proxy rejects the method": {http.StatusMethodNotAllowed, SocketConstrained},
		"proxy has no such route":  {http.StatusNotFound, SocketConstrained},
	} {
		t.Run(name, func(t *testing.T) {
			engine, err := NewEngine(serveDocker(t, tc.status))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = engine.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := engine.ProbeSocketAccess(ctx)
			if err != nil || got != tc.want {
				t.Fatalf("ProbeSocketAccess()=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestProbeSocketAccessUnknownWhenUnreachable(t *testing.T) {
	engine, err := NewEngine(filepath.Join(t.TempDir(), "missing.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := engine.ProbeSocketAccess(ctx)
	if err == nil || got != SocketUnknown {
		t.Fatalf("ProbeSocketAccess()=%q err=%v, want unknown with an error", got, err)
	}
}
