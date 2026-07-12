// SPDX-License-Identifier: AGPL-3.0-only

package webembed

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/* dist/assets/*
var files embed.FS

func Handler() http.Handler {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || uiRoute(name) {
			name = "index.html"
		}
		f, err := assets.Open(name)
		if err != nil && name != "index.html" {
			name = "index.html"
			f, err = assets.Open(name)
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		stat, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, name, stat.ModTime(), f.(io.ReadSeeker))
	})
}
func uiRoute(name string) bool {
	switch name {
	case "watch", "resources", "server", "events", "settings", "login", "setup", "onboarding":
		return true
	}
	return false
}
