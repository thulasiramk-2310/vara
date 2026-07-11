package server

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/transport"
)

// serveWith starts a server with the given options over a fresh root holding one
// initialized repo, and returns the server, the repo working root, and base URL.
func serveWith(t *testing.T, repoName string, opts Options) (*httptest.Server, string, string) {
	t.Helper()
	root := t.TempDir()
	repoRoot := filepath.Join(root, repoName)
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatalf("init: %v", err)
	}
	ts := httptest.NewServer(HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)
	return ts, repoRoot, ts.URL + "/" + repoName
}

// get performs GET base+/info/refs with an optional Authorization header.
func get(t *testing.T, base, authHeader string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/info/refs", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func basic(user, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+secret))
}

func writePolicyFile(t *testing.T, dir, repo, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, repo+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

// --- Proof 2: identity (Basic auth) --------------------------------------

func TestHTTPBasicAuth(t *testing.T) {
	opts := Options{
		Identity: identity.NewBasicSource(map[string]string{"alice": "pw"}),
		Methods:  []string{"auth-basic"},
	}
	_, _, base := serveWith(t, "srv", opts)

	// No credential → 401 with WWW-Authenticate.
	if status, body := get(t, base, ""); status != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d body=%s, want 401", status, body)
	} else if !strings.Contains(body, "UNAUTHENTICATED") {
		t.Fatalf("no-auth body missing UNAUTHENTICATED: %s", body)
	}

	// Valid credential → 200.
	if status, body := get(t, base, basic("alice", "pw")); status != http.StatusOK {
		t.Fatalf("valid-auth status = %d body=%s, want 200", status, body)
	}

	// Wrong password → 401.
	if status, _ := get(t, base, basic("alice", "nope")); status != http.StatusUnauthorized {
		t.Fatalf("wrong-password status = %d, want 401", status)
	}
}

func TestHTTPWWWAuthenticateHeader(t *testing.T) {
	opts := Options{
		Identity: identity.NewBasicSource(map[string]string{"alice": "pw"}),
		Methods:  []string{"auth-basic"},
	}
	_, _, base := serveWith(t, "srv", opts)
	req, _ := http.NewRequest(http.MethodGet, base+"/info/refs", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if wa := resp.Header.Get("WWW-Authenticate"); !strings.Contains(wa, "Basic") {
		t.Fatalf("WWW-Authenticate = %q, want it to advertise Basic", wa)
	}
}

// TestAuthPrecedesRepoLookup proves ordering: an unauthenticated request to a
// NONEXISTENT repository returns 401 (authentication), not 404 (repo lookup) —
// so authentication runs before the transport is consulted (RFC-0018 A10).
func TestAuthPrecedesRepoLookup(t *testing.T) {
	opts := Options{
		Identity: identity.NewBasicSource(map[string]string{"alice": "pw"}),
		Methods:  []string{"auth-basic"},
	}
	ts, _, _ := serveWith(t, "srv", opts)
	// Hit a repo that does not exist, with no credential.
	status, _ := get(t, ts.URL+"/does-not-exist", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (auth before repo lookup)", status)
	}
}

// --- Proof 3: authorization (403 before transport) -----------------------

func TestHTTPAuthorizationDeniesPush(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "srv")
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Policy: anonymous may read but not push.
	policyDir := t.TempDir()
	writePolicyFile(t, policyDir, "srv", `{"version":1,"subjects":{"anonymous":["read"]}}`)
	enf := authz.NewEnforcer(authz.NewStore(policyDir), nil)

	ts := httptest.NewServer(HandlerWithOptions(root, Options{Authz: enf}))
	t.Cleanup(ts.Close)

	// Seed a base commit and set main so a push has something to move.
	c1 := commitFile(t, repoRoot, "one")
	refsOf(repoRoot).Update("refs/heads/main", c1)
	c2 := commitFile(t, repoRoot, "two", c1)

	cli, err := transport.OpenHTTP(ts.URL + "/srv")
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	// Read is allowed (anonymous has `read`).
	if _, err := cli.ListRefs(); err != nil {
		t.Fatalf("anonymous read should be allowed: %v", err)
	}

	// Push is denied → the transport returns an error carrying UNAUTHORIZED, and
	// the server ref MUST be unchanged (the transport was never invoked).
	_, err = cli.ReceivePack(emptyPack(t, repoRoot), []transport.RefUpdate{
		{Name: "refs/heads/main", Old: c1, New: c2},
	})
	if err == nil || !strings.Contains(err.Error(), "UNAUTHORIZED") {
		t.Fatalf("push should be denied with UNAUTHORIZED, got err=%v", err)
	}
	if got, _ := refsOf(repoRoot).Resolve("refs/heads/main"); got != c1 {
		t.Fatalf("server ref moved despite 403: %s (transport must not have run)", got.String()[:7])
	}
}

// TestHTTP401vs403 proves the two are never swapped: a bad credential is 401,
// a valid identity lacking a capability is 403.
func TestHTTP401vs403(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "srv")
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatalf("init: %v", err)
	}
	policyDir := t.TempDir()
	// alice may read but not push; bob may push.
	writePolicyFile(t, policyDir, "srv", `{"version":1,"subjects":{"alice":["read"],"bob":["read","push"]}}`)

	opts := Options{
		Identity: &identity.Multi{
			Sources:        []identity.IdentitySource{identity.NewBasicSource(map[string]string{"alice": "pw", "bob": "pw"})},
			AllowAnonymous: true,
		},
		Authz:   authz.NewEnforcer(authz.NewStore(policyDir), nil),
		Methods: []string{"auth-basic"},
	}
	ts := httptest.NewServer(HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)

	c1 := commitFile(t, repoRoot, "one")
	refsOf(repoRoot).Update("refs/heads/main", c1)
	c2 := commitFile(t, repoRoot, "two", c1)

	push := func(user string) error {
		cli, err := transport.OpenHTTP(ts.URL + "/srv")
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if user != "" {
			cli.SetBasicAuth(user, "pw")
		}
		_, err = cli.ReceivePack(emptyPack(t, repoRoot), []transport.RefUpdate{
			{Name: "refs/heads/main", Old: c1, New: c2},
		})
		return err
	}

	// Bad credential → 401 UNAUTHENTICATED (authentication failure).
	cli, _ := transport.OpenHTTP(ts.URL + "/srv")
	cli.SetBasicAuth("alice", "wrongpw")
	if _, err := cli.ListRefs(); err == nil || !strings.Contains(err.Error(), "UNAUTHENTICATED") {
		t.Fatalf("bad credential should be 401 UNAUTHENTICATED, got %v", err)
	}
	cli.Close()

	// alice (valid identity) lacks push → 403 UNAUTHORIZED (authorization failure).
	if err := push("alice"); err == nil || !strings.Contains(err.Error(), "UNAUTHORIZED") {
		t.Fatalf("alice push should be 403 UNAUTHORIZED, got %v", err)
	}
	if got, _ := refsOf(repoRoot).Resolve("refs/heads/main"); got != c1 {
		t.Fatalf("ref moved despite alice's 403: %s", got.String()[:7])
	}

	// bob has push → succeeds.
	if err := push("bob"); err != nil {
		t.Fatalf("bob push should succeed: %v", err)
	}
	if got, _ := refsOf(repoRoot).Resolve("refs/heads/main"); got != c2 {
		t.Fatalf("bob push did not land: %s", got.String()[:7])
	}
}
