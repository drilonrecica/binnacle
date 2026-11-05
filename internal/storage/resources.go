// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Resource struct {
	ID              string     `json:"id"`
	HostID          string     `json:"-"`
	StableKey       string     `json:"-"`
	SourceKind      string     `json:"sourceKind"`
	Name            string     `json:"name"`
	Context         string     `json:"context,omitempty"`
	Category        string     `json:"category"`
	Status          string     `json:"status"`
	ProjectName     string     `json:"project,omitempty"`
	EnvironmentName string     `json:"environment,omitempty"`
	ArchivedAt      *time.Time `json:"archivedAt,omitempty"`
}

func (m *Manager) UpsertResource(ctx context.Context, r Resource) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	return upsertResource(ctx, m.db, r, time.Now())
}

func upsertResource(ctx context.Context, q querier, r Resource, at time.Time) error {
	now := at.UnixMilli()
	_, e := q.ExecContext(ctx, "INSERT INTO resources(id,host_id,stable_key,source_kind,name,context,project_name,environment_name,category,status,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(host_id,stable_key) DO UPDATE SET name=excluded.name,context=excluded.context,project_name=excluded.project_name,environment_name=excluded.environment_name,category=excluded.category,status=excluded.status,archived_at=NULL,last_seen_at=excluded.last_seen_at", r.ID, r.HostID, r.StableKey, r.SourceKind, r.Name, r.Context, nullable(r.ProjectName), nullable(r.EnvironmentName), r.Category, r.Status, now, now)
	return e
}

func (m *Manager) ArchiveMissingResources(ctx context.Context, active []string, before time.Time) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	return archiveMissingResources(ctx, m.db, active, before, time.Now())
}

func archiveMissingResources(ctx context.Context, q querier, active []string, before, at time.Time) error {
	query := "UPDATE resources SET status='archived',archived_at=? WHERE status<>'archived' AND last_seen_at<?"
	args := []any{at.UTC().UnixMilli(), before.UTC().UnixMilli()}
	if len(active) > 0 {
		query += " AND id NOT IN (" + strings.TrimRight(strings.Repeat("?,", len(active)), ",") + ")"
		for _, id := range active {
			args = append(args, id)
		}
	}
	_, err := q.ExecContext(ctx, query, args...)
	return err
}

// Inventory is the host and resource identities one collector pass saw.
type Inventory struct {
	HostID, HostIdentity, HostName string
	Resources                      []Resource
	// ArchiveBefore archives resources not in Resources whose last_seen_at
	// is older than it.
	ArchiveBefore time.Time
}

// StoreInventory upserts the host and every resource and archives the
// missing ones in one write transaction (STO-4), instead of one transaction
// per row.
func (m *Manager) StoreInventory(ctx context.Context, inv Inventory) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	if err = upsertHost(ctx, tx, inv.HostID, inv.HostIdentity, inv.HostName, now); err != nil {
		return fmt.Errorf("host: %w", err)
	}
	active := make([]string, 0, len(inv.Resources))
	for _, r := range inv.Resources {
		if err = upsertResource(ctx, tx, r, now); err != nil {
			return fmt.Errorf("resource %s: %w", r.ID, err)
		}
		active = append(active, r.ID)
	}
	if err = archiveMissingResources(ctx, tx, active, inv.ArchiveBefore, now); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	return tx.Commit()
}

func (m *Manager) ArchivedResources(ctx context.Context) ([]Resource, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT id,host_id,stable_key,source_kind,name,COALESCE(context,''),COALESCE(project_name,''),COALESCE(environment_name,''),category,status,archived_at FROM resources WHERE status='archived' ORDER BY archived_at DESC,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Resource
	for rows.Next() {
		var resource Resource
		var archived int64
		if err = rows.Scan(&resource.ID, &resource.HostID, &resource.StableKey, &resource.SourceKind, &resource.Name, &resource.Context, &resource.ProjectName, &resource.EnvironmentName, &resource.Category, &resource.Status, &archived); err != nil {
			return nil, err
		}
		value := time.UnixMilli(archived).UTC()
		resource.ArchivedAt = &value
		result = append(result, resource)
	}
	return result, rows.Err()
}

func (m *Manager) Resource(ctx context.Context, id string) (Resource, error) {
	var resource Resource
	var archived sql.NullInt64
	err := m.db.QueryRowContext(ctx, "SELECT id,host_id,stable_key,source_kind,name,COALESCE(context,''),COALESCE(project_name,''),COALESCE(environment_name,''),category,status,archived_at FROM resources WHERE id=?", id).Scan(&resource.ID, &resource.HostID, &resource.StableKey, &resource.SourceKind, &resource.Name, &resource.Context, &resource.ProjectName, &resource.EnvironmentName, &resource.Category, &resource.Status, &archived)
	if archived.Valid {
		value := time.UnixMilli(archived.Int64).UTC()
		resource.ArchivedAt = &value
	}
	return resource, err
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (m *Manager) ArchiveResource(ctx context.Context, id string) error {
	_, err := m.db.ExecContext(ctx, "UPDATE resources SET status='archived', archived_at=? WHERE id=?", time.Now().UnixMilli(), id)
	return err
}
