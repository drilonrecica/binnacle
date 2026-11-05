// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"net/http/httptest"
	"testing"
)

func TestParseTrustedProxiesAcceptsNetworkPrefixes(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"172.18.0.0/16", " 10.0.0.0/8 ", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://binnacle.test", nil)
	for _, peer := range []string{"172.18.0.7:4000", "10.9.8.7:4000", "[fd00::1]:4000"} {
		r.RemoteAddr = peer
		if !proxies.TrustedPeer(r) {
			t.Fatalf("peer %s inside a trusted prefix was not trusted", peer)
		}
	}
	r.RemoteAddr = "203.0.113.5:4000"
	if proxies.TrustedPeer(r) {
		t.Fatal("peer outside every prefix was trusted")
	}
}

func TestParseTrustedProxiesRejectsTrustEverything(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "::/0", "not-a-prefix", "10.0.0.1"} {
		if _, err := ParseTrustedProxies([]string{value}); err == nil {
			t.Fatalf("%q accepted", value)
		}
	}
}

func TestParseExactProxiesRequiresOneHost(t *testing.T) {
	if _, err := ParseExactProxies([]string{"10.0.0.2/32", "2001:db8::2/128"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseExactProxies([]string{"10.0.0.0/8"}); err == nil {
		t.Fatal("network prefix accepted for identity peers")
	}
}

func TestForwardedForIsReadFromEveryHeaderLine(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"10.0.0.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://binnacle.test", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	// A client-supplied line arrives first; the trusted proxy appends its own.
	r.Header.Add("X-Forwarded-For", "203.0.113.9")
	r.Header.Add("X-Forwarded-For", "198.51.100.25, 10.0.0.3")
	if got := proxies.ClientPrefix(r); got != "198.51.100.0/24" {
		t.Fatalf("client prefix=%s", got)
	}
}

func TestForwardedProtoUsesTheRightmostTrustedValue(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"10.0.0.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://binnacle.test", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Add("X-Forwarded-Proto", "https")
	r.Header.Add("X-Forwarded-Proto", "http")
	if proxies.Secure(r) {
		t.Fatal("client-supplied leftmost https was trusted over the proxy's http")
	}
	r.Header.Del("X-Forwarded-Proto")
	r.Header.Add("X-Forwarded-Proto", "http")
	r.Header.Add("X-Forwarded-Proto", "https")
	if !proxies.Secure(r) {
		t.Fatal("proxy-appended https was not honoured")
	}
	r.RemoteAddr = "203.0.113.5:1234"
	if proxies.Secure(r) {
		t.Fatal("untrusted peer's X-Forwarded-Proto was honoured")
	}
}

func TestPublicURLDecidesSecureAndOrigin(t *testing.T) {
	proxies, err := ParseTrustedProxies(nil)
	if err != nil {
		t.Fatal(err)
	}
	proxies, err = proxies.WithPublicURL("https://Binnacle.example.test:443/")
	if err != nil {
		t.Fatal(err)
	}
	// Plain HTTP from an untrusted peer: the TLS terminator is upstream.
	r := httptest.NewRequest("POST", "http://binnacle:8080/api/v1/auth/logout", nil)
	r.RemoteAddr = "172.18.0.2:5000"
	r.Host = "binnacle:8080"
	if !proxies.Secure(r) {
		t.Fatal("https public URL did not make cookies Secure")
	}
	r.Header.Set("Origin", "https://binnacle.example.test")
	if !SameOrigin(r, proxies) {
		t.Fatal("origin matching the public URL rejected")
	}
	r.Header.Set("Origin", "http://binnacle.example.test")
	if SameOrigin(r, proxies) {
		t.Fatal("downgraded scheme accepted")
	}
	r.Header.Set("Origin", "https://binnacle:8080")
	if SameOrigin(r, proxies) {
		t.Fatal("internal host accepted when a public URL is declared")
	}
	r.Header.Del("Origin")
	r.Header.Set("Referer", "https://binnacle.example.test/settings")
	if !SameOrigin(r, proxies) {
		t.Fatal("referer on the public origin rejected")
	}
	plain, err := ParseTrustedProxies(nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = plain.WithPublicURL("http://binnacle.lan:8080")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Secure(r) {
		t.Fatal("http public URL marked cookies Secure")
	}
	r.Header.Set("Origin", "http://binnacle.lan:8080")
	if !SameOrigin(r, plain) {
		t.Fatal("explicit non-default port on the public URL rejected")
	}
}

func TestWithPublicURLRejectsNonOrigins(t *testing.T) {
	base, _ := ParseTrustedProxies(nil)
	for _, value := range []string{"binnacle.example.test", "ftp://binnacle.example.test", "https://user:pw@binnacle.example.test", "https://binnacle.example.test/?x=1", "https://binnacle.example.test/#frag", "https://"} {
		if _, err := base.WithPublicURL(value); err == nil {
			t.Fatalf("%q accepted as a public URL", value)
		}
	}
	if same, err := base.WithPublicURL(""); err != nil || same.PublicOrigin() != "" {
		t.Fatalf("empty public URL err=%v origin=%q", err, same.PublicOrigin())
	}
	withPath, err := base.WithPublicURL("https://binnacle.example.test/monitoring")
	if err != nil || withPath.PublicOrigin() != "https://binnacle.example.test" {
		t.Fatalf("path form err=%v origin=%q", err, withPath.PublicOrigin())
	}
}
