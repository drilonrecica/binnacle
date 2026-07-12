// SPDX-License-Identifier: AGPL-3.0-only

package alerts

import (
	"context"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

// Decorate applies current alert/check health without mutating the Metrics Engine snapshot.
func (r *Repository) Decorate(ctx context.Context, snapshot metrics.Snapshot) metrics.Snapshot {
	out := snapshot
	out.Resources = append([]metrics.ResourceSnapshot(nil), snapshot.Resources...)
	type health struct{ requiredUnknown, requiredPending, requiredFiring, optionalFailure, resourceAlert bool }
	states := map[string]*health{}
	for i := range out.Resources {
		out.Resources[i].SignalStatus = out.Resources[i].Status
		states[string(out.Resources[i].ID)] = &health{}
	}
	if r == nil || r.db == nil {
		return out
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.resource_id,c.required,COALESCE(s.status,'unknown'),COALESCE(e.phase,'healthy') FROM health_checks c LEFT JOIN health_check_state s ON s.check_id=c.id LEFT JOIN alert_evaluation_state e ON e.dedup_key=(CASE WHEN c.required=1 THEN 'required_check_failure:' ELSE 'optional_check_failure:' END)||c.resource_id WHERE c.enabled=1`)
	if err == nil {
		for rows.Next() {
			var resource, status, phase string
			var required bool
			if rows.Scan(&resource, &required, &status, &phase) == nil {
				h := states[resource]
				if h == nil {
					continue
				}
				if required && status == "unknown" {
					h.requiredUnknown = true
				}
				if required && (phase == string(Pending) || phase == string(Recovering)) {
					h.requiredPending = true
				}
				if required && phase == string(Firing) {
					h.requiredFiring = true
				}
				if !required && (status == "failure" || phase == string(Firing) || phase == string(Pending)) {
					h.optionalFailure = true
				}
			}
		}
		rows.Close()
	}
	alertRows, err := r.db.QueryContext(ctx, `SELECT DISTINCT target_id FROM alerts WHERE status='firing' AND target_type='resource' AND family NOT IN (?,?)`, FamilyRequiredCheck, FamilyOptionalCheck)
	if err == nil {
		for alertRows.Next() {
			var id string
			if alertRows.Scan(&id) == nil && states[id] != nil {
				states[id].resourceAlert = true
			}
		}
		alertRows.Close()
	}
	for i := range out.Resources {
		v := &out.Resources[i]
		if v.SignalStatus == metrics.StatusPaused || v.SignalStatus == metrics.StatusArchived || v.SignalStatus == metrics.StatusDown {
			continue
		}
		h := states[string(v.ID)]
		if h == nil {
			continue
		}
		switch {
		case h.requiredFiring:
			v.Status = metrics.StatusDown
		case h.requiredPending || h.optionalFailure || h.resourceAlert:
			v.Status = metrics.StatusDegraded
		case h.requiredUnknown:
			v.Status = metrics.StatusUnknown
		}
	}
	return out
}
