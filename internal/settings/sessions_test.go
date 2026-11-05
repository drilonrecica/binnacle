// SPDX-License-Identifier: AGPL-3.0-only

package settings

import (
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
)

func TestSessionLifetimeCeilings(t *testing.T) {
	tests := []struct {
		name     string
		idle     time.Duration
		absolute time.Duration
		want     string
	}{
		{"defaults within ceilings", Defaults().Sessions.IdleTimeout, Defaults().Sessions.AbsoluteLifetime, ""},
		{"ceilings themselves", auth.MaxSessionIdleTimeout, auth.MaxSessionAbsoluteLifetime, ""},
		{"idle above seven days", auth.MaxSessionIdleTimeout + time.Second, auth.MaxSessionAbsoluteLifetime, "sessions.idle_timeout must be at most"},
		{"absolute above ninety days", time.Hour, auth.MaxSessionAbsoluteLifetime + time.Second, "sessions.absolute_lifetime must be at most"},
		{"absolute below idle", 2 * time.Hour, time.Hour, "sessions.absolute_lifetime must be at least idle_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Defaults()
			config.Sessions = Sessions{IdleTimeout: test.idle, AbsoluteLifetime: test.absolute}
			err := config.Validate()
			switch {
			case test.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)):
				t.Fatalf("err=%v want %q", err, test.want)
			}
		})
	}
}

func TestSessionOverridesAboveCeilingsAreRefused(t *testing.T) {
	if _, err := ResolveOverrides(Defaults(), map[string]string{"sessions.idle_timeout": "192h"}); err == nil {
		t.Fatal("idle override above the ceiling accepted")
	}
	if _, err := ResolveOverrides(Defaults(), map[string]string{"sessions.absolute_lifetime": "2184h"}); err == nil {
		t.Fatal("absolute override above the ceiling accepted")
	}
	resolved, err := ResolveOverrides(Defaults(), map[string]string{"sessions.idle_timeout": "168h", "sessions.absolute_lifetime": "2160h"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Sessions.IdleTimeout != 168*time.Hour || resolved.Sessions.AbsoluteLifetime != 2160*time.Hour {
		t.Fatalf("resolved=%+v", resolved.Sessions)
	}
}
