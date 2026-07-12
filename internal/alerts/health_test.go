// SPDX-License-Identifier: AGPL-3.0-only
package alerts_test

import (
	"context"
	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
	"path/filepath"
	"testing"
	"time"
)

func healthStore(t *testing.T) (*storage.Manager, *alerts.Repository) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.DB()
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO hosts(id,identity_hash,name,updated_at)VALUES('host','host','host',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO resources(id,host_id,stable_key,source_kind,name,category,status,first_seen_at,last_seen_at)VALUES('res_test','host','test','docker','test','application','healthy',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	repo := alerts.NewRepository(db)
	if err := repo.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	return store, repo
}
func TestHealthOverlayRequiredCheckPrecedence(t *testing.T) {
	store, repo := healthStore(t)
	now := time.Now().Unix()
	if _, err := store.DB().Exec(`INSERT INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,required,enabled,created_at,updated_at)VALUES('check','res_test','check','https://example.com','GET',30,5,200,399,1,1,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	snapshot := metrics.Snapshot{Resources: []metrics.ResourceSnapshot{{ID: "res_test", Status: metrics.StatusHealthy}}}
	decorated := repo.Decorate(context.Background(), snapshot)
	if decorated.Resources[0].SignalStatus != metrics.StatusHealthy || decorated.Resources[0].Status != metrics.StatusUnknown {
		t.Fatalf("unknown overlay=%+v", decorated.Resources[0])
	}
	if _, err := store.DB().Exec(`INSERT INTO alert_evaluation_state(dedup_key,rule_id,target_type,target_id,phase,phase_since,last_evaluated_at,details_json)VALUES('required_check_failure:res_test','builtin-required-check','resource','res_test','firing',?,?, '{}')`, now, now); err != nil {
		t.Fatal(err)
	}
	decorated = repo.Decorate(context.Background(), snapshot)
	if decorated.Resources[0].Status != metrics.StatusDown {
		t.Fatalf("firing status=%s", decorated.Resources[0].Status)
	}
	snapshot.Resources[0].Status = metrics.StatusPaused
	decorated = repo.Decorate(context.Background(), snapshot)
	if decorated.Resources[0].Status != metrics.StatusPaused {
		t.Fatalf("paused overridden: %s", decorated.Resources[0].Status)
	}
}
