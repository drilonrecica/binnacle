// SPDX-License-Identifier: AGPL-3.0-only

package app_test

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/drilonrecica/binnacle/internal/api"
	"github.com/drilonrecica/binnacle/internal/app"
	"github.com/drilonrecica/binnacle/internal/webembed"
)

// TestRootRoutesCarrySecurityHeaders wires the root mux the way main does and
// checks that the UI document, a hashed asset, /healthz and /metrics all get
// the same headers as /api/v1 (SEC-6).
func TestRootRoutesCarrySecurityHeaders(t *testing.T) {
	apiServer := api.New()
	apiServer.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "# metrics\n") })
	h := app.NewHTTPServer("127.0.0.1:0", app.New(nil), app.Routes{
		API: apiServer.Routes(), Static: webembed.Handler(), Metrics: metrics, Middleware: apiServer.Middleware,
	})

	index := httptest.NewRecorder()
	h.Handler().ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	asset := regexp.MustCompile(`/assets/[^"]+\.js`).FindString(index.Body.String())
	if asset == "" {
		t.Fatalf("no hashed script in index.html: %s", index.Body.String())
	}

	for _, path := range []string{"/", "/overview", asset, "/healthz", "/metrics", "/api/v1/missing"} {
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		want := map[string]string{
			"Content-Security-Policy": api.ContentSecurityPolicy,
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "strict-origin-when-cross-origin",
		}
		for key, value := range want {
			if got := rec.Header().Get(key); got != value {
				t.Errorf("%s: %s = %q, want %q", path, key, got, value)
			}
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s: missing X-Request-ID", path)
		}
	}
	if !regexp.MustCompile(`frame-ancestors 'none'`).MatchString(api.ContentSecurityPolicy) {
		t.Fatal("CSP must forbid framing")
	}
}

// TestInlineScriptsMatchCSP: every inline script in the served index.html must
// be allowed by its hash, or the browser blocks it and light-theme users get
// the dark flash back (UI-20). Scripts stay out of 'unsafe-inline'.
func TestInlineScriptsMatchCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	webembed.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(rec.Body.String(), -1)
	if len(inline) == 0 {
		t.Fatalf("index.html has no inline theme script: %s", rec.Body.String())
	}
	for _, match := range inline {
		sum := sha256.Sum256([]byte(match[1]))
		hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !strings.Contains(api.ContentSecurityPolicy, hash) {
			t.Errorf("CSP does not allow inline script %s; update themeScriptHash in internal/api/server.go", hash)
		}
	}
	scriptSrc := regexp.MustCompile(`script-src [^;]*`).FindString(api.ContentSecurityPolicy)
	if strings.Contains(scriptSrc, "unsafe-inline") {
		t.Fatalf("script-src must not allow unsafe-inline: %s", scriptSrc)
	}
}
