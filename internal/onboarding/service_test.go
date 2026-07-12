// SPDX-License-Identifier: AGPL-3.0-only
package onboarding

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/drilonrecica/binnacle/internal/diagnostics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type retentionSettings struct {
	current string
	actor   string
}

func (s *retentionSettings) CurrentRetentionPreset() string { return s.current }
func (s *retentionSettings) SetRetentionPreset(_ context.Context, preset, actor string) error {
	s.current, s.actor = preset, actor
	return nil
}

func TestOnboardingPersistsAndCompletesDespiteDiagnosticFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	checker := diagnostics.OnboardingChecker{
		HostProc: "/missing", HostSys: "/missing", DataDir: dir, DB: manager.DB(),
		ReadFile: func(string) ([]byte, error) { return nil, errors.New("unavailable") },
	}
	service := New(manager.DB(), checker)
	retention := &retentionSettings{current: "minimal"}
	service.SetRetentionSettings(retention)
	initial, err := service.State(ctx)
	if err != nil || initial.RetentionPreset != "minimal" {
		t.Fatalf("initial=%+v err=%v", initial, err)
	}
	if _, err := service.Update(ctx, "balanced", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if retention.current != "balanced" || retention.actor != "usr_admin" {
		t.Fatalf("retention=%+v", retention)
	}
	retention.current = "long-term"
	state, err := service.State(ctx)
	if err != nil || state.RetentionPreset != "long-term" {
		t.Fatalf("settings retention not reflected: state=%+v err=%v", state, err)
	}
	state, err = service.Diagnose(ctx, false)
	if err != nil || len(state.Diagnostics) != 7 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	state, err = service.Complete(ctx)
	if err != nil || state.CompletedAt == nil {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if err = service.DismissChecklist(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = service.State(ctx)
	if err != nil || !state.ChecklistDismissed {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

func TestOnboardingCompletionDoesNotRequireExposureMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	service := New(manager.DB(), diagnostics.OnboardingChecker{DataDir: dir, DB: manager.DB()})
	service.SetRetentionSettings(&retentionSettings{current: "balanced"})
	if _, err := service.Update(ctx, "balanced", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Diagnose(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, err := service.Complete(ctx)
	if err != nil || state.CompletedAt == nil || state.ExposureMode != "" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
