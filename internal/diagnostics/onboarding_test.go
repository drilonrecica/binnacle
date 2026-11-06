// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type fakeDocker struct{ fail bool }

func (f fakeDocker) Diagnostics(context.Context) (dockerapi.Diagnostics, error) {
	if f.fail {
		return dockerapi.Diagnostics{}, errors.New("permission denied token=top-secret at 192.0.2.8")
	}
	return dockerapi.Diagnostics{Containers: 1}, nil
}
func (f fakeDocker) List(context.Context) ([]dockerapi.Container, error) {
	return []dockerapi.Container{{ID: "container-one"}}, nil
}
func (f fakeDocker) Inspect(context.Context, string) (dockerapi.Inspect, error) {
	return dockerapi.Inspect{Labels: map[string]string{"com.docker.compose.project": "project"}}, nil
}

func TestOnboardingDiagnosticsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := db.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checker := OnboardingChecker{
		HostProc: "/host/proc", HostSys: "/host/sys", DataDir: dir, DB: db.DB(), Docker: fakeDocker{fail: true},
		ReadFile: func(path string) ([]byte, error) {
			if path == "/host/sys/fs/cgroup/cgroup.controllers" {
				return nil, errors.New("missing")
			}
			return []byte("ok"), nil
		},
	}
	results := checker.Run(ctx, false)
	byID := map[string]CheckResult{}
	for _, result := range results {
		byID[result.ID] = result
	}
	if byID["host_metrics"].Status != CheckPassed || byID["database"].Status != CheckPassed || byID["docker_api"].Status != CheckFailed || byID["cgroup"].Status != CheckFailed {
		t.Fatalf("results=%+v", results)
	}
	if detail := byID["docker_api"].TechnicalDetail; detail == "" || detail == "permission denied token=top-secret at 192.0.2.8" {
		t.Fatalf("unsafe detail=%q", detail)
	}
	if byID["outbound_network"].Status != CheckNotRun {
		t.Fatal("outbound check ran without consent")
	}
}

func TestMetadataDetectionPassesWithCompose(t *testing.T) {
	checker := OnboardingChecker{Docker: fakeDocker{}}
	result := checker.metadata(context.Background())
	if result.Status != CheckPassed {
		t.Fatalf("result=%+v", result)
	}
}

// TestDemoModeReportsSyntheticSources: demo mode has no Docker client and
// never reads /proc, so those checks must say "synthetic" rather than fail,
// while storage is still checked for real (P4-12).
func TestDemoModeReportsSyntheticSources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := db.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checker := OnboardingChecker{
		Demo: true, HostProc: "/missing/proc", HostSys: "/missing/sys", DataDir: dir, DB: db.DB(),
		PersistentProbe: func(string) error { return errors.New("read-only") },
	}
	byID := map[string]CheckResult{}
	for _, result := range checker.Run(ctx, false) {
		byID[result.ID] = result
	}
	for _, id := range []string{"host_metrics", "docker_api", "cgroup", "deployment_metadata"} {
		result := byID[id]
		if result.Status == CheckFailed || !result.Synthetic || !strings.Contains(result.Reason, "synthetic") {
			t.Errorf("%s = %+v, want a synthetic, non-failed result", id, result)
		}
	}
	if result := byID["persistent_storage"]; result.Status != CheckFailed || result.Synthetic {
		t.Errorf("persistent_storage = %+v, want the real probe's failure", result)
	}
	if result := byID["database"]; result.Status != CheckPassed || result.Synthetic {
		t.Errorf("database = %+v, want a real pass", result)
	}
	if byID["outbound_network"].Status != CheckNotRun {
		t.Fatal("outbound check ran without consent")
	}
}
