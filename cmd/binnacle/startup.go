// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const (
	healthcheckTimeout = 3 * time.Second
	socketProbeTimeout = 5 * time.Second
)

// healthcheckURL turns the listen setting into the address a local probe
// dials: an unspecified or empty host means every interface, so loopback.
func healthcheckURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("listen address %q: %w", listen, err)
	}
	if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}

func runHealthcheck(listen string, timeout time.Duration) error {
	target, err := healthcheckURL(listen)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(target)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}

type socketProber interface {
	ProbeSocketAccess(context.Context) (dockerapi.SocketAccess, error)
}

// probeDockerSocket records whether the Docker socket answers a path the
// socket proxy denies, and warns when it does. It never fails startup.
func probeDockerSocket(ctx context.Context, log *slog.Logger, prober socketProber, result *atomic.Value) {
	probeCtx, cancel := context.WithTimeout(ctx, socketProbeTimeout)
	defer cancel()
	access, err := prober.ProbeSocketAccess(probeCtx)
	result.Store(string(access))
	switch access {
	case dockerapi.SocketRaw:
		log.Warn("Docker socket answers paths Binnacle never needs (GET /images/json); it is probably the raw daemon socket. Put the constrained socket proxy in front of it", "docker_socket", string(access))
	case dockerapi.SocketUnknown:
		log.Warn("Docker socket access probe failed", "error", err)
	}
}

// databaseReadiness reports "ok" when the reader pool answers a query within
// ctx, "unavailable" otherwise (RUN-6).
func databaseReadiness(ctx context.Context, store *storage.Manager) string {
	reader := store.Reader()
	if reader == nil {
		return "unavailable"
	}
	var one int
	if err := reader.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return "unavailable"
	}
	return "ok"
}

// collectorStates keeps only each collector's state word; reasons stay
// behind a session.
func collectorStates(collectors map[string]metrics.CollectorHealth) map[string]string {
	out := make(map[string]string, len(collectors))
	for name, health := range collectors {
		out[name] = string(health.State)
	}
	return out
}

// Memory limit (RUN-4). The Go GC does not know about the container's memory
// limit, so without a soft limit the heap grows until the kernel's OOM killer
// ends the process. GOMEMLIMIT, when set, wins (the runtime applies it
// itself); otherwise the soft limit is memoryLimitRatio of the cgroup limit,
// leaving headroom for memory the GC does not manage (SQLite's page cache,
// cgo, thread stacks). Without a cgroup limit the Go default (no soft limit)
// stays.
const (
	cgroupRoot       = "/sys/fs/cgroup"
	memoryLimitRatio = 0.9
	// cgroup v1 reports "no limit" as a page-rounded maximum int64.
	cgroupV1Unlimited = int64(1) << 62
)

// cgroupMemoryLimit reads the memory limit of the process's cgroup under
// root: cgroup v2 memory.max first, then cgroup v1 memory.limit_in_bytes.
func cgroupMemoryLimit(root string) (int64, bool) {
	if raw, err := os.ReadFile(filepath.Join(root, "memory.max")); err == nil {
		value := strings.TrimSpace(string(raw))
		if value == "max" {
			return 0, false
		}
		limit, err := strconv.ParseInt(value, 10, 64)
		return limit, err == nil && limit > 0
	}
	if raw, err := os.ReadFile(filepath.Join(root, "memory", "memory.limit_in_bytes")); err == nil {
		limit, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		return limit, err == nil && limit > 0 && limit < cgroupV1Unlimited
	}
	return 0, false
}

// memoryLimit is the soft limit to set, and false when none should be set.
func memoryLimit(env string, root string) (int64, bool) {
	if strings.TrimSpace(env) != "" {
		return 0, false
	}
	limit, ok := cgroupMemoryLimit(root)
	if !ok {
		return 0, false
	}
	return int64(float64(limit) * memoryLimitRatio), true
}

func applyMemoryLimit(log *slog.Logger, root string) {
	env := os.Getenv("GOMEMLIMIT")
	soft, ok := memoryLimit(env, root)
	switch {
	case env != "":
		log.Info("Go memory limit taken from GOMEMLIMIT", "gomemlimit", env)
	case ok:
		debug.SetMemoryLimit(soft)
		log.Info("Go memory limit set from the cgroup limit", "bytes", soft, "ratio", memoryLimitRatio)
	default:
		log.Info("no cgroup memory limit found; Go memory limit left unset")
	}
}
