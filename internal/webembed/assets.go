// SPDX-License-Identifier: AGPL-3.0-only

package webembed

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed dist/* dist/assets/*
var files embed.FS

// encodings are the precompressed variants the build writes next to a file
// (web/scripts/compress.mjs), in order of preference.
var encodings = []struct{ token, suffix string }{{"br", ".br"}, {"gzip", ".gz"}}

// asset is what the handler knows about one embedded file. The ETag is the
// file's content hash, fixed when the binary is built (UI-16).
type asset struct {
	etag     string
	variants map[string]bool // encoding token -> precompressed variant exists
}

func Handler() http.Handler {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	index := indexAssets(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || uiRoute(name) {
			name = "index.html"
		}
		if _, ok := index[name]; !ok {
			name = "index.html"
		}
		meta, ok := index[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		file, etag := name, meta.etag
		if len(meta.variants) > 0 {
			w.Header().Add("Vary", "Accept-Encoding")
			accepted := acceptedEncodings(r.Header.Get("Accept-Encoding"))
			for _, encoding := range encodings {
				if meta.variants[encoding.token] && accepted[encoding.token] {
					file = name + encoding.suffix
					etag = strings.TrimSuffix(meta.etag, `"`) + "-" + encoding.token + `"`
					w.Header().Set("Content-Encoding", encoding.token)
					break
				}
			}
		}
		f, err := assets.Open(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, name, time.Time{}, f.(io.ReadSeeker))
	})
}

// indexAssets hashes every embedded file once and records which have
// precompressed variants.
func indexAssets(assets fs.FS) map[string]*asset {
	index := map[string]*asset{}
	err := fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		for _, encoding := range encodings {
			if base, ok := strings.CutSuffix(name, encoding.suffix); ok {
				if _, err := fs.Stat(assets, base); err == nil {
					return nil
				}
			}
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		meta := &asset{etag: `"` + hex.EncodeToString(sum[:12]) + `"`, variants: map[string]bool{}}
		for _, encoding := range encodings {
			if _, err := fs.Stat(assets, name+encoding.suffix); err == nil {
				meta.variants[encoding.token] = true
			}
		}
		index[name] = meta
		return nil
	})
	if err != nil {
		panic(err)
	}
	return index
}

// acceptedEncodings parses Accept-Encoding, dropping codings with q=0.
func acceptedEncodings(header string) map[string]bool {
	accepted := map[string]bool{}
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		q := 1.0
		for _, param := range strings.Split(params, ";") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(param), "q="); ok {
				if parsed, err := strconv.ParseFloat(value, 64); err == nil {
					q = parsed
				}
			}
		}
		accepted[token] = q > 0
	}
	return accepted
}

func uiRoute(name string) bool {
	switch name {
	case "overview", "resources", "host", "alerts", "activity", "logs", "settings", "watch", "server", "events", "login", "setup", "onboarding":
		return true
	}
	return false
}
