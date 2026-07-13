// SPDX-License-Identifier: AGPL-3.0-only
package settings

import (
	"strings"
	"testing"
	"time"
)

func TestBinnacleConfigurationNames(t *testing.T) {
	config := Defaults()
	config.Normalize()
	if config.Paths.DataDir != "/var/lib/binnacle" {
		t.Fatalf("data directory = %q", config.Paths.DataDir)
	}
	if config.Paths.DatabasePath != "/var/lib/binnacle/binnacle.db" {
		t.Fatalf("database path = %q", config.Paths.DatabasePath)
	}

	for name := range environment {
		if !strings.HasPrefix(name, "BINNACLE_") {
			t.Fatalf("environment variable %q has the wrong prefix", name)
		}
	}

	const explicit = "/tmp/binnacle.toml"
	path := Discover(func(name string) string {
		if name == "BINNACLE_CONFIG_FILE" {
			return explicit
		}
		return ""
	}, func(string) bool { return false })
	if path != explicit {
		t.Fatalf("discovered config path = %q", path)
	}
}

func TestNotificationDeploymentSettings(t *testing.T) {
	config := Defaults()
	if config.Notifications.MaxConcurrency != 4 || config.Notifications.QueueCapacity != 1000 || config.Notifications.DeliveryTimeout != 15*time.Second || config.Notifications.ReminderInterval != 2*time.Hour || config.Notifications.AllowPrivateTargets {
		t.Fatalf("notification defaults=%+v", config.Notifications)
	}
	values := map[string]string{
		"notifications.allow_private_targets": "true",
		"notifications.max_concurrency":       "8",
		"notifications.queue_capacity":        "250",
		"notifications.delivery_timeout":      "20s",
		"notifications.reminder_interval":     "3h",
	}
	if err := apply(&config, values); err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if !config.Notifications.AllowPrivateTargets || config.Notifications.MaxConcurrency != 8 || config.Notifications.QueueCapacity != 250 || config.Notifications.DeliveryTimeout != 20*time.Second || config.Notifications.ReminderInterval != 3*time.Hour {
		t.Fatalf("notification settings=%+v", config.Notifications)
	}
	for key := range values {
		if UIOverridable(key) {
			t.Fatalf("%s must require restart", key)
		}
	}
	config.Notifications.QueueCapacity = 0
	if err := config.Validate(); err == nil {
		t.Fatal("unbounded notification configuration was accepted")
	}
}
