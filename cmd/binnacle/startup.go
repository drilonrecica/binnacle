// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
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
