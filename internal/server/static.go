package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// staticHandler serves the same-origin Hub UI from dir (RFC-0021 §8). It is a
// strict, read-only fallback: it serves an existing file directly, falls back to
// index.html for unknown paths (so a client-side-routed SPA works), refuses
// dotfiles, and confines every path within dir (no traversal). It is registered
// only on the least-specific "/" pattern, so it can never shadow an API or
// data-plane route (H3).
func staticHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Baseline hardening for the Hub UI (defense-in-depth). The app never uses
		// dangerouslySetInnerHTML and loads only same-origin assets, so a strict
		// same-origin policy is safe. 'unsafe-inline' is kept for style-src only
		// because the UI uses inline React styles; script-src stays strict.
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if requestIsHTTPS(r) {
			// Only meaningful (and only honored) over HTTPS.
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Reject dotfiles / dot-segments outright.
		if strings.Contains(r.URL.Path, "/.") {
			http.NotFound(w, r)
			return
		}
		// Clean to a rooted path so ".." can never escape dir, then resolve.
		clean := filepath.Clean("/" + r.URL.Path)
		full := filepath.Join(dir, clean)
		if fi, err := os.Stat(full); err != nil || fi.IsDir() {
			// Build output lives under /assets/. A missing file there is a real 404,
			// so a stale page never receives HTML where it asked for JavaScript or
			// CSS. Other paths are app routes and may contain dots (a deep link to
			// /r/demo/blob/README.md), so only this prefix is treated as a file.
			if strings.HasPrefix(clean, "/assets/") {
				http.NotFound(w, r)
				return
			}
			// An app route: serve the SPA entry point. It must be revalidated on
			// every load so a new deploy is picked up at once.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, index)
			return
		}
		switch {
		case strings.HasPrefix(clean, "/assets/"):
			// Build output names these files by content hash, so they never change.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}
