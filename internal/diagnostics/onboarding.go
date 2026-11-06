// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type CheckStatus string

const (
	CheckPassed  CheckStatus = "passed"
	CheckWarning CheckStatus = "warning"
	CheckFailed  CheckStatus = "failed"
	CheckNotRun  CheckStatus = "not_run"
)

type CheckResult struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Status          CheckStatus `json:"status"`
	Required        bool        `json:"required"`
	Reason          string      `json:"reason"`
	SuggestedFix    string      `json:"suggestedFix,omitempty"`
	TechnicalDetail string      `json:"technicalDetail,omitempty"`
	// Synthetic marks a check whose source demo mode replaces with generated
	// data. The status is passed because nothing is broken; the interface
	// shows "synthetic data" instead of a pass, never a failure (P4-12).
	Synthetic bool `json:"synthetic,omitempty"`
}

type DockerDiagnostics interface {
	List(context.Context) ([]dockerapi.Container, error)
	Inspect(context.Context, string) (dockerapi.Inspect, error)
	Diagnostics(context.Context) (dockerapi.Diagnostics, error)
}

type OnboardingChecker struct {
	HostProc        string
	HostSys         string
	DataDir         string
	DB              *storage.DB
	Docker          DockerDiagnostics
	HTTPClient      *http.Client
	OutboundURL     string
	ReadFile        func(string) ([]byte, error)
	PersistentProbe func(string) error
	// Demo is set in demo mode, where host metrics, cgroups and Docker are
	// synthetic: those checks report that instead of probing real sources
	// that demo mode never reads. Storage and network are still checked.
	Demo bool
}

func (c OnboardingChecker) Run(ctx context.Context, includeOutbound bool) []CheckResult {
	if c.ReadFile == nil {
		c.ReadFile = os.ReadFile
	}
	if c.PersistentProbe == nil {
		c.PersistentProbe = persistentProbe
	}
	if c.Demo {
		return []CheckResult{
			synthetic("host_metrics", "Host metrics access", true, "Demo mode: host CPU, memory and disks are synthetic data; /proc is not read."),
			synthetic("docker_api", "Docker API access", true, "Demo mode: containers are synthetic data; the Docker API is not queried."),
			synthetic("cgroup", "cgroup access", false, "Demo mode: container resource usage is synthetic data; cgroups are not read."),
			synthetic("deployment_metadata", "Compose/Coolify detection", false, "Demo mode: projects and environments come from the synthetic catalogue."),
			c.persistence(),
			c.database(ctx),
			c.outbound(ctx, includeOutbound),
		}
	}
	return []CheckResult{
		c.hostMetrics(),
		c.dockerAPI(ctx),
		c.cgroup(),
		c.metadata(ctx),
		c.persistence(),
		c.database(ctx),
		c.outbound(ctx, includeOutbound),
	}
}

func (c OnboardingChecker) hostMetrics() CheckResult {
	result := base("host_metrics", "Host metrics access", true)
	for _, name := range []string{"stat", "meminfo"} {
		if _, err := c.ReadFile(filepath.Join(c.HostProc, name)); err != nil {
			return failed(result, "Binnacle cannot read Linux host metrics.", "Verify the read-only host /proc mount and BINNACLE_HOST_PROC.", err)
		}
	}
	result.Status, result.Reason = CheckPassed, "Host CPU and memory interfaces are readable."
	return result
}

func (c OnboardingChecker) dockerAPI(ctx context.Context) CheckResult {
	result := base("docker_api", "Docker API access", true)
	if c.Docker == nil {
		return failed(result, "Docker access is not configured.", "Mount the Docker socket or configure a restricted socket proxy.", nil)
	}
	request, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	detail, err := c.Docker.Diagnostics(request)
	if err != nil {
		return failed(result, "Binnacle cannot query the Docker API.", "Verify the Docker socket mount, proxy allowlist, and socket permissions.", err)
	}
	result.Status, result.Reason = CheckPassed, fmt.Sprintf("Docker API is reachable; %d containers discovered.", detail.Containers)
	return result
}

func (c OnboardingChecker) cgroup() CheckResult {
	result := base("cgroup", "cgroup access", false)
	if _, err := c.ReadFile(filepath.Join(c.HostSys, "fs/cgroup/cgroup.controllers")); err != nil {
		return failed(result, "cgroup v2 information is unavailable.", "Verify the read-only host /sys mount and cgroup v2 availability.", err)
	}
	result.Status, result.Reason = CheckPassed, "cgroup v2 interfaces are readable."
	return result
}

func (c OnboardingChecker) metadata(ctx context.Context) CheckResult {
	result := base("deployment_metadata", "Compose/Coolify detection", false)
	if c.Docker == nil {
		result.Status, result.Reason = CheckWarning, "Metadata detection is unavailable without Docker access."
		return result
	}
	containers, err := c.Docker.List(ctx)
	if err != nil {
		return failed(result, "Container metadata could not be inspected.", "Restore Docker API access; plain host monitoring can continue.", err)
	}
	if len(containers) > 50 {
		containers = containers[:50]
	}
	compose, coolify := false, false
	for _, container := range containers {
		value, inspectErr := c.Docker.Inspect(ctx, container.ID)
		if inspectErr != nil {
			continue
		}
		compose = compose || value.Labels["com.docker.compose.project"] != ""
		coolify = coolify || value.Labels["coolify.managed"] != "" || value.Labels["coolify.resource.uuid"] != ""
	}
	switch {
	case coolify:
		result.Status, result.Reason = CheckPassed, "Coolify-managed container metadata was detected."
	case compose:
		result.Status, result.Reason = CheckPassed, "Docker Compose metadata was detected."
	default:
		result.Status, result.Reason = CheckWarning, "No Compose or Coolify labels were detected; containers remain visible as unmanaged resources."
	}
	return result
}

func (c OnboardingChecker) persistence() CheckResult {
	result := base("persistent_storage", "Persistent storage", true)
	if err := c.PersistentProbe(c.DataDir); err != nil {
		return failed(result, "The Binnacle data directory is not writable.", "Verify the persistent volume mount, ownership, and free disk space.", err)
	}
	result.Status, result.Reason = CheckPassed, "The persistent data directory is writable."
	return result
}

func (c OnboardingChecker) database(ctx context.Context) CheckResult {
	result := base("database", "Database initialization", true)
	if c.DB == nil {
		return failed(result, "SQLite is not initialized.", "Review the startup log and database recovery guidance.", nil)
	}
	if err := c.DB.PingContext(ctx); err != nil {
		return failed(result, "SQLite did not respond to an integrity probe.", "Review storage permissions, disk space, and corruption recovery guidance.", err)
	}
	result.Status, result.Reason = CheckPassed, "SQLite is initialized and responsive."
	return result
}

func (c OnboardingChecker) outbound(ctx context.Context, include bool) CheckResult {
	result := base("outbound_network", "Outbound network availability", false)
	if !include {
		result.Status, result.Reason = CheckNotRun, "Optional outbound check was not requested."
		return result
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	target := c.OutboundURL
	if target == "" {
		target = "https://github.com"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err == nil {
		var response *http.Response
		response, err = client.Do(request)
		if response != nil {
			// A HEAD that answered proves reachability; a failed close is
			// reported with it.
			err = errors.Join(err, response.Body.Close())
		}
	}
	if err != nil {
		return failed(result, "Outbound HTTPS is unavailable; core monitoring remains functional.", "Verify DNS, firewall, and proxy settings if release checks are desired.", err)
	}
	result.Status, result.Reason = CheckPassed, "Outbound HTTPS is available."
	return result
}

func base(id, name string, required bool) CheckResult {
	return CheckResult{ID: id, Name: name, Required: required}
}

func synthetic(id, name string, required bool, reason string) CheckResult {
	result := base(id, name, required)
	result.Status, result.Reason, result.Synthetic = CheckPassed, reason, true
	return result
}

func failed(result CheckResult, reason, fix string, err error) CheckResult {
	result.Status, result.Reason, result.SuggestedFix = CheckFailed, reason, fix
	if err != nil {
		result.TechnicalDetail = sanitizeDetail(err.Error())
	}
	return result
}

func persistentProbe(dir string) error {
	if dir == "" {
		return errors.New("data directory is empty")
	}
	file, err := os.CreateTemp(dir, ".binnacle-write-probe-")
	if err != nil {
		return err
	}
	name := file.Name()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if removeErr := os.Remove(name); err == nil {
		err = removeErr
	}
	return err
}

var (
	ipDetail    = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	secretQuery = regexp.MustCompile(`(?i)(token|password|secret|key)=([^&\s]+)`)
)

func sanitizeDetail(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, value)
	if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
		parsed.User = nil
		value = parsed.String()
	}
	value = secretQuery.ReplaceAllString(value, "$1=[redacted]")
	value = ipDetail.ReplaceAllString(value, "[ip redacted]")
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}
