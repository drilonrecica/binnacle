// SPDX-License-Identifier: AGPL-3.0-only

package settings

import (
	"strings"
	"testing"
)

// P3-12: estimator.window_hours defaults to 24, loads from the environment,
// is editable live, and is refused outside 6 to 168 hours.
func TestEstimatorWindowHours(t *testing.T) {
	if got := Defaults().Estimator.WindowHours; got != 24 {
		t.Fatalf("default window=%d, want 24", got)
	}
	config, _, err := LoadWith(func(name string) string {
		if name == "BINNACLE_ESTIMATOR_WINDOW_HOURS" {
			return "48"
		}
		return ""
	}, func(string) bool { return false }, nil)
	if err != nil || config.Estimator.WindowHours != 48 {
		t.Fatalf("environment window=%d err=%v", config.Estimator.WindowHours, err)
	}
	if editable["estimator.window_hours"] != ApplyLive || !UIOverridable("estimator.window_hours") {
		t.Fatal("estimator.window_hours is not editable live")
	}
	for _, value := range []string{"5", "169", "0", "day"} {
		if _, err := ResolveOverrides(Defaults(), map[string]string{"estimator.window_hours": value}); err == nil {
			t.Fatalf("window %q accepted", value)
		}
	}
	for _, value := range []string{"6", "168"} {
		resolved, err := ResolveOverrides(Defaults(), map[string]string{"estimator.window_hours": value})
		if err != nil || lookup(resolved, "estimator.window_hours") != value {
			t.Fatalf("window %q: resolved=%d err=%v", value, resolved.Estimator.WindowHours, err)
		}
	}
	bad := Defaults()
	bad.Estimator.WindowHours = 200
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "estimator.window_hours") {
		t.Fatalf("validate err=%v", err)
	}
}
