package server

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// withMiddleware wraps the whole API and static handler. In order, for every
// request it:
//
//  1. gives the request an ID, returned as X-Request-Id so a user reporting a
//     failure can quote it and an operator can find it in the log;
//  2. refuses a state-changing request that a browser sent from another site
//     (the Origin check, a second layer behind the SameSite=Strict cookie);
//  3. turns a panic inside a handler into a 500 for that one request, so a bug
//     in one route cannot take the process or other connections down.
func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestID(r)
		w.Header().Set("X-Request-Id", id)

		if isUnsafeMethod(r.Method) && crossSiteBrowserRequest(r) {
			writeError(w, http.StatusForbidden, "CROSS_ORIGIN",
				"this request was sent from a different site and was refused")
			return
		}

		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec) // net/http's own signal to drop the connection
				}
				log.Printf("vara serve: panic in %s %s [%s]: %v", r.Method, r.URL.Path, id, rec)
				writeError(w, http.StatusInternalServerError, protocol.CodeInternal,
					"internal error (request "+id+")")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// crossSiteBrowserRequest reports whether a browser marked this request as
// coming from another site. Non-browser clients (the vara command, curl) send no
// Origin and no Sec-Fetch-Site, so they are never refused here; they must still
// present credentials as before.
func crossSiteBrowserRequest(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" || (origin == "null" && r.Header.Get("Sec-Fetch-Site") == "") {
		// No Origin: not a browser cross-site request. A literal "null" origin
		// without fetch metadata is treated the same way.
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return true
	}
	// Behind a reverse proxy the Host header may be rewritten; honour the
	// forwarded host the proxy reports.
	for _, h := range []string{r.Host, r.Header.Get("X-Forwarded-Host")} {
		if h != "" && strings.EqualFold(h, u.Host) {
			return false
		}
	}
	return true
}

// requestID keeps a caller-supplied ID when it is short and made of safe
// characters (so it can be logged and echoed without escaping), and otherwise
// makes a fresh random one.
func requestID(r *http.Request) string {
	if v := r.Header.Get("X-Request-Id"); validRequestID(v) {
		return v
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

func validRequestID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
