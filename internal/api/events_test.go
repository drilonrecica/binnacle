// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestEventsSupportFiltersAndCursor(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	now := time.Now().UTC()
	for index := 0; index < 6; index++ {
		severity := "info"
		eventType := "container_start"
		if index%3 == 0 {
			severity = "critical"
			eventType = "container_oom"
		}
		ts := now.Add(-time.Duration(index) * time.Minute).UnixMilli()
		if _, err := manager.DB().ExecContext(ctx, "INSERT INTO events(id,ts,type,severity,summary,resource_id,source,created_at) VALUES(?,?,?,?,?,?,?,?)", fmt.Sprintf("event-%d", index), ts, eventType, severity, "summary", "res_1", "docker", ts); err != nil {
			t.Fatal(err)
		}
	}
	server := New()
	server.EnableEvents(manager, DemoAuthorizer(true), auth.NewProtection(4096, auth.TrustedProxies{}))
	fetch := func(path string) ([]struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
		Type     string `json:"type"`
	}, int) {
		request := httptest.NewRequest(http.MethodGet, "http://binnacle.test"+path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		var rows []struct {
			ID       string `json:"id"`
			Severity string `json:"severity"`
			Type     string `json:"type"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &rows)
		return rows, response.Code
	}

	rows, code := fetch("/api/v1/events?severity=critical")
	if code != 200 || len(rows) != 2 || rows[0].Severity != "critical" {
		t.Fatalf("severity filter code=%d rows=%+v", code, rows)
	}
	rows, code = fetch("/api/v1/events?type=container_oom,container_stop")
	if code != 200 || len(rows) != 2 || rows[0].Type != "container_oom" {
		t.Fatalf("type filter code=%d rows=%+v", code, rows)
	}
	rows, code = fetch("/api/v1/events?limit=2")
	if code != 200 || len(rows) != 2 || rows[0].ID != "event-0" {
		t.Fatalf("limit code=%d rows=%+v", code, rows)
	}
	rows, code = fetch("/api/v1/events?limit=2&before=" + rows[1].ID)
	if code != 200 || len(rows) != 2 || rows[0].ID != "event-2" {
		t.Fatalf("cursor code=%d rows=%+v", code, rows)
	}
	for _, path := range []string{"/api/v1/events?severity=bogus", "/api/v1/events?limit=0", "/api/v1/events?limit=501"} {
		if _, code = fetch(path); code != 400 {
			t.Errorf("%s code=%d", path, code)
		}
	}
}
