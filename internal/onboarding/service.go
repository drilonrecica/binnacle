// SPDX-License-Identifier: AGPL-3.0-only
package onboarding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/drilonrecica/binnacle/internal/diagnostics"
)

type State struct {
	ExposureMode       string                    `json:"exposureMode,omitempty"`
	RetentionPreset    string                    `json:"retentionPreset,omitempty"`
	Diagnostics        []diagnostics.CheckResult `json:"diagnostics,omitempty"`
	CompletedAt        *time.Time                `json:"completedAt,omitempty"`
	ChecklistDismissed bool                      `json:"checklistDismissed"`
}

type Service struct {
	db        *sql.DB
	checker   diagnostics.OnboardingChecker
	retention RetentionSettings
	now       func() time.Time
}

type RetentionSettings interface {
	CurrentRetentionPreset() string
	SetRetentionPreset(context.Context, string, string) error
}

func New(db *sql.DB, checker diagnostics.OnboardingChecker) *Service {
	return &Service{db: db, checker: checker, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SetDB(db *sql.DB) {
	s.db = db
	s.checker.DB = db
}
func (s *Service) SetRetentionSettings(settings RetentionSettings) { s.retention = settings }
func (s *Service) SetDocker(client diagnostics.DockerDiagnostics)  { s.checker.Docker = client }

func (s *Service) State(ctx context.Context) (State, error) {
	if s == nil || s.db == nil {
		return State{}, errors.New("onboarding storage is unavailable")
	}
	var state State
	var exposure, retention, raw sql.NullString
	var completed, dismissed sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT exposure_mode,retention_preset,diagnostics_json,completed_at,checklist_dismissed_at FROM onboarding_state WHERE id=1").Scan(&exposure, &retention, &raw, &completed, &dismissed)
	if errors.Is(err, sql.ErrNoRows) {
		if s.retention != nil {
			state.RetentionPreset = s.retention.CurrentRetentionPreset()
		}
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	state.ExposureMode, state.RetentionPreset = exposure.String, retention.String
	if s.retention != nil {
		state.RetentionPreset = s.retention.CurrentRetentionPreset()
	}
	state.ChecklistDismissed = dismissed.Valid
	if raw.Valid && raw.String != "" {
		if err = json.Unmarshal([]byte(raw.String), &state.Diagnostics); err != nil {
			return State{}, err
		}
	}
	if completed.Valid {
		value := time.UnixMilli(completed.Int64).UTC()
		state.CompletedAt = &value
	}
	return state, nil
}

func (s *Service) Update(ctx context.Context, retention, actor string) (State, error) {
	if retention != "minimal" && retention != "balanced" && retention != "long-term" {
		return State{}, errors.New("retention preset is invalid")
	}
	if s.retention == nil {
		return State{}, errors.New("retention settings are unavailable")
	}
	if err := s.retention.SetRetentionPreset(ctx, retention, actor); err != nil {
		return State{}, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO onboarding_state(id,retention_preset,updated_at) VALUES(1,?,?)
		ON CONFLICT(id) DO UPDATE SET retention_preset=excluded.retention_preset,updated_at=excluded.updated_at`, retention, s.now().UnixMilli())
	if err != nil {
		return State{}, err
	}
	return s.State(ctx)
}

func (s *Service) Diagnose(ctx context.Context, outbound bool) (State, error) {
	results := s.checker.Run(ctx, outbound)
	raw, err := json.Marshal(results)
	if err != nil {
		return State{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO onboarding_state(id,diagnostics_json,updated_at) VALUES(1,?,?)
		ON CONFLICT(id) DO UPDATE SET diagnostics_json=excluded.diagnostics_json,updated_at=excluded.updated_at`, string(raw), s.now().UnixMilli())
	if err != nil {
		return State{}, err
	}
	return s.State(ctx)
}

func (s *Service) Complete(ctx context.Context) (State, error) {
	state, err := s.State(ctx)
	if err != nil {
		return State{}, err
	}
	if state.RetentionPreset == "" || len(state.Diagnostics) == 0 {
		return State{}, errors.New("onboarding choices and diagnostics are required")
	}
	_, err = s.db.ExecContext(ctx, "UPDATE onboarding_state SET completed_at=COALESCE(completed_at,?),updated_at=? WHERE id=1", s.now().UnixMilli(), s.now().UnixMilli())
	if err != nil {
		return State{}, err
	}
	return s.State(ctx)
}

func (s *Service) DismissChecklist(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE onboarding_state SET checklist_dismissed_at=COALESCE(checklist_dismissed_at,?),updated_at=? WHERE id=1 AND completed_at IS NOT NULL", s.now().UnixMilli(), s.now().UnixMilli())
	return err
}
