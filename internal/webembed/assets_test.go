// SPDX-License-Identifier: AGPL-3.0-only

package webembed

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func get(t *testing.T, handler http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func scriptAsset(t *testing.T, handler http.Handler) string {
	t.Helper()
	asset := regexp.MustCompile(`/assets/[^"]+\.js`).FindString(get(t, handler, "/", nil).Body.String())
	if asset == "" {
		t.Fatal("index.html references no script")
	}
	return asset
}

// UI-16: every file carries an ETag from its embedded content, and a
// matching If-None-Match answers 304 with no body.
func TestAssetsCarryETags(t *testing.T) {
	handler := Handler()
	for _, path := range []string{"/", "/overview", scriptAsset(t, handler)} {
		first := get(t, handler, path, nil)
		etag := first.Header().Get("ETag")
		if first.Code != http.StatusOK || !regexp.MustCompile(`^"[0-9a-f]{24}"$`).MatchString(etag) {
			t.Fatalf("%s: status %d, ETag %q", path, first.Code, etag)
		}
		again := get(t, handler, path, map[string]string{"If-None-Match": etag})
		if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
			t.Fatalf("%s revalidation: status %d, %d body bytes", path, again.Code, again.Body.Len())
		}
	}
	if get(t, handler, "/", nil).Header().Get("ETag") != get(t, handler, "/settings", nil).Header().Get("ETag") {
		t.Fatal("UI routes and / serve index.html under different ETags")
	}
}

// UI-16: the script is served from its precompressed variant when the
// client accepts it, with its own ETag and Vary: Accept-Encoding.
func TestAssetsServePrecompressedVariants(t *testing.T) {
	handler := Handler()
	asset := scriptAsset(t, handler)
	identity := get(t, handler, asset, nil)
	if identity.Header().Get("Content-Encoding") != "" || identity.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("identity response headers %v", identity.Header())
	}
	gz := get(t, handler, asset, map[string]string{"Accept-Encoding": "gzip"})
	if gz.Header().Get("Content-Encoding") != "gzip" || gz.Body.Len() >= identity.Body.Len() {
		t.Fatalf("gzip response: encoding %q, %d bytes vs %d", gz.Header().Get("Content-Encoding"), gz.Body.Len(), identity.Body.Len())
	}
	reader, err := gzip.NewReader(gz.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(reader); !bytes.Equal(body, identity.Body.Bytes()) {
		t.Fatal("gzip variant does not decode to the asset")
	}
	if gz.Header().Get("Content-Type") != identity.Header().Get("Content-Type") {
		t.Fatalf("content type %q vs %q", gz.Header().Get("Content-Type"), identity.Header().Get("Content-Type"))
	}
	br := get(t, handler, asset, map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if br.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("br preferred over gzip, got %q", br.Header().Get("Content-Encoding"))
	}
	etags := map[string]bool{identity.Header().Get("ETag"): true, gz.Header().Get("ETag"): true, br.Header().Get("ETag"): true}
	if len(etags) != 3 {
		t.Fatalf("encodings share an ETag: %v", etags)
	}
	refused := get(t, handler, asset, map[string]string{"Accept-Encoding": "br;q=0, gzip;q=0"})
	if refused.Header().Get("Content-Encoding") != "" || !bytes.Equal(refused.Body.Bytes(), identity.Body.Bytes()) {
		t.Fatal("q=0 codings were used")
	}
}

func TestAcceptedEncodings(t *testing.T) {
	got := acceptedEncodings(" GZIP;q=0.5 , br;q=0, identity")
	if !got["gzip"] || got["br"] || !got["identity"] {
		t.Fatalf("acceptedEncodings = %v", got)
	}
}

// The variants exist in the embedded build: make web-build runs the
// compression step.
func TestBuildIsPrecompressed(t *testing.T) {
	assets, _ := fs.Sub(files, "dist")
	found := false
	for _, meta := range indexAssets(assets) {
		if meta.variants["br"] && meta.variants["gzip"] {
			found = true
		}
	}
	if !found {
		t.Fatal("no precompressed asset in the embedded build; run make web-build")
	}
}
