// SPDX-License-Identifier: AGPL-3.0-only

package settings

import "testing"

func TestHostRootSetting(t *testing.T) {
	if Defaults().Paths.HostRoot != "/" {
		t.Fatalf("default host root %q", Defaults().Paths.HostRoot)
	}
	values := map[string]string{"BINNACLE_HOST_ROOT": "/host/root"}
	config, effective, err := LoadWith(func(key string) string { return values[key] }, func(string) bool { return false }, NoopOverrideProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Paths.HostRoot != "/host/root" {
		t.Fatalf("host root %q", config.Paths.HostRoot)
	}
	if got := effective["paths.host_root"]; got.Source != SourceEnvironment || got.Value != "/host/root" || !got.RestartRequired {
		t.Fatalf("effective host root %+v", got)
	}
	values["BINNACLE_HOST_ROOT"] = "host/root"
	if _, _, err := LoadWith(func(key string) string { return values[key] }, func(string) bool { return false }, NoopOverrideProvider{}); err == nil {
		t.Fatal("relative host root accepted")
	}
}
