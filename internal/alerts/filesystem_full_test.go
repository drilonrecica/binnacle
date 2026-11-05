// SPDX-License-Identifier: AGPL-3.0-only
package alerts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

func fullTrend(days, confidence float64) metrics.Trend {
	return metrics.Trend{State: metrics.TrendFilling, DaysToFull: &days, Confidence: confidence, WindowHours: 24}
}

// P3-12: filesystem_full_within_days fires a warning when a filesystem is
// projected full in under the threshold (7 days) with confidence of at least
// 0.6, holds while the estimate is unknown or unconfident, and resolves once
// the filesystem stops filling.
func TestFilesystemFullWithinDays(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return at }
	hostCPU(engine, 10)
	used := 50.0
	engine.PublishFilesystems([]metrics.FilesystemObservation{{MountKey: "data", MountPoint: "/data", UsedPercent: &used}})
	phase := func() string {
		t.Helper()
		var p string
		err := store.DB().QueryRowContext(ctx, `SELECT phase FROM alert_evaluation_state WHERE dedup_key='filesystem_full_within_days:data'`).Scan(&p)
		if errors.Is(err, sql.ErrNoRows) {
			return "healthy"
		}
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	step := func(trend metrics.Trend, advance time.Duration) string {
		t.Helper()
		at = at.Add(advance)
		engine.SetTrends(24, metrics.UnknownTrend(24), map[string]metrics.Trend{"data": trend})
		if err := evaluator.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
		return phase()
	}

	if got := step(fullTrend(3, 0.5), 0); got != "healthy" {
		t.Fatalf("low confidence: phase=%s", got)
	}
	if got := step(fullTrend(10, 0.9), time.Minute); got != "healthy" {
		t.Fatalf("beyond threshold: phase=%s", got)
	}
	if got := step(fullTrend(3, 0.9), time.Minute); got != "pending" {
		t.Fatalf("confident 3 days: phase=%s", got)
	}
	if got := step(fullTrend(3, 0.9), 15*time.Minute); got != "firing" {
		t.Fatalf("after the trigger duration: phase=%s", got)
	}
	var severity string
	var observed float64
	if err := store.DB().QueryRowContext(ctx, `SELECT severity,observed_value FROM alerts WHERE dedup_key='filesystem_full_within_days:data' AND status='firing'`).Scan(&severity, &observed); err != nil || severity != "warning" || observed != 3 {
		t.Fatalf("alert severity=%s observed=%v err=%v", severity, observed, err)
	}
	if got := step(metrics.UnknownTrend(24), time.Minute); got != "firing" {
		t.Fatalf("unknown estimate must hold: phase=%s", got)
	}
	if got := step(fullTrend(5, 0.3), time.Minute); got != "firing" {
		t.Fatalf("unconfident estimate must hold: phase=%s", got)
	}
	if got := step(metrics.Trend{State: metrics.TrendSteady, Confidence: 0.9, WindowHours: 24}, time.Minute); got != "recovering" {
		t.Fatalf("steady: phase=%s", got)
	}
	// From recovering the generic lifecycle resolves after the recovery
	// duration; that step is not driven here because resolution currently
	// fails for every family (DSC-12).
	if got := step(metrics.Trend{State: metrics.TrendShrinking, Confidence: 0.9, WindowHours: 24}, time.Minute); got != "recovering" {
		t.Fatalf("shrinking: phase=%s", got)
	}
	if got := step(fullTrend(3, 0.9), time.Minute); got != "firing" {
		t.Fatalf("filling again while recovering: phase=%s", got)
	}
	if got := step(fullTrend(9, 0.9), time.Minute); got != "recovering" {
		t.Fatalf("projection past the recovery threshold: phase=%s", got)
	}
}

// The family follows the filesystem scoping of the other filesystem rules:
// a scoped rule replaces the global one for its mount.
func TestFilesystemFullScopedRule(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	threshold := 30.0
	if err := repo.CreateRule(ctx, alerts.Rule{ID: "scoped-full", Family: alerts.FamilyFilesystemFull, Name: "Data full within a month", Enabled: true, Severity: alerts.Critical, ScopeType: "filesystem", ScopeID: "data", Threshold: &threshold, RecoveryThreshold: &threshold}); err != nil {
		t.Fatal(err)
	}
	engine := metrics.NewEngine(4)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	engine.PublishFilesystems([]metrics.FilesystemObservation{{MountKey: "data"}, {MountKey: "root"}})
	engine.SetTrends(24, metrics.UnknownTrend(24), map[string]metrics.Trend{"data": fullTrend(20, 0.9), "root": fullTrend(20, 0.9)})
	if err := evaluator.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	var rule, phase string
	if err := store.DB().QueryRowContext(ctx, `SELECT rule_id,phase FROM alert_evaluation_state WHERE dedup_key='filesystem_full_within_days:data'`).Scan(&rule, &phase); err != nil || rule != "scoped-full" || phase != "pending" {
		t.Fatalf("data rule=%s phase=%s err=%v", rule, phase, err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT phase FROM alert_evaluation_state WHERE dedup_key='filesystem_full_within_days:root'`).Scan(&phase); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("root under the 7-day global rule: phase=%s err=%v", phase, err)
	}
}

// Raising the threshold above an unchanged recovery threshold (a partial rule
// update) must not make one estimate both fail and recover.
func TestFilesystemFullRecoveryNeverBelowThreshold(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	threshold := 14.0
	rules, err := repo.Rules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		if rule.Family == alerts.FamilyFilesystemFull {
			rule.Threshold = &threshold // recovery stays at 7
			if err := repo.UpdateRule(ctx, rule); err != nil {
				t.Fatal(err)
			}
		}
	}
	engine := metrics.NewEngine(4)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return at }
	engine.PublishFilesystems([]metrics.FilesystemObservation{{MountKey: "data"}})
	engine.SetTrends(24, metrics.UnknownTrend(24), map[string]metrics.Trend{"data": fullTrend(10, 0.9)})
	for _, advance := range []time.Duration{0, 16 * time.Minute, time.Minute} {
		at = at.Add(advance)
		if err := evaluator.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var phase string
	if err := store.DB().QueryRowContext(ctx, `SELECT phase FROM alert_evaluation_state WHERE dedup_key='filesystem_full_within_days:data'`).Scan(&phase); err != nil || phase != "firing" {
		t.Fatalf("phase=%s err=%v, want firing to hold at 10 days under a 14-day threshold", phase, err)
	}
}
