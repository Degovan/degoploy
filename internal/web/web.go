// Package web serves the embedded SvelteKit SPA (built into ./dist by `make web`).
package web

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Precompressed siblings emitted by adapter-static, in order of preference.
var encodings = []struct{ name, ext string }{
	{"br", ".br"},
	{"gzip", ".gz"},
}

// Handler serves static assets and falls back to index.html for client-side routes.
func Handler() http.Handler {
	root, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				serveFile(w, r, root, name)
				return
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			// Missing files that look like assets are real 404s, not SPA routes.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}

		if _, err := fs.Stat(root, "index.html"); err != nil {
			http.Error(w, "web UI not built: run `make web`", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		serveFile(w, r, root, "index.html")
	})
}

// serveFile writes name, preferring a precompressed sibling the client accepts.
func serveFile(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	h := w.Header()
	h.Add("Vary", "Accept-Encoding")
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		h.Set("Content-Type", ct)
	}

	accept := r.Header.Get("Accept-Encoding")
	for _, enc := range encodings {
		if !acceptsEncoding(accept, enc.name) {
			continue
		}
		if f, err := root.Open(name + enc.ext); err == nil {
			defer f.Close()
			h.Set("Content-Encoding", enc.name)
			// Byte ranges would refer to the compressed stream; serve whole bodies only.
			r.Header.Del("Range")
			serveContent(w, r, name, f)
			return
		}
	}

	f, err := root.Open(name)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	serveContent(w, r, name, f)
}

func serveContent(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	// Embedded files carry no mod time; hashed asset names and no-cache handle freshness.
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	if r.Method != http.MethodHead {
		io.Copy(w, f)
	}
}

// acceptsEncoding reports whether an Accept-Encoding header allows enc (q > 0).
func acceptsEncoding(header, enc string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(token), enc) {
			continue
		}
		params = strings.TrimSpace(params)
		if q, ok := strings.CutPrefix(params, "q="); ok {
			v, err := strconv.ParseFloat(q, 64)
			return err == nil && v > 0
		}
		return true
	}
	return false
}
