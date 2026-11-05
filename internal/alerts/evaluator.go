// SPDX-License-Identifier: AGPL-3.0-only

package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/observe"
)

type Evaluator struct {
	Repo     *Repository
	Engine   *metrics.Engine
	Interval time.Duration
	Now      func() time.Time
	Logger   *slog.Logger
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	// errlog rate-limits evaluation failures, which repeat every interval
	// while the cause lasts (RUN-7).
	errlog     observe.ErrorLog
	errlogOnce sync.Once
}

// errorLog is the evaluator's rate-limited log; it writes through logger().
func (e *Evaluator) errorLog() *observe.ErrorLog {
	e.errlogOnce.Do(func() { e.errlog.Logger = e.logger })
	return &e.errlog
}

func NewEvaluator(repo *Repository, engine *metrics.Engine) *Evaluator {
	return &Evaluator{Repo: repo, Engine: engine, Interval: 10 * time.Second, Now: time.Now, Logger: slog.Default()}
}
func (e *Evaluator) Start(ctx context.Context) error {
	if e.Repo == nil || e.Engine == nil {
		return errors.New("alert evaluator dependencies unavailable")
	}
	ctx, e.cancel = context.WithCancel(ctx)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.Evaluate(ctx); err != nil && ctx.Err() == nil {
					e.errorLog().Error("alert evaluation failed", "error", err)
				}
			}
		}
	}()
	return nil
}
func (e *Evaluator) Stop(context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	return nil
}

// Evaluate runs one evaluation of every alert family. Families are evaluated
// independently: an error in one is collected and the others still run, so a
// single broken rule or query cannot silence every alert (EVT-5). The returned
// error joins every failure, each prefixed with its family.
func (e *Evaluator) Evaluate(ctx context.Context) error {
	rules, err := e.Repo.Rules(ctx)
	if err != nil {
		return fmt.Errorf("loading alert rules: %w", err)
	}
	snap := e.Engine.Snapshot()
	now := e.Now().UTC()
	var errs []error
	step := func(family string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", family, err))
		}
	}
	step("cleanup", e.Repo.Cleanup(ctx, now))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		switch r.Family {
		case FamilyHostCPU:
			if effectiveRule(rules, r.Family, "host", "server").ID != r.ID {
				continue
			}
			if snap.Host.CPUPercent != nil {
				step(r.Family, e.evaluate(ctx, r, "host", "server", *snap.Host.CPUPercent, *snap.Host.CPUPercent > *r.Threshold, *snap.Host.CPUPercent < *r.RecoveryThreshold, now))
			}
		case FamilyHostMemory:
			if effectiveRule(rules, r.Family, "host", "server").ID != r.ID {
				continue
			}
			if snap.Host.MemoryPercent != nil {
				step(r.Family, e.evaluate(ctx, r, "host", "server", *snap.Host.MemoryPercent, *snap.Host.MemoryPercent > *r.Threshold, *snap.Host.MemoryPercent < *r.RecoveryThreshold, now))
			}
		case FamilyDockerDown:
			if effectiveRule(rules, r.Family, "server", "docker").ID != r.ID {
				continue
			}
			h := snap.Collectors["docker"]
			down := h.State == metrics.CollectorDown
			step(r.Family, e.evaluate(ctx, r, "server", "docker", 0, down, h.State == metrics.CollectorHealthy, now))
		}
	}
	step("filesystems", e.evaluateFilesystems(ctx, rules, snap.Filesystems, now))
	step("events", e.evaluateEvents(ctx, rules, now))
	step(FamilyPersistence, e.evaluatePersistence(ctx, rules, now))
	step("deployments", e.correlateDeployments(ctx, now))
	step("checks", e.evaluateChecks(ctx, rules, now))
	return errors.Join(errs...)
}

func (e *Evaluator) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

func (e *Evaluator) evaluatePersistence(ctx context.Context, rules []Rule, now time.Time) error {
	rule := effectiveRule(rules, FamilyPersistence, "server", "persistence")
	if rule.ID == "" {
		return nil
	}
	var eventType string
	err := e.Repo.db.QueryRowContext(ctx, `SELECT type FROM events WHERE type IN ('persistence_emergency','persistence_degraded','persistence_resumed') ORDER BY ts DESC,rowid DESC LIMIT 1`).Scan(&eventType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	failing := eventType == "persistence_emergency" || eventType == "persistence_degraded"
	return e.evaluate(ctx, rule, "server", "persistence", 0, failing, eventType == "persistence_resumed", now)
}
func (e *Evaluator) evaluateChecks(ctx context.Context, rules []Rule, now time.Time) error {
	rows, err := e.Repo.db.QueryContext(ctx, `SELECT c.id,c.resource_id,c.required,COALESCE(s.status,'unknown'),COALESCE(s.consecutive_successes,0) FROM health_checks c LEFT JOIN health_check_state s ON s.check_id=c.id WHERE c.enabled=1`)
	if err != nil {
		return err
	}
	type observation struct {
		checkID, resource, status string
		required                  bool
		successes                 int
	}
	observations := []observation{}
	for rows.Next() {
		var v observation
		if err = rows.Scan(&v.checkID, &v.resource, &v.required, &v.status, &v.successes); err != nil {
			rows.Close()
			return err
		}
		observations = append(observations, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var errs []error
	for _, v := range observations {
		family := FamilyOptionalCheck
		if v.required {
			family = FamilyRequiredCheck
		}
		rule := effectiveRule(rules, family, "resource", v.resource)
		if rule.ID == "" {
			continue
		}
		failing := v.status == "failure"
		recovered := v.status == "success"
		if v.successes >= 2 {
			rule.RecoveryDuration = 0
		}
		if err := e.evaluate(ctx, rule, "resource", v.resource, 0, failing, recovered, now); err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", family, v.resource, err))
		}
	}
	return errors.Join(errs...)
}

// evaluateFilesystems reads the current values from the engine snapshot, the
// latest collection of every mounted filesystem, instead of a latest-per-mount
// scan of filesystem_samples_1m on every tick (STO-7). A mount that is no
// longer collected is no longer evaluated.
func (e *Evaluator) evaluateFilesystems(ctx context.Context, rules []Rule, filesystems []metrics.FilesystemObservation, now time.Time) error {
	var errs []error
	for _, fs := range filesystems {
		for _, rule := range rules {
			if !rule.Enabled || rule.Threshold == nil || rule.RecoveryThreshold == nil {
				continue
			}
			if effectiveRule(rules, rule.Family, "filesystem", fs.MountKey).ID != rule.ID {
				continue
			}
			if rule.Family == FamilyFilesystemFull {
				value, failing, recovered := fullWithin(fs.Trend, *rule.Threshold, *rule.RecoveryThreshold)
				if err := e.evaluate(ctx, rule, "filesystem", fs.MountKey, value, failing, recovered, now); err != nil {
					errs = append(errs, fmt.Errorf("%s %s: %w", rule.Family, fs.MountKey, err))
				}
				continue
			}
			var value *float64
			switch rule.Family {
			case FamilyFilesystemWarning, FamilyFilesystemCritical:
				value = fs.UsedPercent
			case FamilyInodeWarning, FamilyInodeCritical:
				value = fs.InodesUsedPercent
			default:
				continue
			}
			if value != nil {
				if err := e.evaluate(ctx, rule, "filesystem", fs.MountKey, *value, *value > *rule.Threshold, *value < *rule.RecoveryThreshold, now); err != nil {
					errs = append(errs, fmt.Errorf("%s %s: %w", rule.Family, fs.MountKey, err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// MinFullConfidence is the estimate confidence at or above which the
// filesystem-full family may fire.
const MinFullConfidence = 0.6

// fullWithin evaluates the filesystem-full family against one estimate. It
// fails when the filesystem is filling, would be full in fewer than threshold
// days, and the estimate's confidence is at least MinFullConfidence. It
// recovers when the filesystem stops filling or the projection reaches
// recovery days, never fewer than threshold days: more days is healthier, so
// a recovery below the threshold would let one estimate both fail and
// recover and the alert would flap. An unknown or low-confidence estimate neither fails nor
// recovers, so a firing alert holds until the estimate is trustworthy again.
// The observed value is the projection in days, 0 when there is none.
func fullWithin(t metrics.Trend, threshold, recovery float64) (value float64, failing, recovered bool) {
	switch t.State {
	case metrics.TrendSteady, metrics.TrendShrinking:
		return 0, false, true
	case metrics.TrendFilling:
		if t.DaysToFull == nil {
			return 0, false, false
		}
		days := *t.DaysToFull
		return days, days < threshold && t.Confidence >= MinFullConfidence, days >= math.Max(recovery, threshold)
	}
	return 0, false, false
}

func effectiveRule(rules []Rule, family, targetType, target string) Rule {
	var global Rule
	for _, rule := range rules {
		if !rule.Enabled || rule.Family != family {
			continue
		}
		if rule.ScopeType == targetType && rule.ScopeID == target {
			return rule
		}
		if rule.ScopeType == "global" {
			global = rule
		}
	}
	return global
}

// evaluateEvents evaluates the restart-storm and OOM-loop rules, each on its
// own so that a failing rule does not skip the other (EVT-5).
func (e *Evaluator) evaluateEvents(ctx context.Context, rules []Rule, now time.Time) error {
	var errs []error
	for _, rule := range rules {
		if !rule.Enabled || rule.Threshold == nil || (rule.Family != FamilyRestartStorm && rule.Family != FamilyOOMLoop) {
			continue
		}
		if err := e.evaluateEventRule(ctx, rule, now); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rule.Family, err))
		}
	}
	return errors.Join(errs...)
}

func (e *Evaluator) evaluateEventRule(ctx context.Context, rule Rule, now time.Time) error {
	eventType := "container_restart"
	if rule.Family == FamilyOOMLoop {
		eventType = "container_oom"
	}
	rows, err := e.Repo.db.QueryContext(ctx, `SELECT resource_id,COUNT(*) FROM events WHERE type=? AND resource_id IS NOT NULL AND ts>=? GROUP BY resource_id`, eventType, now.Add(-rule.Window).UnixMilli())
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	counts := map[string]float64{}
	for rows.Next() {
		var resource string
		var count float64
		if err = rows.Scan(&resource, &count); err != nil {
			rows.Close()
			return err
		}
		counts[resource] = count
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for resource, count := range counts {
		seen[resource] = true
		failing := count > *rule.Threshold
		if rule.Family == FamilyOOMLoop {
			failing = count >= *rule.Threshold
		}
		if err = e.evaluate(ctx, rule, "resource", resource, count, failing, count <= 0, now); err != nil {
			return err
		}
	}
	stateRows, err := e.Repo.db.QueryContext(ctx, `SELECT target_id FROM alert_evaluation_state WHERE rule_id=? AND phase!='healthy'`, rule.ID)
	if err != nil {
		return err
	}
	unseen := []string{}
	for stateRows.Next() {
		var resource string
		if err = stateRows.Scan(&resource); err != nil {
			stateRows.Close()
			return err
		}
		if !seen[resource] {
			unseen = append(unseen, resource)
		}
	}
	if err = stateRows.Err(); err != nil {
		stateRows.Close()
		return err
	}
	stateRows.Close()
	for _, resource := range unseen {
		if err = e.evaluate(ctx, rule, "resource", resource, 0, false, true, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) correlateDeployments(ctx context.Context, now time.Time) error {
	rows, err := e.Repo.db.QueryContext(ctx, `SELECT resource_id,MAX(ts),CASE WHEN MAX(CASE WHEN type='deployment' THEN 1 ELSE 0 END)=1 THEN 'confirmed' ELSE 'likely' END FROM events WHERE type IN ('deployment','deployment_likely','container_replacement') AND resource_id IS NOT NULL AND ts>=? GROUP BY resource_id`, now.Add(-10*time.Minute).UnixMilli())
	if err != nil {
		return err
	}
	type deployment struct {
		ms         int64
		confidence string
	}
	deployments := map[string]deployment{}
	for rows.Next() {
		var resource string
		var ms int64
		var confidence string
		if err = rows.Scan(&resource, &ms, &confidence); err != nil {
			rows.Close()
			return err
		}
		deployments[resource] = deployment{ms, confidence}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for resource, deployment := range deployments {
		at := time.UnixMilli(deployment.ms).UTC()
		// The same deployment is correlated on every tick for ten minutes;
		// write the grace period only when it differs from the stored one,
		// and never recreate one that already ended (Cleanup removes those).
		if !at.Add(5 * time.Minute).After(now) {
			continue
		}
		var starts, ends int64
		var confidence string
		err = e.Repo.db.QueryRowContext(ctx, `SELECT starts_at,ends_at,confidence FROM deployment_grace_periods WHERE resource_id=?`, resource).Scan(&starts, &ends, &confidence)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && starts == at.Unix() && ends == at.Add(5*time.Minute).Unix() && confidence == deployment.confidence {
			continue
		}
		if _, err = e.Repo.db.ExecContext(ctx, `INSERT INTO deployment_grace_periods(resource_id,starts_at,ends_at,confidence)VALUES(?,?,?,?) ON CONFLICT(resource_id) DO UPDATE SET starts_at=excluded.starts_at,ends_at=excluded.ends_at,confidence=excluded.confidence`, resource, at.Unix(), at.Add(5*time.Minute).Unix(), deployment.confidence); err != nil {
			return err
		}
	}
	return nil
}
func (e *Evaluator) evaluate(ctx context.Context, rule Rule, targetType, target string, value float64, failing, recovered bool, now time.Time) error {
	key := dedup(rule.Family, target)
	tx, err := e.Repo.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state := State{Phase: Healthy, Since: now}
	var phase, storedRule string
	var since int64
	var notified, cooldown sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT rule_id,phase,phase_since,last_notified_at,cooldown_until FROM alert_evaluation_state WHERE dedup_key=?`, key).Scan(&storedRule, &phase, &since, &notified, &cooldown)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	stored := err == nil
	if stored {
		state.Phase = Phase(phase)
		state.Since = time.Unix(since, 0).UTC()
		if notified.Valid {
			v := time.Unix(notified.Int64, 0).UTC()
			state.LastNotifiedAt = &v
		}
		if cooldown.Valid {
			v := time.Unix(cooldown.Int64, 0).UTC()
			state.CooldownUntil = &v
		}
	}
	suppressed, err := suppressedTx(ctx, tx, rule, target, now)
	if err != nil {
		return err
	}
	tr := Advance(now, state, failing, recovered, suppressed, rule)
	if !stateChanged(stored, storedRule, state, rule.ID, tr.State) {
		// STO-4: an unchanged phase is not rewritten every tick. Nothing
		// reads last_evaluated_at or observed_value back; a firing alert's
		// latest value is kept on its alerts row below.
		if state.Phase == Firing || state.Phase == Recovering {
			if _, err = tx.ExecContext(ctx, `UPDATE alerts SET last_observed_at=?,observed_value=? WHERE dedup_key=? AND status='firing'`, now.Unix(), value, key); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	// JSON has no NaN or Inf, so a non-finite observation is stored as null.
	var observed any = value
	if math.IsNaN(value) || math.IsInf(value, 0) {
		observed = nil
	}
	details, err := json.Marshal(map[string]any{"observed": observed})
	if err != nil {
		return fmt.Errorf("encode evaluation details: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alert_evaluation_state(dedup_key,rule_id,target_type,target_id,phase,phase_since,last_evaluated_at,last_notified_at,cooldown_until,observed_value,details_json)VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(dedup_key) DO UPDATE SET rule_id=excluded.rule_id,phase=excluded.phase,phase_since=excluded.phase_since,last_evaluated_at=excluded.last_evaluated_at,last_notified_at=excluded.last_notified_at,cooldown_until=excluded.cooldown_until,observed_value=excluded.observed_value,details_json=excluded.details_json`, key, rule.ID, targetType, target, tr.State.Phase, tr.State.Since.Unix(), now.Unix(), timePtr(tr.State.LastNotifiedAt), timePtr(tr.State.CooldownUntil), value, string(details))
	if err != nil {
		return err
	}
	if tr.Triggered {
		alertID := id()
		_, err = tx.ExecContext(ctx, `INSERT INTO alerts(id,dedup_key,rule_id,family,severity,target_type,target_id,status,started_at,last_observed_at,observed_value,message)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, alertID, key, rule.ID, rule.Family, rule.Severity, targetType, target, "firing", now.Unix(), now.Unix(), value, message(rule, target))
		if err == nil && e.Repo.sink != nil {
			err = e.Repo.sink.AlertFiredTx(ctx, tx, Alert{ID: alertID, DedupKey: key, RuleID: rule.ID, Family: rule.Family, Severity: rule.Severity, TargetType: targetType, TargetID: target, Status: "firing", StartedAt: now, LastObservedAt: now, ObservedValue: &value, Message: message(rule, target)}, now)
		}
		if err == nil {
			err = insertEvent(ctx, tx, alertID, rule, target, "alert_triggered", now)
		}
	} else if tr.Repeated {
		err = insertEvent(ctx, tx, id(), rule, target, "alert_repeated", now)
	} else if tr.Resolved {
		var alertID string
		err = tx.QueryRowContext(ctx, `SELECT id FROM alerts WHERE dedup_key=? AND status='firing' ORDER BY started_at DESC LIMIT 1`, key).Scan(&alertID)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE alerts SET status='resolved',resolved_at=?,last_observed_at=? WHERE id=?`, now.Unix(), now.Unix(), alertID)
			if err == nil && e.Repo.sink != nil {
				err = e.Repo.sink.AlertResolvedTx(ctx, tx, alertID, now)
			}
			if err == nil {
				err = insertEvent(ctx, tx, alertID, rule, target, "alert_resolved", now)
			}
		}
	} else if state.Phase == Firing || state.Phase == Recovering {
		_, err = tx.ExecContext(ctx, `UPDATE alerts SET last_observed_at=?,observed_value=? WHERE dedup_key=? AND status='firing'`, now.Unix(), value, key)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// stateChanged reports whether an evaluation must write the state row: the
// row is missing and the target is no longer healthy, the effective rule
// changed, or the transition moved the phase or its timestamps. A missing row
// reads as healthy everywhere, so a healthy target never gets one.
func stateChanged(stored bool, storedRule string, previous State, ruleID string, next State) bool {
	if !stored {
		return next.Phase != Healthy || next.LastNotifiedAt != nil || next.CooldownUntil != nil
	}
	return storedRule != ruleID || previous.Phase != next.Phase || !previous.Since.Equal(next.Since) || !sameTime(previous.LastNotifiedAt, next.LastNotifiedAt) || !sameTime(previous.CooldownUntil, next.CooldownUntil)
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func suppressedTx(ctx context.Context, tx *sql.Tx, rule Rule, target string, now time.Time) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM silences s WHERE s.starts_at<=? AND s.ends_at>? AND ((s.scope_type='server') OR (s.scope_type='resource' AND s.scope_id=?) OR (s.scope_type='rule' AND s.scope_id=?) OR (s.scope_type='project' AND EXISTS(SELECT 1 FROM resources r WHERE r.id=? AND r.project_name=s.scope_id)))`, now.Unix(), now.Unix(), target, rule.ID, target).Scan(&n)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if rule.SuppressDuringDeployment {
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployment_grace_periods WHERE resource_id=? AND starts_at<=? AND ends_at>?`, target, now.Unix(), now.Unix()).Scan(&n)
	}
	return n > 0, err
}
func insertEvent(ctx context.Context, tx *sql.Tx, eventID string, rule Rule, target, eventType string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO events(id,ts,resource_id,type,severity,summary,correlation_key,source,created_at)VALUES(?,?,?,?,?,?,?,?,?)`, "alert-"+eventID, now.UnixMilli(), nullableTarget(target), eventType, rule.Severity, message(rule, target), dedup(rule.Family, target), "alerts", now.UnixMilli())
	return err
}
func nullableTarget(v string) any {
	if v == "server" || v == "docker" {
		return nil
	}
	return v
}
func timePtr(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Unix()
}
func (e *Evaluator) DeploymentGrace(ctx context.Context, resource, confidence string, at time.Time) error {
	if confidence != "confirmed" && confidence != "likely" {
		return fmt.Errorf("invalid deployment confidence")
	}
	_, err := e.Repo.db.ExecContext(ctx, `INSERT INTO deployment_grace_periods(resource_id,starts_at,ends_at,confidence)VALUES(?,?,?,?) ON CONFLICT(resource_id) DO UPDATE SET starts_at=excluded.starts_at,ends_at=excluded.ends_at,confidence=excluded.confidence`, resource, at.Unix(), at.Add(5*time.Minute).Unix(), confidence)
	return err
}
