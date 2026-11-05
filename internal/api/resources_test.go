// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestResourceDetailRouteServesLiveAndMissingResources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	engine := metrics.NewEngine(10)
	engine.Publish(metrics.Snapshot{At: time.Now().UTC(), Resources: []metrics.ResourceSnapshot{{ID: "res_active", Name: "API", Status: metrics.StatusHealthy, Category: "application", Project: "shop", Environment: "production"}}})
	server := New()
	server.EnableResources(engine, DemoAuthorizer(true), manager, auth.NewProtection(4096, auth.TrustedProxies{}))

	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/resources/res_active", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var resource metrics.ResourceSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &resource); err != nil || resource.Name != "API" || resource.Project != "shop" {
		t.Fatalf("resource=%+v err=%v", resource, err)
	}

	for _, path := range []string{"/api/v1/resources/res_missing", "/api/v1/resources/", "/api/v1/resources/res_active/extra"} {
		request = httptest.NewRequest(http.MethodGet, "http://binnacle.test"+path, nil)
		response = httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != 404 {
			t.Errorf("%s status=%d", path, response.Code)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/resources", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	var list []metrics.ResourceSnapshot
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list) != 1 {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFilesystemsEndpointAndSnapshotCarryMounts(t *testing.T) {
	engine := metrics.NewEngine(10)
	total, used := int64(80<<30), int64(42<<30)
	engine.PublishFilesystems([]metrics.FilesystemObservation{{MountKey: "root", MountPoint: "/", FSType: "ext4", TotalBytes: &total, UsedBytes: &used}})
	engine.Publish(metrics.Snapshot{At: time.Now().UTC()})
	if got := engine.Snapshot().Filesystems; len(got) != 1 || got[0].MountPoint != "/" {
		t.Fatalf("snapshot filesystems=%+v", got)
	}
	server := New()
	server.EnableCurrent(engine, DemoAuthorizer(true))
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/filesystems", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	var mounts []metrics.FilesystemObservation
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &mounts) != nil || len(mounts) != 1 || mounts[0].FSType != "ext4" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	empty := metrics.NewEngine(10)
	server = New()
	server.EnableCurrent(empty, DemoAuthorizer(true))
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != "[]\n" && response.Body.String() != "[]" {
		t.Fatalf("empty status=%d body=%q", response.Code, response.Body.String())
	}
}

// STO-9: the archived list is paginated with limit and a before cursor, the
// same convention as events.
func TestArchivedResourcesEndpointPaginates(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.UpsertHost(ctx, "host", storage.HostIdentity("", "test"), "Server"); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"res_a", "res_b", "res_c"} {
		if err := manager.UpsertResource(ctx, storage.Resource{ID: id, HostID: "host", StableKey: id, SourceKind: "compose", Name: id, Category: "service", Status: "healthy"}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.DB().ExecContext(ctx, "UPDATE resources SET status='archived',archived_at=? WHERE id=?", time.Now().Add(-time.Duration(3-i)*time.Hour).UnixMilli(), id); err != nil {
			t.Fatal(err)
		}
	}
	server := New()
	server.EnableResources(metrics.NewEngine(10), DemoAuthorizer(true), manager, auth.NewProtection(4096, auth.TrustedProxies{}))
	get := func(query string) []storage.Resource {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/resources?"+query, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		var page []storage.Resource
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatalf("%s status=%d body=%s", query, response.Code, response.Body.String())
		}
		return page
	}
	first := get("state=archived&limit=2")
	if len(first) != 2 || first[0].ID != "res_c" || first[1].ID != "res_b" {
		t.Fatalf("first page=%+v", first)
	}
	second := get("state=archived&limit=2&before=" + first[1].ID)
	if len(second) != 1 || second[0].ID != "res_a" {
		t.Fatalf("second page=%+v", second)
	}
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/resources?state=archived&limit=0", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatalf("limit=0 status=%d body=%s", response.Code, response.Body.String())
	}
}
