package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticHubServing(t *testing.T) {
	dir := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("index.html", "<html>vara hub</html>")
	must("app.js", "console.log('hub')")
	must(".secret", "do not serve")

	ts := httptest.NewServer(HandlerWithOptions(t.TempDir(), Options{HubDir: dir}))
	t.Cleanup(ts.Close)

	// Root serves index.html.
	if _, b := cpReq(t, http.MethodGet, ts.URL+"/", "", ""); !strings.Contains(b, "vara hub") {
		t.Fatalf("GET / did not serve index.html: %q", b)
	}
	// A real asset is served.
	if _, b := cpReq(t, http.MethodGet, ts.URL+"/app.js", "", ""); !strings.Contains(b, "console.log") {
		t.Fatalf("GET /app.js not served: %q", b)
	}
	// SPA fallback: an unknown client route serves index.html.
	if _, b := cpReq(t, http.MethodGet, ts.URL+"/repositories/demo/history", "", ""); !strings.Contains(b, "vara hub") {
		t.Fatalf("SPA fallback did not serve index.html: %q", b)
	}
	// The API is NOT shadowed by static serving (H3).
	if st, b := cpReq(t, http.MethodGet, ts.URL+"/_vara/whoami", "", ""); st != http.StatusOK || !strings.Contains(b, "anonymous") {
		t.Fatalf("whoami shadowed by static: %d %s", st, b)
	}
	// Dotfiles are refused.
	if st, _ := cpReq(t, http.MethodGet, ts.URL+"/.secret", "", ""); st != http.StatusNotFound {
		t.Fatalf("dotfile served, want 404, got %d", st)
	}
}

func TestCookieSession(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	if err := mgr.CreateAccount("alice", "password1"); err != nil {
		t.Fatal(err)
	}

	// Cookie-mode login: secret withheld from the body, set as an httpOnly cookie.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/_vara/sessions?cookie=1", strings.NewReader(`{"username":"alice","password":"password1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), `"secret"`) {
		t.Fatalf("cookie login leaked the secret in the body: %s", body)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("no session cookie set")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie not httpOnly+SameSite=Strict: %+v", cookie)
	}

	// The cookie authenticates a subsequent request.
	get := func(c *http.Cookie) int {
		r, _ := http.NewRequest(http.MethodGet, ts.URL+"/_vara/tokens", nil)
		if c != nil {
			r.AddCookie(c)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if st := get(cookie); st != http.StatusOK {
		t.Fatalf("cookie should authenticate, got %d", st)
	}

	// Logout via the cookie revokes the session immediately (S6).
	lr, _ := http.NewRequest(http.MethodDelete, ts.URL+"/_vara/sessions/current", nil)
	lr.AddCookie(cookie)
	lresp, err := http.DefaultClient.Do(lr)
	if err != nil {
		t.Fatal(err)
	}
	lresp.Body.Close()
	if lresp.StatusCode != http.StatusNoContent {
		t.Fatalf("cookie logout want 204, got %d", lresp.StatusCode)
	}
	if st := get(cookie); st != http.StatusUnauthorized {
		t.Fatalf("revoked cookie still authenticates, got %d", st)
	}
}
