// SPDX-License-Identifier: AGPL-3.0-only
package settings

import "testing"

// RUN-8: settings that never had an effect were removed. An existing config
// file or stored override that still names them must not break an upgrade.
func TestRetiredKeysAreIgnoredNotFatal(t *testing.T) {
	for key := range retiredKeys {
		if _, ok := effective(Defaults(), nil)[key]; ok {
			t.Fatalf("%s is retired but still listed as a setting", key)
		}
		if UIOverridable(key) && editable[key] != "" {
			t.Fatalf("%s is retired but still editable", key)
		}
	}

	resolved, err := ResolveOverrides(Defaults(), map[string]string{
		"database.warning_ratio": "0.5", "database.critical_ratio": "0.6",
		"database.emergency_pause_ratio": "0.7", "charts.max_points_per_series": "300",
		"collection.host_interval": "3s",
	})
	if err != nil || resolved.Collection.HostInterval.String() != "3s" {
		t.Fatalf("resolved=%+v err=%v", resolved.Collection, err)
	}
}
