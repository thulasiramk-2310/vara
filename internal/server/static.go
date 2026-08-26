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
			// Unknown path or directory → serve the SPA entry point.
			http.ServeFile(w, r, index)
			return
		}
		fs.ServeHTTP(w, r)
	})
}
