// SPDX-License-Identifier: AGPL-3.0-only

package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func testRepository(t *testing.T) (*storage.Manager, *Repository) {
	t.Helper()
	ctx := context.Background()
	store := storage.New(filepath.Join(t.TempDir(), "binnacle.db"), filepath.Join(t.TempDir(), "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	secrets, err := auth.NewSecretStore(store.DB(), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return store, NewRepository(store.DB(), secrets)
}

func insertRule(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO alert_rules(id,family,name,built_in,enabled,severity,scope_type,scope_id,trigger_seconds,recovery_seconds,created_at,updated_at) VALUES('rule','test','Test',0,1,'warning','resource','r',0,0,1,1)`)
	if err != nil {
		t.Fatal(err)
	}
}

func fire(t *testing.T, repo *Repository, a alerts.Alert, at time.Time) {
	t.Helper()
	tx, err := repo.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO alerts(id,dedup_key,rule_id,family,severity,target_type,target_id,status,started_at,last_observed_at,message) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, a.ID, a.DedupKey, "rule", a.Family, a.Severity, a.TargetType, a.TargetID, "firing", at.Unix(), at.Unix(), a.Message)
	if err == nil {
		err = repo.AlertFiredTx(context.Background(), tx, a, at)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestIncidentGroupingAndLifecycle(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	now := time.Now().UTC().Truncate(time.Second)
	fire(t, repo, alerts.Alert{ID: "a1", DedupKey: "one", Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "resource-1", Message: "warning"}, now)
	fire(t, repo, alerts.Alert{ID: "a2", DedupKey: "two", Family: "test", Severity: alerts.Critical, TargetType: "resource", TargetID: "resource-1", Message: "critical"}, now.Add(time.Second))
	list, err := repo.Incidents(context.Background(), "open", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AlertCount != 2 || list[0].Severity != "critical" {
		t.Fatalf("unexpected grouped incident: %+v", list)
	}
	resolve := func(id string, at time.Time) {
		tx, e := repo.db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		_, e = tx.Exec(`UPDATE alerts SET status='resolved',resolved_at=? WHERE id=?`, at.Unix(), id)
		if e == nil {
			e = repo.AlertResolvedTx(context.Background(), tx, id, at)
		}
		if e == nil {
			e = tx.Commit()
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	resolve("a2", now.Add(2*time.Second))
	list, _ = repo.Incidents(context.Background(), "open", "", 50, 0)
	if len(list) != 1 || list[0].Severity != "warning" {
		t.Fatalf("incident resolved or failed to demote early: %+v", list)
	}
	resolve("a1", now.Add(3*time.Second))
	list, _ = repo.Incidents(context.Background(), "resolved", "", 50, 0)
	if len(list) != 1 || list[0].FiringCount != 0 {
		t.Fatalf("incident not resolved: %+v", list)
	}
	fire(t, repo, alerts.Alert{ID: "a3", DedupKey: "three", Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "resource-1", Message: "later"}, now.Add(4*time.Second))
	list, _ = repo.Incidents(context.Background(), "", "", 50, 0)
	if len(list) != 2 {
		t.Fatalf("sequential alert reused resolved incident: %+v", list)
	}
}

func TestGroupKeys(t *testing.T) {
	cases := []struct {
		name string
		a    alerts.Alert
		want string
	}{{"resource", alerts.Alert{TargetType: "resource", TargetID: "r"}, "resource:r"}, {"check", alerts.Alert{TargetType: "check", TargetID: "r"}, "resource:r"}, {"host", alerts.Alert{TargetType: "host", TargetID: "server"}, "host:server"}, {"filesystem", alerts.Alert{TargetType: "filesystem", TargetID: "/data"}, "filesystem:/data"}, {"docker", alerts.Alert{Family: alerts.FamilyDockerDown, TargetType: "resource", TargetID: "docker"}, "subsystem:docker"}, {"persistence", alerts.Alert{Family: alerts.FamilyPersistence, TargetType: "server", TargetID: "persistence"}, "subsystem:persistence"}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := groupFor(tt.a)
			if got != tt.want {
				t.Fatalf("group=%q want %q", got, tt.want)
			}
		})
	}
}

func TestReconcileFiringAlertsWithoutRetroactiveDelivery(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	ctx := context.Background()
	_, err := repo.CreateChannel(ctx, Channel{Name: "Existing", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, err = store.DB().Exec(`INSERT INTO alerts(id,dedup_key,rule_id,family,severity,target_type,target_id,status,started_at,last_observed_at,message) VALUES('existing-alert','existing','rule','test','warning','resource','existing','firing',?,?, 'Existing firing alert')`, started.Unix(), started.Unix())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err = repo.Reconcile(ctx, now); err != nil {
		t.Fatal(err)
	}
	var opened, next int64
	var memberships, deliveries int
	if err = store.DB().QueryRow(`SELECT opened_at,next_reminder_at FROM incidents WHERE status='open'`).Scan(&opened, &next); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM incident_alerts WHERE alert_id='existing-alert'`).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if opened != started.Unix() || next != now.Add(2*time.Hour).Unix() || memberships != 1 || deliveries != 0 {
		t.Fatalf("opened=%d next=%d memberships=%d deliveries=%d", opened, next, memberships, deliveries)
	}
}

func TestHexMasterKey(t *testing.T) {
	store, _ := testRepository(t)
	defer store.Close()
	if _, err := auth.NewSecretStore(store.DB(), "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryCoalescingFilteringAndTransientCancellation(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	ctx := context.Background()
	warning, err := repo.CreateChannel(ctx, Channel{Name: "Warning", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/hook"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateChannel(ctx, Channel{Name: "Critical", Kind: "webhook", Enabled: true, MinimumSeverity: "critical", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/critical"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	fire(t, repo, alerts.Alert{ID: "coalesce-1", DedupKey: "coalesce-1", Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "coalesce", Message: "one"}, now)
	var count int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("warning filter count=%d err=%v", count, err)
	}
	fire(t, repo, alerts.Alert{ID: "coalesce-2", DedupKey: "coalesce-2", Family: "test", Severity: alerts.Critical, TargetType: "resource", TargetID: "coalesce", Message: "two"}, now.Add(time.Second))
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("coalesced/escalated count=%d err=%v", count, err)
	}
	var payload, event string
	if err = store.DB().QueryRow(`SELECT payload_json,event_type FROM notification_deliveries WHERE channel_id=?`, warning.ID).Scan(&payload, &event); err != nil {
		t.Fatal(err)
	}
	if event != "opened" || !strings.Contains(payload, `"eventType":"opened"`) || !strings.Contains(payload, `"alertCount":2`) {
		t.Fatalf("opening delivery was not coalesced correctly: event=%s payload=%s", event, payload)
	}

	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE alerts SET status='resolved',resolved_at=? WHERE id='coalesce-1'`, now.Add(2*time.Second).Unix())
	if err == nil {
		err = repo.AlertResolvedTx(ctx, tx, "coalesce-1", now.Add(2*time.Second))
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE alerts SET status='resolved',resolved_at=? WHERE id='coalesce-2'`, now.Add(3*time.Second).Unix())
	}
	if err == nil {
		err = repo.AlertResolvedTx(ctx, tx, "coalesce-2", now.Add(30*time.Second))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries WHERE status='cancelled'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("transient deliveries cancelled=%d err=%v", count, err)
	}
}

func TestChannelResponsesAreSanitized(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	ctx := context.Background()
	_, err := repo.CreateChannel(ctx, Channel{Name: "Secret", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://secret.example.test/path", BearerToken: "bearer-secret", SigningSecret: "signing-secret"})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := repo.Channels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded := fmt.Sprint(channels)
	for _, secret := range []string{"secret.example.test", "bearer-secret", "signing-secret"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("channel response leaked %q: %s", secret, encoded)
		}
	}
}

func TestWebhookPayloadBoundsMemberAlerts(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	ctx := context.Background()
	channel, err := repo.CreateChannel(ctx, Channel{Name: "Payload", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("payload-%02d", i)
		fire(t, repo, alerts.Alert{ID: id, DedupKey: id, Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "payload", Message: id}, now.Add(time.Duration(i)*time.Second))
	}
	var payload string
	if err = store.DB().QueryRow(`SELECT payload_json FROM notification_deliveries WHERE channel_id=?`, channel.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Incident Incident `json:"incident"`
	}
	if err = json.Unmarshal([]byte(payload), &body); err != nil {
		t.Fatal(err)
	}
	if body.Incident.AlertCount != 25 || len(body.Incident.Alerts) != 20 {
		t.Fatalf("alertCount=%d payload members=%d", body.Incident.AlertCount, len(body.Incident.Alerts))
	}
}

func TestDatabaseEnforcesChannelLimit(t *testing.T) {
	store, _ := testRepository(t)
	defer store.Close()
	now := time.Now().Unix()
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("channel-%02d", i)
		if _, err := store.DB().Exec(`INSERT INTO notification_channels(id,name,kind,enabled,minimum_severity,notify_resolved,config_json,secret_ref,created_at,updated_at) VALUES(?,?,'webhook',0,'warning',1,'{}',?,?,?)`, id, id, id, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DB().Exec(`INSERT INTO notification_channels(id,name,kind,enabled,minimum_severity,notify_resolved,config_json,secret_ref,created_at,updated_at) VALUES('channel-overflow','overflow','webhook',0,'warning',1,'{}','overflow',?,?)`, now, now); err == nil {
		t.Fatal("database accepted more than 32 active channels")
	}
}

func TestRestartRecoveryAndManualRetryPreserveIdempotency(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	ctx := context.Background()
	channel, err := repo.CreateChannel(ctx, Channel{Name: "Recovery", Kind: "webhook", Enabled: false, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/hook"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	_, err = store.DB().Exec(`INSERT INTO notification_deliveries(id,channel_id,event_type,payload_json,idempotency_key,status,attempt_count,started_at,created_at,updated_at)VALUES('recover-delivery',?,'test','{}','stable-key','in_progress',2,?,?,?)`, channel.ID, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(repo, Config{MaxConcurrency: 1, QueueCapacity: 1})
	if err = worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_ = worker.Stop(ctx)
	var status, key string
	if err = store.DB().QueryRow(`SELECT status,idempotency_key FROM notification_deliveries WHERE id='recover-delivery'`).Scan(&status, &key); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || key != "stable-key" {
		t.Fatalf("recovered status=%s key=%s", status, key)
	}
	_, err = store.DB().Exec(`UPDATE notification_deliveries SET status='permanent_failure',completed_at=? WHERE id='recover-delivery'`, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Retry(ctx, "recover-delivery"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err = store.DB().QueryRow(`SELECT status,idempotency_key,attempt_count FROM notification_deliveries WHERE id='recover-delivery'`).Scan(&status, &key, &attempts); err != nil || status != "pending" || key != "stable-key" || attempts != 0 {
		t.Fatalf("retry status=%s key=%s attempts=%d err=%v", status, key, attempts, err)
	}
}

func TestChannelDeleteCancelsPendingDelivery(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	ctx := context.Background()
	channel, err := repo.CreateChannel(ctx, Channel{Name: "Delete", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := repo.Test(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.DeleteChannel(ctx, channel.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	var completed sql.NullInt64
	if err = store.DB().QueryRow(`SELECT status,completed_at FROM notification_deliveries WHERE id=?`, id).Scan(&status, &completed); err != nil || status != "cancelled" || !completed.Valid {
		t.Fatalf("status=%s completed=%v err=%v", status, completed, err)
	}
}

func TestResolutionOnlyReachesPreviouslyAttemptedChannels(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	ctx := context.Background()
	notified, err := repo.CreateChannel(ctx, Channel{Name: "Notified", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/notified"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	fire(t, repo, alerts.Alert{ID: "resolution-1", DedupKey: "resolution-1", Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "resolution", Message: "one"}, now)
	if _, err = store.DB().Exec(`UPDATE notification_deliveries SET status='succeeded',attempt_count=1,completed_at=? WHERE channel_id=?`, now.Unix(), notified.ID); err != nil {
		t.Fatal(err)
	}
	late, err := repo.CreateChannel(ctx, Channel{Name: "Late", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/late"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	resolvedAt := now.Add(time.Minute)
	if _, err = tx.Exec(`UPDATE alerts SET status='resolved',resolved_at=? WHERE id='resolution-1'`, resolvedAt.Unix()); err == nil {
		err = repo.AlertResolvedTx(ctx, tx, "resolution-1", resolvedAt)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
	var notifiedResolutions, lateResolutions int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries WHERE channel_id=? AND event_type='resolved'`, notified.ID).Scan(&notifiedResolutions); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries WHERE channel_id=? AND event_type='resolved'`, late.ID).Scan(&lateResolutions); err != nil {
		t.Fatal(err)
	}
	if notifiedResolutions != 1 || lateResolutions != 0 {
		t.Fatalf("resolution deliveries: notified=%d late=%d", notifiedResolutions, lateResolutions)
	}
}

func TestReminderScheduleUsesConfiguredInterval(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	insertRule(t, store.DB())
	ctx := context.Background()
	_, err := repo.CreateChannel(ctx, Channel{Name: "Reminder", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test/reminder"})
	if err != nil {
		t.Fatal(err)
	}
	interval := 37 * time.Minute
	NewWorker(repo, Config{ReminderInterval: interval})
	now := time.Now().UTC().Truncate(time.Second)
	fire(t, repo, alerts.Alert{ID: "reminder-1", DedupKey: "reminder-1", Family: "test", Severity: alerts.Warning, TargetType: "resource", TargetID: "reminder", Message: "one"}, now)
	var incidentID string
	var next int64
	if err = store.DB().QueryRow(`SELECT id,next_reminder_at FROM incidents WHERE status='open'`).Scan(&incidentID, &next); err != nil {
		t.Fatal(err)
	}
	if next != now.Add(interval).Unix() {
		t.Fatalf("next reminder=%s want %s", time.Unix(next, 0), now.Add(interval))
	}
	if _, err = store.DB().Exec(`UPDATE incidents SET next_reminder_at=? WHERE id=?`, now.Add(-time.Second).Unix(), incidentID); err != nil {
		t.Fatal(err)
	}
	if err = repo.ScheduleReminders(ctx, now, interval); err != nil {
		t.Fatal(err)
	}
	var reminders int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries WHERE incident_id=? AND event_type='reminder'`, incidentID).Scan(&reminders); err != nil || reminders != 1 {
		t.Fatalf("reminders=%d err=%v", reminders, err)
	}
}

func TestDisablingChannelCancelsPendingDeliveries(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	ctx := context.Background()
	channel, err := repo.CreateChannel(ctx, Channel{Name: "Disable", Kind: "webhook", Enabled: true, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := repo.Test(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel.Enabled = false
	if _, err = repo.PatchChannel(ctx, channel.ID, channel, SecretPatch{}); err != nil {
		t.Fatal(err)
	}
	var status string
	var completed sql.NullInt64
	if err = store.DB().QueryRow(`SELECT status,completed_at FROM notification_deliveries WHERE id=?`, id).Scan(&status, &completed); err != nil || status != "cancelled" || !completed.Valid {
		t.Fatalf("status=%s completed=%v err=%v", status, completed, err)
	}
}

func TestCleanupRetentionBoundaries(t *testing.T) {
	store, repo := testRepository(t)
	defer store.Close()
	ctx := context.Background()
	channel, err := repo.CreateChannel(ctx, Channel{Name: "Retention", Kind: "webhook", Enabled: false, MinimumSeverity: "warning", NotifyResolved: true}, ChannelSecrets{URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, row := range []struct {
		id string
		at time.Time
	}{{"old-delivery", now.Add(-91 * 24 * time.Hour)}, {"new-delivery", now.Add(-89 * 24 * time.Hour)}} {
		_, err = store.DB().Exec(`INSERT INTO notification_deliveries(id,channel_id,event_type,payload_json,idempotency_key,status,completed_at,created_at,updated_at) VALUES(?,?,'test','{}',?,'succeeded',?,?,?)`, row.id, channel.ID, row.id+"-key", row.at.Unix(), row.at.Unix(), row.at.Unix())
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = repo.Cleanup(ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM notification_deliveries`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("retained deliveries=%d err=%v", count, err)
	}
	for _, row := range []struct {
		id string
		at time.Time
	}{{"old-incident", now.Add(-366 * 24 * time.Hour)}, {"new-incident", now.Add(-364 * 24 * time.Hour)}} {
		_, err = store.DB().Exec(`INSERT INTO incidents(id,group_key,status,severity,target_type,target_id,title,opened_at,updated_at,resolved_at,version) VALUES(?,?,'resolved','warning','resource',?,'Retention incident',?,?,?,1)`, row.id, "resource:"+row.id, row.id, row.at.Unix(), row.at.Unix(), row.at.Unix())
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = repo.Cleanup(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM incidents`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("retained incidents=%d err=%v", count, err)
	}
}
