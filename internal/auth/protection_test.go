// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterRecoversAfterRefill(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(2)
	l.now = func() time.Time { return now }
	p := BucketPolicy{Capacity: 1, Refill: time.Minute}
	if ok, _ := l.Allow("ip", p); !ok {
		t.Fatal("first request denied")
	}
	if ok, _ := l.Allow("ip", p); ok {
		t.Fatal("burst limit not enforced")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow("ip", p); !ok {
		t.Fatal("bucket did not refill")
	}
}

func TestProtectionPoliciesAreBounded(t *testing.T) {
	p := NewProtection(8, TrustedProxies{})
	for i := 0; i < 100; i++ {
		r := httptest.NewRequest("GET", "http://binnacle.test/api/v1/metrics", nil)
		r.RemoteAddr = fmt.Sprintf("198.18.%d.1:1234", i)
		_, _ = p.AllowMetrics(r)
	}
	if p.limiter.order.Len() > 8 {
		t.Fatalf("entries=%d", p.limiter.order.Len())
	}
	r := httptest.NewRequest("POST", "http://binnacle.test/api/v1/diagnostics", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	for i := 0; i < 3; i++ {
		if ok, _ := p.AllowDiagnostics(r, "usr"); !ok {
			t.Fatal("diagnostics denied early")
		}
	}
	if ok, _ := p.AllowDiagnostics(r, "usr"); ok {
		t.Fatal("diagnostics burst accepted")
	}
}

func TestSameOriginRejectsSpoofing(t *testing.T) {
	r := httptest.NewRequest("POST", "https://binnacle.test/api/v1/auth/logout", nil)
	r.Host = "binnacle.test"
	r.Header.Set("Origin", "https://binnacle.test")
	if !SameOrigin(r, TrustedProxies{}) {
		t.Fatal("same origin rejected")
	}
	r.Header.Set("Origin", "https://evil.test")
	if SameOrigin(r, TrustedProxies{}) {
		t.Fatal("cross origin accepted")
	}
	r.Header.Del("Origin")
	if SameOrigin(r, TrustedProxies{}) {
		t.Fatal("missing origin accepted")
	}
}

func TestLoginAndSetupLimitsRecover(t *testing.T) {
	now := time.Unix(0, 0)
	p := NewProtection(32, TrustedProxies{})
	p.limiter.now = func() time.Time { return now }
	r := httptest.NewRequest("POST", "http://binnacle.test/api/v1/setup/verify", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	for i := 0; i < 5; i++ {
		if ok, _ := p.AllowSetup(r); !ok {
			t.Fatal("setup denied early")
		}
	}
	if ok, _ := p.AllowSetup(r); ok {
		t.Fatal("setup burst accepted")
	}
	now = now.Add(5 * time.Minute)
	if ok, _ := p.AllowSetup(r); !ok {
		t.Fatal("setup did not recover")
	}
	for i := 0; i < 5; i++ {
		if ok, _ := p.AllowLogin(r, "admin", known); !ok {
			t.Fatal("login denied early")
		}
	}
	if ok, _ := p.AllowLogin(r, "admin", known); ok {
		t.Fatal("account burst accepted")
	}
}

func known(string) bool { return true }

func TestAccountBucketsTrackOnlyRealAccounts(t *testing.T) {
	p := NewProtection(4096, TrustedProxies{})
	unknown := func(string) bool { return false }
	for i := 0; i < 50; i++ {
		r := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
		r.RemoteAddr = fmt.Sprintf("198.18.%d.1:1234", i)
		if ok, _ := p.AllowLogin(r, fmt.Sprintf("ghost-%d", i), unknown); !ok {
			t.Fatal("unknown name denied by an account bucket")
		}
	}
	if n := p.accounts.order.Len(); n != 0 {
		t.Fatalf("account buckets for unknown names=%d", n)
	}
	r := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	if ok, _ := p.AllowLogin(r, " Admin ", known); !ok {
		t.Fatal("real account denied")
	}
	if n := p.accounts.order.Len(); n != 1 {
		t.Fatalf("account buckets after a real login=%d", n)
	}
	if _, ok := p.accounts.values["admin"]; !ok {
		t.Fatal("account bucket is not keyed by the normalized username")
	}
}

func TestNetworkLimitedStrangerCannotDrainTheAdminBucket(t *testing.T) {
	p := NewProtection(4096, TrustedProxies{})
	stranger := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
	stranger.RemoteAddr = "203.0.113.5:1234"
	for i := 0; i < 10; i++ {
		if ok, _ := p.AllowLogin(stranger, fmt.Sprintf("ghost-%d", i), func(string) bool { return false }); !ok {
			t.Fatal("stranger denied inside the network budget")
		}
	}
	lookups := 0
	for i := 0; i < 100; i++ {
		ok, _ := p.AllowLogin(stranger, "admin", func(string) bool { lookups++; return true })
		if ok {
			t.Fatal("network-limited stranger accepted")
		}
	}
	if lookups != 0 {
		t.Fatalf("account lookups while network-limited=%d", lookups)
	}
	admin := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
	admin.RemoteAddr = "192.0.2.10:1234"
	for i := 0; i < 5; i++ {
		if ok, _ := p.AllowLogin(admin, "admin", known); !ok {
			t.Fatalf("admin denied on attempt %d after a stranger's flood", i+1)
		}
	}
}

func TestFloodingUnknownNamesDoesNotEvictTheAdminBucket(t *testing.T) {
	p := NewProtection(64, TrustedProxies{})
	admin := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
	admin.RemoteAddr = "192.0.2.10:1234"
	for i := 0; i < 4; i++ {
		if ok, _ := p.AllowLogin(admin, "admin", known); !ok {
			t.Fatal("admin denied early")
		}
	}
	for i := 0; i < 500; i++ {
		r := httptest.NewRequest("POST", "http://binnacle.test/api/v1/auth/login", nil)
		r.RemoteAddr = fmt.Sprintf("198.18.%d.1:1234", i%256)
		_, _ = p.AllowLogin(r, fmt.Sprintf("ghost-%d", i), func(string) bool { return false })
		_, _ = p.AllowMetrics(r)
	}
	if ok, _ := p.AllowLogin(admin, "admin", known); !ok {
		t.Fatal("admin denied on the fifth attempt")
	}
	if ok, _ := p.AllowLogin(admin, "admin", known); ok {
		t.Fatal("flood reset the admin bucket")
	}
}

func TestTrustedProxyStripsSpoofedForwardingEntries(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"10.0.0.2/32"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://binnacle.test", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.25, 10.0.0.2")
	if got := proxies.ClientPrefix(r); got != "198.51.100.0/24" {
		t.Fatalf("client prefix=%s", got)
	}
}
