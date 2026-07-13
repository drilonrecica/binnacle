// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/drilonrecica/binnacle/internal/checks"
	"strings"
	"time"
)

type CheckRunner struct{}

func (CheckRunner) Run(_ context.Context, c checks.Check) checks.Result {
	r := checks.Result{CheckID: c.ID, Status: "success", CheckedAt: time.Unix(0, 0).UTC(), LatencyMS: 24}
	if strings.HasSuffix(c.ID, "failure") {
		r.Status = "failure"
		r.FailureCode = checks.FailureUnexpectedStatus
	}
	return r
}
func SeedChecksAlerts(ctx context.Context, db *sql.DB, count, resources int, now time.Time) error {
	if count < 1 {
		return nil
	}
	if resources < 1 {
		resources = 1
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO hosts(id,identity_hash,name,updated_at)VALUES('demo-host','demo-host','Demo server',?)`, now.Format(time.RFC3339)); err != nil {
		return err
	}
	for i := 0; i < resources; i++ {
		resource := fmt.Sprintf("res_demo_%d", i+1)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO resources(id,host_id,stable_key,source_kind,name,category,status,first_seen_at,last_seen_at)VALUES(?,'demo-host',?,'demo',?,'service','healthy',?,?)`, resource, resource, fmt.Sprintf("demo-service-%d", i+1), now.UnixMilli(), now.UnixMilli()); err != nil {
			return err
		}
	}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("demo-check-%03d", i+1)
		if i == 1 {
			id = "demo-check-failure"
		}
		resource := fmt.Sprintf("res_demo_%d", i%resources+1)
		status := "success"
		var failure any
		successes, failures := 2, 0
		if i == 1 {
			status = "failure"
			failure = "unexpected_status"
			successes, failures = 0, 4
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,required,enabled,created_at,updated_at)VALUES(?,?,?,?, 'GET',30,5,200,399,?,1,?,?)`, id, resource, fmt.Sprintf("Demo check %d", i+1), fmt.Sprintf("https://demo-%d.invalid/health", i+1), i%2 == 0, now.Unix(), now.Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO health_check_state(check_id,status,failure_code,latency_ms,checked_at,consecutive_successes,consecutive_failures,next_run_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?)`, id, status, failure, 24, now.Unix(), successes, failures, now.Add(30*time.Second).Unix(), now.Unix())
		if err != nil {
			return err
		}
	}
	if count > 1 {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO alert_evaluation_state(dedup_key,rule_id,target_type,target_id,phase,phase_since,last_evaluated_at,observed_value,details_json)VALUES('optional_check_failure:res_demo_2','builtin-optional-check','resource','res_demo_2','firing',?,?,0,'{}')`, now.Add(-3*time.Minute).Unix(), now.Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO alerts(id,dedup_key,rule_id,family,severity,target_type,target_id,status,started_at,last_observed_at,observed_value,message)VALUES('demo-alert','optional_check_failure:res_demo_2','builtin-optional-check','optional_check_failure','warning','resource','res_demo_2','firing',?,?,0,'Demo optional check is failing')`, now.Add(-time.Minute).Unix(), now.Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO incidents(id,group_key,status,severity,target_type,target_id,title,opened_at,updated_at,version,next_reminder_at)VALUES('demo-incident','resource:res_demo_2','open','warning','resource','res_demo_2','Demo service health incident',?,?,1,?)`, now.Add(-time.Minute).Unix(), now.Unix(), now.Add(2*time.Hour).Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO incident_alerts(incident_id,alert_id,joined_at)VALUES('demo-incident','demo-alert',?)`, now.Add(-time.Minute).Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_channels(id,name,kind,enabled,minimum_severity,notify_resolved,config_json,secret_ref,created_at,updated_at)VALUES('demo-channel','Example webhook','webhook',0,'warning',1,'{}','demo.notification.secret',?,?)`, now.Add(-time.Hour).Unix(), now.Add(-time.Hour).Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_deliveries(id,channel_id,incident_id,event_type,payload_json,idempotency_key,status,attempt_count,completed_at,created_at,updated_at)VALUES('demo-delivery','demo-channel','demo-incident','opened','{}','demo-idempotency','succeeded',1,?,?,?)`, now.Add(-50*time.Minute).Unix(), now.Add(-50*time.Minute).Unix(), now.Add(-50*time.Minute).Unix())
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO silences(id,scope_type,scope_id,reason,starts_at,ends_at,created_by,created_at)VALUES('demo-silence','resource','res_demo_3','Demo maintenance',?,?,'demo',?)`, now.Add(-time.Minute).Unix(), now.Add(time.Hour).Unix(), now.Unix())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO deployment_grace_periods(resource_id,starts_at,ends_at,confidence)VALUES('res_demo_1',?,?,'confirmed')`, now.Add(-time.Minute).Unix(), now.Add(4*time.Minute).Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}
