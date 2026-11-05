// SPDX-License-Identifier: AGPL-3.0-only

package settings

import (
	"strings"
	"testing"
)

func TestPublicURLLoadsFromEnvironment(t *testing.T) {
	values := map[string]string{
		"BINNACLE_PUBLIC_URL":          " https://binnacle.example.test/ ",
		"BINNACLE_TRUSTED_PROXY_CIDRS": "172.18.0.0/16, 10.0.0.0/8",
	}
	config, effective, err := LoadWith(func(key string) string { return values[key] }, func(string) bool { return false }, NoopOverrideProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTP.PublicURL != "https://binnacle.example.test" {
		t.Fatalf("public url=%q", config.HTTP.PublicURL)
	}
	if got := effective["http.public_url"]; got.Source != SourceEnvironment || got.Value != "https://binnacle.example.test" || !got.RestartRequired {
		t.Fatalf("effective public url=%+v", got)
	}
	if len(config.HTTP.TrustedProxyCIDRs) != 2 || config.HTTP.TrustedProxyCIDRs[0] != "172.18.0.0/16" {
		t.Fatalf("trusted proxies=%v", config.HTTP.TrustedProxyCIDRs)
	}
	if UIOverridable("http.public_url") {
		t.Fatal("public URL must require a deployment change")
	}
}

func TestPublicURLValidation(t *testing.T) {
	tests := []struct {
		name, value string
		wantErr     bool
	}{
		{"empty", "", false},
		{"https origin", "https://binnacle.example.test", false},
		{"http with port", "http://binnacle.lan:8080", false},
		{"missing scheme", "binnacle.example.test", true},
		{"other scheme", "ftp://binnacle.example.test", true},
		{"userinfo", "https://admin:pw@binnacle.example.test", true},
		{"query", "https://binnacle.example.test/?a=1", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Defaults()
			config.HTTP.PublicURL = test.value
			err := config.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error=%v, wantErr=%v", err, test.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "http.public_url") {
				t.Fatalf("error does not name the key: %v", err)
			}
		})
	}
}

func TestTrustedProxyCIDRsAcceptNetworksButNotEverything(t *testing.T) {
	config := Defaults()
	config.HTTP.TrustedProxyCIDRs = []string{"172.16.0.0/12", "fd00::/8"}
	if err := config.Validate(); err != nil {
		t.Fatalf("network prefixes rejected: %v", err)
	}
	config.HTTP.TrustedProxyCIDRs = []string{"0.0.0.0/0"}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "http.trusted_proxy_cidrs") {
		t.Fatalf("/0 accepted or unnamed: %v", err)
	}
}
