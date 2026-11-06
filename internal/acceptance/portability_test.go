// SPDX-License-Identifier: AGPL-3.0-only

//go:build acceptance

package acceptance

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

// Portability acceptance (PRC-1): the real binary with
// BINNACLE_FEATURE_PORTABILITY on, read by API-token clients that carry no
// cookies, the way scripts and Prometheus do.

const (
	scopeServer    = "server:read"
	scopeResources = "resources:read"
	scopeMetrics   = "metrics:read"
	scopeEvents    = "events:read"
	scopeIncidents = "incidents:read"
)

var allScopes = []string{scopeServer, scopeResources, scopeMetrics, scopeEvents, scopeIncidents}

func portabilityEnv(t *testing.T, dataDir string) map[string]string {
	t.Helper()
	env := baseEnv(dataDir, freeAddress(t))
	env["BINNACLE_FEATURE_PORTABILITY"] = "true"
	env["BINNACLE_PROMETHEUS_ENABLED"] = "true"
	return env
}

// signedIn returns a browser with a session for the administrator.
func signedIn(t *testing.T, base string) *browser {
	t.Helper()
	b := newBrowser(t, base)
	expect(t, "password login", login(b, ""), http.StatusNoContent, "")
	return b
}

type createdToken struct {
	ID        string
	Plaintext string
}

func createToken(t *testing.T, b *browser, name string, scopes ...string) createdToken {
	t.Helper()
	created := b.do(http.MethodPost, "/api/v1/api-tokens", map[string]any{"name": name, "scopes": scopes})
	expect(t, "create token "+name, created, http.StatusCreated, "")
	body := decode[struct {
		Token struct {
			ID     string   `json:"id"`
			Prefix string   `json:"prefix"`
			Scopes []string `json:"scopes"`
		} `json:"token"`
		Plaintext string `json:"plaintext"`
	}](t, created)
	if body.Token.ID == "" || !strings.HasPrefix(body.Plaintext, "bnk_") || !strings.HasPrefix(body.Plaintext, body.Token.Prefix) {
		t.Fatalf("created token %s does not carry an id and a bnk_ plaintext", created.Body)
	}
	// Binnacle stores scopes sorted.
	want := slices.Sorted(slices.Values(scopes))
	if !slices.Equal(body.Token.Scopes, want) {
		t.Fatalf("token %s has scopes %v, want %v", name, body.Token.Scopes, want)
	}
	return createdToken{ID: body.Token.ID, Plaintext: body.Plaintext}
}

// bearer is a cookie-less client that authenticates with one API token.
func bearer(t *testing.T, base, plaintext string) *browser {
	t.Helper()
	b := newBrowser(t, base)
	b.origin = ""
	b.header.Set("Authorization", "Bearer "+plaintext)
	return b
}

func rfc3339(at time.Time) string { return at.UTC().Format(time.RFC3339) }

func window(from, to time.Time) string {
	return "from=" + url.QueryEscape(rfc3339(from)) + "&to=" + url.QueryEscape(rfc3339(to))
}

func TestPortabilityBearerReads(t *testing.T) {
	inst := start(t, portabilityEnv(t, t.TempDir()))
	admin := signedIn(t, inst.URL)
	tokens := map[string]createdToken{}
	for _, scope := range allScopes {
		tokens[scope] = createToken(t, admin, "only "+scope, scope)
	}
	full := createToken(t, admin, "every scope", allScopes...)

	resources := decode[[]struct {
		ID string `json:"id"`
	}](t, bearer(t, inst.URL, full.Plaintext).do(http.MethodGet, "/api/v1/resources", nil))
	if len(resources) == 0 {
		t.Fatal("demo mode listed no resources")
	}
	now := time.Now()
	recent := window(now.Add(-time.Hour), now)

	t.Run("each read needs its scope", func(t *testing.T) {
		reads := []struct{ path, scope string }{
			{"/api/v1/server", scopeServer},
			{"/api/v1/collector-health", scopeServer},
			{"/api/v1/filesystems", scopeServer},
			{"/api/v1/resources", scopeResources},
			{"/api/v1/resources/" + url.PathEscape(resources[0].ID), scopeResources},
			{"/api/v1/resources?state=archived", scopeResources},
			{"/api/v1/metrics?scope=host&metrics=cpu,memory&" + recent, scopeMetrics},
			{"/api/v1/metrics/sparklines?range=1h&metrics=cpu", scopeMetrics},
			{"/api/v1/events", scopeEvents},
			{"/api/v1/incidents", scopeIncidents},
			{"/api/v1/exports/resources.json", scopeResources},
			{"/api/v1/exports/metrics.csv?scope=host&metrics=cpu&" + recent, scopeMetrics},
			{"/api/v1/exports/events.json?" + recent, scopeEvents},
			{"/api/v1/exports/incidents.json?" + recent, scopeIncidents},
		}
		for _, read := range reads {
			for _, scope := range allScopes {
				got := bearer(t, inst.URL, tokens[scope].Plaintext).do(http.MethodGet, read.path, nil)
				if scope == read.scope {
					expect(t, read.path+" with "+scope, got, http.StatusOK, "")
				} else {
					expect(t, read.path+" with "+scope, got, http.StatusForbidden, "insufficient_scope")
				}
			}
			expect(t, read.path+" with every scope", bearer(t, inst.URL, full.Plaintext).do(http.MethodGet, read.path, nil), http.StatusOK, "")
			expect(t, read.path+" without credentials", newBrowser(t, inst.URL).do(http.MethodGet, read.path, nil), http.StatusUnauthorized, "unauthorized")
		}
	})

	t.Run("exports are attachments with schema metadata", func(t *testing.T) {
		client := bearer(t, inst.URL, full.Plaintext)
		csv := client.do(http.MethodGet, "/api/v1/exports/metrics.csv?scope=host&metrics=cpu&"+recent, nil)
		expect(t, "metrics export", csv, http.StatusOK, "")
		if !strings.HasPrefix(string(csv.Body), "schema_version,exported_at,scope,id,resolution,metric,unit,timestamp,min,avg,max,count\n") {
			t.Fatalf("metrics export does not start with its schema header:\n%.200s", csv.Body)
		}
		if strings.Count(string(csv.Body), "\n") < 2 {
			t.Fatalf("metrics export of the seeded demo history has no rows:\n%s", csv.Body)
		}
		for name, got := range map[string]response{
			"binnacle-metrics.csv":    csv,
			"binnacle-resources.json": client.do(http.MethodGet, "/api/v1/exports/resources.json", nil),
			"binnacle-events.json":    client.do(http.MethodGet, "/api/v1/exports/events.json?"+recent, nil),
		} {
			if got.Header.Get("Content-Disposition") != `attachment; filename="`+name+`"` || got.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s: Content-Disposition %q, Cache-Control %q", name, got.Header.Get("Content-Disposition"), got.Header.Get("Cache-Control"))
			}
		}
		envelope := decode[struct {
			SchemaVersion int `json:"schemaVersion"`
			Rows          []struct {
				ID string `json:"id"`
			} `json:"rows"`
		}](t, client.do(http.MethodGet, "/api/v1/exports/resources.json", nil))
		if envelope.SchemaVersion != 1 || len(envelope.Rows) != len(resources) {
			t.Fatalf("resources export has schema %d and %d rows, want schema 1 and %d rows", envelope.SchemaVersion, len(envelope.Rows), len(resources))
		}
	})

	t.Run("tokens never reach session-only routes", func(t *testing.T) {
		client := bearer(t, inst.URL, full.Plaintext)
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/live"},
			{http.MethodGet, "/api/v1/logs?resource=" + url.QueryEscape(resources[0].ID)},
			{http.MethodGet, "/api/v1/processes"},
			{http.MethodGet, "/api/v1/settings"},
			{http.MethodGet, "/api/v1/preferences"},
			{http.MethodPut, "/api/v1/preferences"},
			{http.MethodGet, "/api/v1/monitor-health"},
			{http.MethodGet, "/api/v1/checks"},
			{http.MethodGet, "/api/v1/alerts"},
			{http.MethodGet, "/api/v1/alert-rules"},
			{http.MethodGet, "/api/v1/silences"},
			{http.MethodGet, "/api/v1/onboarding"},
			{http.MethodGet, "/api/v1/integrations/coolify"},
			{http.MethodGet, "/api/v1/auth/session"},
			{http.MethodGet, "/api/v1/api-tokens"},
			{http.MethodPost, "/api/v1/api-tokens"},
			{http.MethodDelete, "/api/v1/api-tokens/" + tokens[scopeServer].ID},
			{http.MethodPost, "/api/v1/history/deletion-previews"},
			{http.MethodPost, "/api/v1/diagnostics/previews"},
		} {
			got := client.do(route.method, route.path, map[string]any{})
			if got.Status != http.StatusUnauthorized && got.Status != http.StatusForbidden {
				t.Errorf("%s %s with an every-scope token: got %d %s, want 401 or 403", route.method, route.path, got.Status, strings.TrimSpace(string(got.Body)))
			}
		}
		// The incidents scope covers incidents only; the notification
		// routes behind the same authorizer refuse the token by path.
		for _, path := range []string{"/api/v1/notification-channels", "/api/v1/notification-deliveries"} {
			expect(t, path+" with incidents:read", bearer(t, inst.URL, tokens[scopeIncidents].Plaintext).do(http.MethodGet, path, nil), http.StatusUnauthorized, "invalid_token")
			expect(t, path+" with every scope", client.do(http.MethodGet, path, nil), http.StatusUnauthorized, "invalid_token")
			expect(t, path+" with a session", admin.do(http.MethodGet, path, nil), http.StatusOK, "")
		}
		// The token the session-only DELETE named is still valid.
		expect(t, "server token after the refused revoke", bearer(t, inst.URL, tokens[scopeServer].Plaintext).do(http.MethodGet, "/api/v1/server", nil), http.StatusOK, "")
	})

	t.Run("no cookie fallback", func(t *testing.T) {
		expect(t, "session read", admin.do(http.MethodGet, "/api/v1/server", nil), http.StatusOK, "")
		for _, header := range []string{"Bearer bnk_not_a_token", "Basic YWRtaW46cGFzc3dvcmQ=", "Bearer", tokens[scopeServer].Plaintext} {
			admin.header.Set("Authorization", header)
			expect(t, "token read with a session cookie and Authorization "+header, admin.do(http.MethodGet, "/api/v1/server", nil), http.StatusUnauthorized, "invalid_token")
		}
		admin.header.Set("Authorization", "Bearer "+tokens[scopeEvents].Plaintext)
		expect(t, "wrong-scope token with a session cookie", admin.do(http.MethodGet, "/api/v1/server", nil), http.StatusForbidden, "insufficient_scope")
		expect(t, "session-only read with a session cookie and a token", admin.do(http.MethodGet, "/api/v1/settings", nil), http.StatusUnauthorized, "")
		expect(t, "token management with a session cookie and a token", admin.do(http.MethodGet, "/api/v1/api-tokens", nil), http.StatusUnauthorized, "unauthorized")
		expect(t, "metrics scrape with a session cookie and a wrong-scope token", admin.do(http.MethodGet, "/metrics", nil), http.StatusForbidden, "insufficient_scope")
		admin.header.Del("Authorization")
		expect(t, "metrics scrape with only a session cookie", admin.do(http.MethodGet, "/metrics", nil), http.StatusUnauthorized, "invalid_token")
		expect(t, "session read after the token attempts", admin.do(http.MethodGet, "/api/v1/server", nil), http.StatusOK, "")
	})

	t.Run("prometheus scrape", func(t *testing.T) {
		anonymous := newBrowser(t, inst.URL).do(http.MethodGet, "/metrics", nil)
		expect(t, "scrape without a token", anonymous, http.StatusUnauthorized, "invalid_token")
		if got := anonymous.Header.Get("WWW-Authenticate"); got != `Bearer realm="binnacle", scope="metrics:read"` {
			t.Errorf("WWW-Authenticate = %q", got)
		}
		expect(t, "scrape with an unknown token", bearer(t, inst.URL, "bnk_unknown").do(http.MethodGet, "/metrics", nil), http.StatusUnauthorized, "invalid_token")
		expect(t, "scrape with server:read", bearer(t, inst.URL, tokens[scopeServer].Plaintext).do(http.MethodGet, "/metrics", nil), http.StatusForbidden, "insufficient_scope")
		scraper := bearer(t, inst.URL, tokens[scopeMetrics].Plaintext)
		expect(t, "scrape with POST", scraper.do(http.MethodPost, "/metrics", nil), http.StatusMethodNotAllowed, "")
		scrape := scraper.do(http.MethodGet, "/metrics", nil)
		expect(t, "scrape with metrics:read", scrape, http.StatusOK, "")
		if got := scrape.Header.Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
			t.Errorf("scrape Content-Type = %q", got)
		}
		text := string(scrape.Body)
		for _, series := range []string{"binnacle_host_cpu_percent ", "binnacle_host_memory_total_bytes ", `binnacle_resource_memory_bytes{category=`, `binnacle_collector_state{collector="docker"`, `binnacle_health_check_state{check_id=`} {
			if !strings.Contains(text, series) {
				t.Errorf("scrape has no %s series:\n%.2000s", series, text)
			}
		}
		for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
			if fields := strings.Fields(line); len(fields) != 2 || !strings.HasPrefix(fields[0], "binnacle_") {
				t.Errorf("scrape line is not a binnacle sample: %q", line)
			}
		}
	})

	t.Run("revoked tokens stop working", func(t *testing.T) {
		expect(t, "revoke", admin.do(http.MethodDelete, "/api/v1/api-tokens/"+tokens[scopeMetrics].ID, nil), http.StatusNoContent, "")
		expect(t, "scrape with a revoked token", bearer(t, inst.URL, tokens[scopeMetrics].Plaintext).do(http.MethodGet, "/metrics", nil), http.StatusUnauthorized, "invalid_token")
		expect(t, "read with a revoked token", bearer(t, inst.URL, tokens[scopeMetrics].Plaintext).do(http.MethodGet, "/api/v1/metrics/sparklines?metrics=cpu", nil), http.StatusUnauthorized, "invalid_token")
		listed := decode[struct {
			Tokens []struct {
				ID        string  `json:"id"`
				RevokedAt *string `json:"revokedAt"`
			} `json:"tokens"`
		}](t, admin.do(http.MethodGet, "/api/v1/api-tokens", nil))
		revoked := 0
		for _, token := range listed.Tokens {
			if token.RevokedAt != nil {
				revoked++
				if token.ID != tokens[scopeMetrics].ID {
					t.Errorf("token %s is revoked, want only %s", token.ID, tokens[scopeMetrics].ID)
				}
			}
		}
		if len(listed.Tokens) != len(allScopes)+1 || revoked != 1 {
			t.Fatalf("token list has %d tokens and %d revoked, want %d and 1", len(listed.Tokens), revoked, len(allScopes)+1)
		}
	})
}

// Seeded event windows for the export bounds, all well inside the default
// retention and away from the events demo mode writes at the current time.
const (
	// rowsCount events fill the rows window, one every rowsSpacing, and one
	// more sits rowsOverflowAt after its start: the window up to rowsSpan
	// holds exactly the row cap, the window up to the extra event one more.
	rowsCount      = 10000
	rowsSpacing    = 50 * time.Millisecond
	rowsSpan       = 500 * time.Second
	rowsOverflowAt = 600 * time.Second
	// bulkyCount events of bulkySize bytes each exceed the 16 MiB byte cap
	// while staying far below the row cap.
	bulkyCount = 1100
	bulkySize  = 16 << 10
)

// seedEvents writes the export fixtures straight into the database before
// binnacle opens it. storage.Open applies the migrations, so the binary
// starts on a current schema with history already in place.
func seedEvents(t *testing.T, dataDir string, rowsStart, bulkyStart time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store := storage.New(filepath.Join(dataDir, "binnacle.db"), t.TempDir())
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("close seeded database: %v", err)
		}
	}()
	tx, err := store.Writer().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(at time.Time, summary, details string) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO events(id,ts,host_id,type,severity,summary,details_json,source,created_at) VALUES(?,?,NULL,'acceptance_seed','info',?,?,'binnacle',?)", metrics.NewEventID(), at.UnixMilli(), summary, details, at.UnixMilli()); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	for i := range rowsCount {
		insert(rowsStart.Add(time.Duration(i)*rowsSpacing), "row", "")
	}
	insert(rowsStart.Add(rowsOverflowAt), "row past the cap", "")
	padding := strings.Repeat("x", bulkySize)
	for i := range bulkyCount {
		insert(bulkyStart.Add(time.Duration(i)*time.Second), "bulky", padding)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestPortabilityExportBounds(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	rowsStart := now.Add(-6 * time.Hour)
	bulkyStart := now.Add(-4 * time.Hour)
	seedEvents(t, dataDir, rowsStart, bulkyStart)
	inst := start(t, portabilityEnv(t, dataDir))
	admin := signedIn(t, inst.URL)
	client := bearer(t, inst.URL, createToken(t, admin, "exports", allScopes...).Plaintext)

	t.Run("time", func(t *testing.T) {
		for _, path := range []string{"/api/v1/exports/events.json", "/api/v1/exports/incidents.json", "/api/v1/exports/metrics.csv"} {
			query := ""
			if strings.HasSuffix(path, ".csv") {
				query = "scope=host&metrics=cpu&"
			}
			for name, bounds := range map[string]string{
				"no range":            "",
				"no end":              "from=" + url.QueryEscape(rfc3339(now.Add(-time.Hour))),
				"unparsable start":    "from=yesterday&to=" + url.QueryEscape(rfc3339(now)),
				"empty range":         window(now, now),
				"reversed range":      window(now, now.Add(-time.Hour)),
				"longer than 30 days": window(now.Add(-30*24*time.Hour-time.Second), now),
			} {
				expect(t, path+" with "+name, client.do(http.MethodGet, path+"?"+query+bounds, nil), http.StatusBadRequest, "invalid_time_range")
			}
			// Ends before the seeded windows, so only the range is at stake.
			end := rowsStart.Add(-time.Hour)
			expect(t, path+" over exactly 30 days", client.do(http.MethodGet, path+"?"+query+window(end.Add(-30*24*time.Hour), end), nil), http.StatusOK, "")
		}
	})

	t.Run("rows", func(t *testing.T) {
		type eventsExport struct {
			SchemaVersion int    `json:"schemaVersion"`
			From          string `json:"from"`
			To            string `json:"to"`
			Rows          []struct {
				Summary string `json:"summary"`
			} `json:"rows"`
		}
		full := client.do(http.MethodGet, "/api/v1/exports/events.json?"+window(rowsStart, rowsStart.Add(rowsSpan)), nil)
		expect(t, "export of exactly 10,000 rows", full, http.StatusOK, "")
		body := decode[eventsExport](t, full)
		if len(body.Rows) != rowsCount || body.SchemaVersion != 1 || body.From != rfc3339(rowsStart) || body.To != rfc3339(rowsStart.Add(rowsSpan)) {
			t.Fatalf("export holds %d rows, schema %d, range %s..%s; want %d rows of the requested range", len(body.Rows), body.SchemaVersion, body.From, body.To, rowsCount)
		}
		over := client.do(http.MethodGet, "/api/v1/exports/events.json?"+window(rowsStart, rowsStart.Add(rowsOverflowAt)), nil)
		expect(t, "export of 10,001 rows", over, http.StatusRequestEntityTooLarge, "export_too_large")
		if !strings.Contains(string(over.Body), "10,000 rows") || over.Header.Get("Content-Disposition") != "" {
			t.Fatalf("over-cap export is not an explicit row refusal: %s %v", over.Body, over.Header)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		got := client.do(http.MethodGet, "/api/v1/exports/events.json?"+window(bulkyStart, bulkyStart.Add(bulkyCount*time.Second)), nil)
		expect(t, "export over 16 MiB", got, http.StatusRequestEntityTooLarge, "export_too_large")
		if !strings.Contains(string(got.Body), "16 MiB") || got.Header.Get("Content-Disposition") != "" {
			t.Fatalf("over-size export is not an explicit byte refusal: %s %v", got.Body, got.Header)
		}
		// Half the window is under the cap and exports whole.
		half := client.do(http.MethodGet, "/api/v1/exports/events.json?"+window(bulkyStart, bulkyStart.Add(bulkyCount/2*time.Second-time.Second)), nil)
		expect(t, "export under 16 MiB", half, http.StatusOK, "")
		if rows := len(decode[struct {
			Rows []struct{} `json:"rows"`
		}](t, half).Rows); rows != bulkyCount/2 {
			t.Fatalf("export under 16 MiB holds %d rows, want %d", rows, bulkyCount/2)
		}
	})
}

func TestPortabilityGateOff(t *testing.T) {
	t.Run("routes stay hidden and stored tokens stay inert", func(t *testing.T) {
		dataDir := t.TempDir()
		on := portabilityEnv(t, dataDir)
		first := start(t, on)
		token := createToken(t, signedIn(t, first.URL), "kept", allScopes...)
		expect(t, "read with the gate on", bearer(t, first.URL, token.Plaintext).do(http.MethodGet, "/api/v1/server", nil), http.StatusOK, "")
		first.stop(t)

		off := start(t, baseEnv(dataDir, on["BINNACLE_LISTEN_ADDRESS"]))
		admin := signedIn(t, off.URL)
		now := time.Now()
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/api-tokens"},
			{http.MethodPost, "/api/v1/api-tokens"},
			{http.MethodDelete, "/api/v1/api-tokens/" + token.ID},
			{http.MethodGet, "/api/v1/exports/metrics.csv?scope=host&metrics=cpu&" + window(now.Add(-time.Hour), now)},
			{http.MethodGet, "/api/v1/exports/events.json?" + window(now.Add(-time.Hour), now)},
			{http.MethodGet, "/api/v1/exports/incidents.json?" + window(now.Add(-time.Hour), now)},
			{http.MethodGet, "/api/v1/exports/resources.json"},
		} {
			expect(t, route.method+" "+route.path+" with a session", admin.do(route.method, route.path, map[string]any{"name": "x", "scopes": allScopes}), http.StatusNotFound, "not_found")
			expect(t, route.method+" "+route.path+" with a token", bearer(t, off.URL, token.Plaintext).do(route.method, route.path, nil), http.StatusNotFound, "not_found")
		}
		for _, client := range []*browser{admin, bearer(t, off.URL, token.Plaintext), newBrowser(t, off.URL)} {
			if got := client.do(http.MethodGet, "/metrics", nil); got.Status != http.StatusNotFound {
				t.Fatalf("/metrics with the gate off: got %d %s, want 404", got.Status, got.Body)
			}
		}
		for _, path := range []string{"/api/v1/server", "/api/v1/resources", "/api/v1/metrics/sparklines?metrics=cpu", "/api/v1/events", "/api/v1/incidents"} {
			expect(t, path+" with a token and the gate off", bearer(t, off.URL, token.Plaintext).do(http.MethodGet, path, nil), http.StatusUnauthorized, "unauthorized")
			expect(t, path+" with a session and the gate off", admin.do(http.MethodGet, path, nil), http.StatusOK, "")
		}
		admin.header.Set("Authorization", "Bearer "+token.Plaintext)
		expect(t, "session plus token with the gate off", admin.do(http.MethodGet, "/api/v1/server", nil), http.StatusUnauthorized, "unauthorized")
		off.stop(t)

		again := start(t, on)
		expect(t, "the stored token once the gate is back on", bearer(t, again.URL, token.Plaintext).do(http.MethodGet, "/api/v1/server", nil), http.StatusOK, "")
	})

	t.Run("prometheus needs its own setting", func(t *testing.T) {
		env := portabilityEnv(t, t.TempDir())
		delete(env, "BINNACLE_PROMETHEUS_ENABLED")
		inst := start(t, env)
		token := createToken(t, signedIn(t, inst.URL), "scraper", scopeMetrics)
		if got := bearer(t, inst.URL, token.Plaintext).do(http.MethodGet, "/metrics", nil); got.Status != http.StatusNotFound {
			t.Fatalf("/metrics with Prometheus off: got %d %s, want 404", got.Status, got.Body)
		}
		expect(t, "token read with Prometheus off", bearer(t, inst.URL, token.Plaintext).do(http.MethodGet, "/api/v1/metrics/sparklines?metrics=cpu", nil), http.StatusOK, "")
	})

	t.Run("prometheus without portability refuses to start", func(t *testing.T) {
		env := baseEnv(t.TempDir(), freeAddress(t))
		env["BINNACLE_PROMETHEUS_ENABLED"] = "true"
		output, err := runToExit(t, env)
		if err == nil {
			t.Fatalf("binnacle started with Prometheus on and the gate off\n%s", output)
		}
		if !strings.Contains(output, "prometheus requires features.portability=true") {
			t.Fatalf("refusal does not explain itself:\n%s", output)
		}
	})
}
