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
