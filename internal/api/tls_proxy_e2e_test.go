// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/preferences"
	"github.com/drilonrecica/binnacle/internal/storage"
)

// tlsProxyStack is Binnacle on plain HTTP behind a TLS-terminating reverse
// proxy that forwards like Traefik or Caddy: it sets X-Forwarded-For and
// X-Forwarded-Proto and the Host the browser used (SEC-3).
type tlsProxyStack struct {
	proxy   *httptest.Server
	backend *httptest.Server
	client  *http.Client
}

func newTLSProxyStack(t *testing.T, configure func(public string) auth.TrustedProxies) *tlsProxyStack {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	credentials := auth.NewCredentials(manager.DB())
	if _, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(manager.DB(), auth.SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})

	// The proxy's public address is only known once it listens, so start it
	// with a placeholder director and point it at the backend afterwards.
	var backendURL *url.URL
	reverse := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(backendURL)
		r.Out.Host = r.In.Host
		r.SetXForwarded()
	}}
	proxy := httptest.NewTLSServer(reverse)
	t.Cleanup(proxy.Close)

	proxies := configure(proxy.URL)
	sessions.SetTrustedProxies(proxies)
	protection := auth.NewProtection(128, proxies)
	server := New()
	server.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.SetTrustedProxies(proxies)
	server.EnableAuth(credentials, sessions, protection)
	server.EnablePreferences(preferences.NewRepository(manager.DB()), sessions)
	backend := httptest.NewServer(server.Handler())
	t.Cleanup(backend.Close)
	var err error
	if backendURL, err = url.Parse(backend.URL); err != nil {
		t.Fatal(err)
	}

	client := proxy.Client()
	if client.Jar, err = cookiejar.New(nil); err != nil {
		t.Fatal(err)
	}
	return &tlsProxyStack{proxy: proxy, backend: backend, client: client}
}

// do sends a browser-like request through the proxy: JSON body, Origin on
// mutations, and the CSRF cookie echoed into X-CSRF-Token.
func (s *tlsProxyStack) do(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, s.proxy.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Origin", s.proxy.URL)
	}
	for _, cookie := range s.client.Jar.Cookies(req.URL) {
		if cookie.Name == auth.CSRFCookieName {
			req.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func (s *tlsProxyStack) login(t *testing.T) *http.Response {
	t.Helper()
	return s.do(t, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"correct horse battery staple"}`)
}

func TestLoginAndMutationBehindTLSTerminatingProxy(t *testing.T) {
	configurations := map[string]func(public string) auth.TrustedProxies{
		"trusted proxy network": func(string) auth.TrustedProxies {
			p, err := auth.ParseTrustedProxies([]string{"127.0.0.0/8", "::1/128"})
			if err != nil {
				t.Fatal(err)
			}
			return p
		},
		"declared public url": func(public string) auth.TrustedProxies {
			p, err := auth.TrustedProxies{}.WithPublicURL(public)
			if err != nil {
				t.Fatal(err)
			}
			return p
		},
	}
	for name, configure := range configurations {
		t.Run(name, func(t *testing.T) {
			stack := newTLSProxyStack(t, configure)
			login := stack.login(t)
			if login.StatusCode != http.StatusNoContent {
				t.Fatalf("login status=%d body=%s", login.StatusCode, readAll(t, login.Body))
			}
			if login.Header.Get("Strict-Transport-Security") == "" {
				t.Fatal("HSTS missing on an HTTPS response relayed by the proxy")
			}
			for _, cookie := range login.Cookies() {
				if !cookie.Secure {
					t.Fatalf("cookie %s is not Secure behind the TLS proxy", cookie.Name)
				}
			}
			put := stack.do(t, http.MethodPut, "/api/v1/preferences", `{"theme":"dark","density":"comfortable","pinnedResources":[],"landingPage":"overview","chartRange":"1h"}`)
			if put.StatusCode != http.StatusOK {
				t.Fatalf("mutation status=%d body=%s", put.StatusCode, readAll(t, put.Body))
			}
			get := stack.do(t, http.MethodGet, "/api/v1/preferences", "")
			if get.StatusCode != http.StatusOK || !strings.Contains(readAll(t, get.Body), `"theme":"dark"`) {
				t.Fatalf("preferences were not persisted through the proxy (status=%d)", get.StatusCode)
			}
			logout := stack.do(t, http.MethodPost, "/api/v1/auth/logout", "")
			if logout.StatusCode != http.StatusNoContent {
				t.Fatalf("logout status=%d body=%s", logout.StatusCode, readAll(t, logout.Body))
			}
		})
	}
}

// Without either setting Binnacle has no way to know the browser used HTTPS:
// the Origin check expects http://, so the session is issued but no mutation
// succeeds and cookies lack Secure. This is the shipped failure mode the two
// settings exist to remove; it documents why the packaging must set one.
func TestUnconfiguredProxyTrustRejectsMutationsOverTLS(t *testing.T) {
	stack := newTLSProxyStack(t, func(string) auth.TrustedProxies { return auth.TrustedProxies{} })
	login := stack.login(t)
	if login.StatusCode != http.StatusNoContent {
		t.Fatalf("login status=%d", login.StatusCode)
	}
	for _, cookie := range login.Cookies() {
		if cookie.Secure {
			t.Fatalf("cookie %s unexpectedly Secure without proxy trust", cookie.Name)
		}
	}
	if login.Header.Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS sent without any evidence of HTTPS")
	}
	put := stack.do(t, http.MethodPut, "/api/v1/preferences", `{"theme":"dark"}`)
	if put.StatusCode != http.StatusForbidden {
		t.Fatalf("mutation status=%d, want 403", put.StatusCode)
	}
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
