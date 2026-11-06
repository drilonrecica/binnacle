// SPDX-License-Identifier: AGPL-3.0-only

//go:build acceptance

package acceptance

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Advanced-auth acceptance (PRC-1): the real binary with
// BINNACLE_FEATURE_ADVANCED_AUTH on, driven over HTTP the way a browser and a
// reverse proxy would. Each binnacle process has its own in-memory login
// limiter (five attempts per account, enrolment included), so the phases
// below are spread over restarts of one data directory rather than retried
// in place.

func advancedAuthEnv(t *testing.T, dataDir string) map[string]string {
	t.Helper()
	env := baseEnv(dataDir, freeAddress(t))
	env["BINNACLE_FEATURE_ADVANCED_AUTH"] = "true"
	return env
}

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

func login(b *browser, code string) response {
	b.t.Helper()
	return b.do(http.MethodPost, "/api/v1/auth/login", loginBody{Username: adminUsername, Password: adminPassword, Code: code})
}

func TestAdvancedAuthTOTPLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	env := advancedAuthEnv(t, dataDir)

	// Phase 1: enrol, confirm, and sign in with TOTP.
	first := start(t, env)
	b := newBrowser(t, first.URL)
	methods := decode[map[string]any](t, b.do(http.MethodGet, "/api/v1/auth/methods", nil))
	if methods["mfaAvailable"] != true || methods["local"] != true {
		t.Fatalf("auth methods with the gate on = %v, want local login with MFA available", methods)
	}
	expect(t, "password login before enrolment", login(b, ""), http.StatusNoContent, "")
	elsewhere := newBrowser(t, first.URL)
	expect(t, "second session before enrolment", login(elsewhere, ""), http.StatusNoContent, "")
	status := decode[map[string]bool](t, b.do(http.MethodGet, "/api/v1/auth/mfa", nil))
	if status["enabled"] {
		t.Fatal("MFA reported enabled before enrolment")
	}
	enrolled := b.do(http.MethodPost, "/api/v1/auth/mfa/enroll", map[string]string{"password": adminPassword})
	expect(t, "enrolment", enrolled, http.StatusOK, "")
	enrollment := decode[struct {
		Seed string `json:"seed"`
		URI  string `json:"uri"`
	}](t, enrolled)
	uri, err := url.Parse(enrollment.URI)
	if err != nil || uri.Scheme != "otpauth" || uri.Host != "totp" || uri.Query().Get("secret") != enrollment.Seed || uri.Query().Get("issuer") != "Binnacle" {
		t.Fatalf("enrolment URI %q does not carry the seed %q", enrollment.URI, enrollment.Seed)
	}
	app := newAuthenticator(t, enrollment.Seed)
	expect(t, "confirmation with a wrong code", b.do(http.MethodPost, "/api/v1/auth/mfa/confirm", map[string]string{"code": wrongCode(app)}), http.StatusBadRequest, "mfa_confirmation_failed")
	confirmCode := app.next()
	confirmed := b.do(http.MethodPost, "/api/v1/auth/mfa/confirm", map[string]string{"code": confirmCode})
	expect(t, "confirmation", confirmed, http.StatusOK, "")
	recovery := decode[struct {
		Codes []string `json:"recoveryCodes"`
	}](t, confirmed).Codes
	if len(recovery) != 10 {
		t.Fatalf("got %d recovery codes, want 10", len(recovery))
	}
	seen := map[string]bool{}
	for _, code := range recovery {
		if seen[code] || len(strings.ReplaceAll(code, "-", "")) != 16 {
			t.Fatalf("recovery codes are not 10 distinct 16-character codes: %v", recovery)
		}
		seen[code] = true
	}
	// The enrolling session stays signed in to show the codes; every other
	// session ends (DSC-31).
	expect(t, "the enrolling session after MFA was enabled", b.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusOK, "")
	expect(t, "another session after MFA was enabled", elsewhere.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusUnauthorized, "unauthorized")
	expect(t, "password login without a code", login(b, ""), http.StatusUnauthorized, "invalid_credentials")
	totp := app.next()
	expect(t, "TOTP login", login(b, totp), http.StatusNoContent, "")
	expect(t, "session after TOTP login", b.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusOK, "")
	status = decode[map[string]bool](t, b.do(http.MethodGet, "/api/v1/auth/mfa", nil))
	if !status["enabled"] {
		t.Fatal("MFA not reported enabled after confirmation")
	}
	first.stop(t)

	// Phase 2: with enrolment stored, turning the gate off refuses to start
	// rather than silently dropping the second factor.
	off := baseEnv(dataDir, env["BINNACLE_LISTEN_ADDRESS"])
	output, err := runToExit(t, off)
	if err == nil {
		t.Fatalf("binnacle started with the gate off while TOTP enrolment exists\n%s", output)
	}
	if !strings.Contains(output, "features.advanced_auth must be enabled while TOTP enrollment exists") {
		t.Fatalf("refusal does not explain itself:\n%s", output)
	}

	// Phase 3: back on, the enrolment is intact, replays stay refused across
	// the restart and each recovery code works exactly once.
	second := start(t, env)
	b = newBrowser(t, second.URL)
	expect(t, "replayed TOTP code", login(b, totp), http.StatusUnauthorized, "invalid_credentials")
	expect(t, "earlier TOTP code still inside the drift window", login(b, confirmCode), http.StatusUnauthorized, "invalid_credentials")
	expect(t, "recovery code login", login(b, recovery[0]), http.StatusNoContent, "")
	expect(t, "session after recovery code login", b.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusOK, "")
	expect(t, "consumed recovery code", login(b, recovery[0]), http.StatusUnauthorized, "invalid_credentials")
	expect(t, "next TOTP code", login(b, app.next()), http.StatusNoContent, "")
}

// wrongCode is a well-formed code the server cannot accept: none of the
// steps in the drift window produce it.
func wrongCode(app *authenticator) string {
	step := time.Now().Unix() / period
	valid := map[string]bool{}
	for delta := int64(-2); delta <= 2; delta++ {
		valid[app.codeAt(step+delta)] = true
	}
	for _, candidate := range []string{"000000", "111111", "222222", "333333", "444444", "555555"} {
		if !valid[candidate] {
			return candidate
		}
	}
	return "999999"
}

// identityProxy is a test reverse proxy in front of binnacle. It
// authenticates clients by a bearer token, as ForwardAuth or auth_request
// would, and dials binnacle from 127.0.0.2 so it is a distinct peer from
// clients that talk to binnacle directly from 127.0.0.1.
type identityProxy struct {
	server *httptest.Server
}

// forwarding is how a proxy passes on the identity it established.
type forwarding int

const (
	// replaceIdentity drops whatever the client sent: the safe configuration.
	replaceIdentity forwarding = iota
	// appendIdentity adds a second header line after the client's.
	appendIdentity
	// joinIdentity appends to the client's value with a comma, as a naive
	// proxy_set_header "$http_x_forwarded_user, $subject" would.
	joinIdentity
)

const (
	identityHeader = "X-Forwarded-User"
	proxyPeer      = "127.0.0.2"
	allowedSubject = "alice@example.com"
)

var upstreamUsers = map[string]string{"Bearer alice-token": allowedSubject, "Bearer bob-token": "bob@example.com"}

func newIdentityProxy(t *testing.T, target *url.URL, mode forwarding) *identityProxy {
	t.Helper()
	p := &identityProxy{}
	dialer := &net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(proxyPeer)}}
	reverse := &httputil.ReverseProxy{
		Transport: trackTransport(&http.Transport{DialContext: dialer.DialContext}),
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
			r.SetXForwarded()
			subject := upstreamUsers[r.In.Header.Get("Authorization")]
			r.Out.Header.Del("Authorization")
			if subject == "" && mode != replaceIdentity {
				return
			}
			switch mode {
			case replaceIdentity:
				r.Out.Header.Del(identityHeader)
				if subject != "" {
					r.Out.Header.Set(identityHeader, subject)
				}
			case appendIdentity:
				r.Out.Header.Add(identityHeader, subject)
			case joinIdentity:
				values := append(r.Out.Header.Values(identityHeader), subject)
				r.Out.Header.Set(identityHeader, strings.Join(values, ", "))
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			t.Errorf("test proxy could not reach binnacle from %s: %v", proxyPeer, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	p.server = httptest.NewServer(reverse)
	t.Cleanup(p.server.Close)
	return p
}

func TestAdvancedAuthProxyBootstrap(t *testing.T) {
	listen := freeAddress(t)
	target := &url.URL{Scheme: "http", Host: listen}
	proxy := newIdentityProxy(t, target, replaceIdentity)
	appending := newIdentityProxy(t, target, appendIdentity)
	joining := newIdentityProxy(t, target, joinIdentity)
	env := advancedAuthEnv(t, t.TempDir())
	env["BINNACLE_LISTEN_ADDRESS"] = listen
	env["BINNACLE_AUTH_MODE"] = "proxy"
	env["BINNACLE_AUTH_PROXY_CIDRS"] = proxyPeer + "/32"
	env["BINNACLE_AUTH_IDENTITY_HEADER"] = identityHeader
	env["BINNACLE_AUTH_ALLOWED_SUBJECT"] = allowedSubject
	env["BINNACLE_PUBLIC_URL"] = proxy.server.URL
	inst := start(t, env)
	public := proxy.server.URL

	// A client the proxy has authenticated as the allowed subject.
	alice := newBrowser(t, public)
	alice.header.Set("Authorization", "Bearer alice-token")
	methods := decode[map[string]any](t, alice.do(http.MethodGet, "/api/v1/auth/methods", nil))
	if methods["mode"] != "proxy" || methods["local"] != false || methods["proxyAvailable"] != true {
		t.Fatalf("auth methods through the proxy = %v, want proxy-only with an identity available", methods)
	}
	expect(t, "local login in proxy mode", login(alice, ""), http.StatusNotFound, "not_found")
	expect(t, "session before bootstrap", alice.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusUnauthorized, "unauthorized")
	alice.origin = "http://evil.example"
	expect(t, "cross-origin bootstrap", alice.do(http.MethodPost, "/api/v1/auth/external-session", nil), http.StatusForbidden, "origin_invalid")
	alice.origin = public
	expect(t, "proxy bootstrap", alice.do(http.MethodPost, "/api/v1/auth/external-session", nil), http.StatusNoContent, "")
	if alice.cookie("binnacle_session") == "" || alice.cookie("binnacle_csrf") == "" {
		t.Fatal("proxy bootstrap did not set the session and CSRF cookies")
	}
	session := decode[struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		AuthMethod string `json:"authMethod"`
	}](t, alice.do(http.MethodGet, "/api/v1/auth/session", nil))
	if session.User.Username != adminUsername || session.AuthMethod != "proxy" {
		t.Fatalf("bootstrapped session = %+v, want the local administrator via proxy", session)
	}

	// Spoofed identities. Each request is same-origin, so only the identity
	// check can refuse it.
	cases := []struct {
		name    string
		base    string
		auth    string
		headers []string
	}{
		{name: "unauthenticated client spoofing the header through the proxy", base: public, headers: []string{allowedSubject}},
		{name: "authenticated client spoofing the header through the proxy", base: public, auth: "Bearer bob-token", headers: []string{allowedSubject}},
		{name: "proxy-authenticated subject that is not allowed", base: public, auth: "Bearer bob-token"},
		{name: "header sent straight to binnacle from an untrusted peer", base: inst.URL, headers: []string{allowedSubject}},
		{name: "appending proxy forwarding a duplicate header", base: appending.server.URL, auth: "Bearer alice-token", headers: []string{allowedSubject}},
		{name: "joining proxy forwarding a comma-joined header", base: joining.server.URL, auth: "Bearer alice-token", headers: []string{allowedSubject}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newBrowser(t, tc.base)
			client.origin = public
			if tc.auth != "" {
				client.header.Set("Authorization", tc.auth)
			}
			for _, value := range tc.headers {
				client.header.Add(identityHeader, value)
			}
			methods := decode[map[string]any](t, client.do(http.MethodGet, "/api/v1/auth/methods", nil))
			if methods["proxyAvailable"] != false {
				t.Fatalf("auth methods report an identity: %v", methods)
			}
			expect(t, "bootstrap", client.do(http.MethodPost, "/api/v1/auth/external-session", nil), http.StatusUnauthorized, "external_auth_invalid")
			if client.cookie("binnacle_session") != "" {
				t.Fatal("a refused bootstrap set a session cookie")
			}
		})
	}
}

func TestAdvancedAuthGateOff(t *testing.T) {
	t.Run("routes stay hidden", func(t *testing.T) {
		inst := start(t, baseEnv(t.TempDir(), freeAddress(t)))
		b := newBrowser(t, inst.URL)
		methods := decode[map[string]any](t, b.do(http.MethodGet, "/api/v1/auth/methods", nil))
		if methods["mode"] != "local" || methods["proxy"] != false || methods["mfaAvailable"] != false {
			t.Fatalf("auth methods with the gate off = %v, want local only without MFA", methods)
		}
		// A code is ignored, not required, while the gate is off.
		expect(t, "password login", login(b, "123456"), http.StatusNoContent, "")
		b.header.Set(identityHeader, allowedSubject)
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/auth/mfa"},
			{http.MethodPost, "/api/v1/auth/mfa/enroll"},
			{http.MethodPost, "/api/v1/auth/mfa/confirm"},
			{http.MethodPost, "/api/v1/auth/mfa/disable"},
			{http.MethodPost, "/api/v1/auth/external-session"},
		} {
			expect(t, route.method+" "+route.path, b.do(route.method, route.path, map[string]string{"password": adminPassword}), http.StatusNotFound, "not_found")
		}
		expect(t, "session after probing hidden routes", b.do(http.MethodGet, "/api/v1/auth/session", nil), http.StatusOK, "")
	})
	t.Run("proxy mode refuses to start", func(t *testing.T) {
		env := baseEnv(t.TempDir(), freeAddress(t))
		env["BINNACLE_AUTH_MODE"] = "proxy"
		env["BINNACLE_AUTH_PROXY_CIDRS"] = proxyPeer + "/32"
		env["BINNACLE_AUTH_ALLOWED_SUBJECT"] = allowedSubject
		output, err := runToExit(t, env)
		if err == nil {
			t.Fatalf("binnacle started in proxy mode with the gate off\n%s", output)
		}
		if !strings.Contains(output, "proxy authentication requires features.advanced_auth=true") {
			t.Fatalf("refusal does not explain itself:\n%s", output)
		}
	})
}
