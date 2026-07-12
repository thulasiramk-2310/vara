package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repomanager"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/transport"
)

const cp = "/_vara/repositories"

// policyClock forces each policy write to have a strictly increasing mtime, so
// the authz store's mtime cache always observes an edit (works around coarse
// filesystem timestamp resolution — see the scanner NTFS note in project memory).
var policyClock int64

func putPolicy(t *testing.T, dir, repo, body string) {
	t.Helper()
	p := filepath.Join(dir, repo+".json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write policy %s: %v", repo, err)
	}
	policyClock++
	ft := time.Now().Add(time.Duration(policyClock) * time.Second)
	if err := os.Chtimes(p, ft, ft); err != nil {
		t.Fatalf("chtimes %s: %v", repo, err)
	}
}

// newAuthHub starts a Hub server with Basic identity + authorization + a manager,
// all sharing one policy root. Returns the server, the repo root, and policy root.
func newAuthHub(t *testing.T, creds map[string]string) (*httptest.Server, string, string) {
	t.Helper()
	root := t.TempDir()
	policyRoot := t.TempDir()
	metaDir := filepath.Join(t.TempDir(), "meta")
	mgr, err := repomanager.New(root, policyRoot, metaDir)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Identity: &identity.Multi{
			Sources:        []identity.IdentitySource{identity.NewBasicSource(creds)},
			AllowAnonymous: true,
		},
		Authz:   authz.NewEnforcer(authz.NewStore(policyRoot), nil),
		Methods: []string{"auth-basic"},
		Manager: mgr,
	}
	ts := httptest.NewServer(HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)
	return ts, root, policyRoot
}

// cpReq issues a control-plane request and returns status + body.
func cpReq(t *testing.T, method, url, auth, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// --- server-scope authorization + default-deny (M8) --------------------------

func TestControlCreateServerScope(t *testing.T) {
	ts, _, policyRoot := newAuthHub(t, map[string]string{"alice": "pw"})

	// No _server.json → default-deny → alice may not create (403).
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "pw"), `{"name":"proj"}`); st != http.StatusForbidden {
		t.Fatalf("create without grant = %d %s, want 403", st, body)
	} else if !strings.Contains(body, "UNAUTHORIZED") {
		t.Fatalf("body missing UNAUTHORIZED: %s", body)
	}

	// Grant alice create-repo on the server → 201.
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"alice":["create-repo","list-repos"]}}`)
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "pw"), `{"name":"proj"}`); st != http.StatusCreated {
		t.Fatalf("create with grant = %d %s, want 201", st, body)
	}

	// The owner (alice) was seeded with read, so the data plane now serves it.
	cli, _ := transport.OpenHTTP(ts.URL + "/proj")
	defer cli.Close()
	cli.SetBasicAuth("alice", "pw")
	if _, err := cli.ListRefs(); err != nil {
		t.Fatalf("owner should read the new repo over the data plane: %v", err)
	}
}

// TestControl401vs403 proves the split holds on the control plane too.
func TestControl401vs403(t *testing.T) {
	ts, _, policyRoot := newAuthHub(t, map[string]string{"alice": "pw", "bob": "pw"})
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"alice":["create-repo"]}}`)

	// Bad credential → 401.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "wrong"), `{"name":"x"}`); st != http.StatusUnauthorized {
		t.Fatalf("bad cred = %d %s, want 401", st, body)
	}
	// Valid identity lacking create-repo → 403.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, basic("bob", "pw"), `{"name":"x"}`); st != http.StatusForbidden {
		t.Fatalf("bob create = %d %s, want 403", st, body)
	}
	// Valid identity holding create-repo → 201.
	if st, _ := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "pw"), `{"name":"x"}`); st != http.StatusCreated {
		t.Fatalf("alice create want 201, got %d", st)
	}
}

// TestControlRenameDualCapability proves rename needs BOTH rename-repo (old) and
// create-repo (server) — RFC-0019 §5.7.
func TestControlRenameDualCapability(t *testing.T) {
	ts, _, policyRoot := newAuthHub(t, map[string]string{"alice": "pw", "carol": "pw"})
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"alice":["create-repo"]}}`)

	// alice creates "s"; then we grant carol only rename-repo on it (not server).
	if st, _ := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "pw"), `{"name":"s"}`); st != http.StatusCreated {
		t.Fatalf("seed create want 201, got %d", st)
	}
	putPolicy(t, policyRoot, "s", `{"version":1,"subjects":{"carol":["read","rename-repo"]}}`)

	// carol has rename-repo but not create-repo → 403.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp+"/s/rename", basic("carol", "pw"), `{"new_name":"s2"}`); st != http.StatusForbidden {
		t.Fatalf("rename with only rename-repo = %d %s, want 403", st, body)
	}

	// Grant carol create-repo on the server too → 200.
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"alice":["create-repo"],"carol":["create-repo"]}}`)
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp+"/s/rename", basic("carol", "pw"), `{"new_name":"s2"}`); st != http.StatusOK {
		t.Fatalf("rename with both caps = %d %s, want 200", st, body)
	}
}

// TestControlDeleteAuthorization proves a non-owner cannot delete (repo survives)
// and the owner can.
func TestControlDeleteAuthorization(t *testing.T) {
	ts, _, policyRoot := newAuthHub(t, map[string]string{"alice": "pw", "bob": "pw"})
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"alice":["create-repo"]}}`)
	if st, _ := cpReq(t, http.MethodPost, ts.URL+cp, basic("alice", "pw"), `{"name":"d"}`); st != http.StatusCreated {
		t.Fatalf("seed create want 201, got %d", st)
	}

	// bob lacks delete-repo on d → 403; the repo survives.
	if st, body := cpReq(t, http.MethodDelete, ts.URL+cp+"/d", basic("bob", "pw"), ""); st != http.StatusForbidden {
		t.Fatalf("bob delete = %d %s, want 403", st, body)
	}
	if st, _ := cpReq(t, http.MethodGet, ts.URL+cp+"/d", basic("alice", "pw"), ""); st != http.StatusOK {
		t.Fatalf("repo should survive a denied delete")
	}

	// alice (owner) holds delete-repo → 204. (Confirming absence via GET is
	// covered by the allow-all lifecycle test; here the policy is gone with the
	// repo, so a GET would be denied by authz-before-lookup rather than 404.)
	if st, _ := cpReq(t, http.MethodDelete, ts.URL+cp+"/d", basic("alice", "pw"), ""); st != http.StatusNoContent {
		t.Fatalf("owner delete want 204, got %d", st)
	}
	// The data plane no longer serves it (metadata gone).
	cli, _ := transport.OpenHTTP(ts.URL + "/d")
	defer cli.Close()
	cli.SetBasicAuth("alice", "pw")
	if _, err := cli.ListRefs(); err == nil {
		t.Fatalf("deleted repo must not be served on the data plane")
	}
}

// --- metadata-gated serving (M10) --------------------------------------------

// TestMetadataGatedServing proves only a manager-created (Active) repository is
// served: a hand-placed directory with no metadata is 404 even though its .vara
// exists, while a control-plane-created repository is served.
func TestMetadataGatedServing(t *testing.T) {
	// Allow-all hub (no authz) to isolate the metadata gate from authorization.
	root := t.TempDir()
	policyRoot := t.TempDir()
	metaDir := filepath.Join(t.TempDir(), "meta")
	mgr, err := repomanager.New(root, policyRoot, metaDir)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(HandlerWithOptions(root, Options{Manager: mgr}))
	t.Cleanup(ts.Close)

	// A hand-Init'd repo with NO metadata must not be served (M10).
	if _, err := repository.Init(filepath.Join(root, "ghost")); err != nil {
		t.Fatal(err)
	}
	ghost, _ := transport.OpenHTTP(ts.URL + "/ghost")
	defer ghost.Close()
	if _, err := ghost.ListRefs(); err == nil {
		t.Fatalf("hand-placed repo with no metadata must not be served")
	}

	// A control-plane-created repo IS served.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, "", `{"name":"real"}`); st != http.StatusCreated {
		t.Fatalf("create want 201, got %d %s", st, body)
	}
	real, _ := transport.OpenHTTP(ts.URL + "/real")
	defer real.Close()
	if _, err := real.ListRefs(); err != nil {
		t.Fatalf("Active managed repo should be served: %v", err)
	}
}

// --- full lifecycle over HTTP (allow-all) ------------------------------------

func TestControlLifecycleRoundTrip(t *testing.T) {
	root := t.TempDir()
	policyRoot := t.TempDir()
	metaDir := filepath.Join(t.TempDir(), "meta")
	mgr, err := repomanager.New(root, policyRoot, metaDir)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(HandlerWithOptions(root, Options{Manager: mgr}))
	t.Cleanup(ts.Close)

	if st, _ := cpReq(t, http.MethodPost, ts.URL+cp, "", `{"name":"proj"}`); st != http.StatusCreated {
		t.Fatalf("create want 201, got %d", st)
	}
	// Duplicate → 409 REPOSITORY_EXISTS.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp, "", `{"name":"proj"}`); st != http.StatusConflict || !strings.Contains(body, "REPOSITORY_EXISTS") {
		t.Fatalf("dup create = %d %s, want 409 REPOSITORY_EXISTS", st, body)
	}
	// List shows it.
	if st, body := cpReq(t, http.MethodGet, ts.URL+cp, "", ""); st != http.StatusOK || !strings.Contains(body, `"name":"proj"`) {
		t.Fatalf("list = %d %s", st, body)
	}
	// Rename.
	if st, body := cpReq(t, http.MethodPost, ts.URL+cp+"/proj/rename", "", `{"new_name":"proj2"}`); st != http.StatusOK || !strings.Contains(body, `"name":"proj2"`) {
		t.Fatalf("rename = %d %s", st, body)
	}
	// Old name gone, new name present.
	if st, _ := cpReq(t, http.MethodGet, ts.URL+cp+"/proj", "", ""); st != http.StatusNotFound {
		t.Fatalf("old name Get want 404, got %d", st)
	}
	if st, _ := cpReq(t, http.MethodGet, ts.URL+cp+"/proj2", "", ""); st != http.StatusOK {
		t.Fatalf("new name Get want 200, got %d", st)
	}
	// Delete.
	if st, _ := cpReq(t, http.MethodDelete, ts.URL+cp+"/proj2", "", ""); st != http.StatusNoContent {
		t.Fatalf("delete want 204, got %d", st)
	}
	// Reserved route → 501.
	if st, body := cpReq(t, http.MethodPut, ts.URL+cp+"/proj2/policy", "", `{}`); st != http.StatusNotImplemented || !strings.Contains(body, "NOT_IMPLEMENTED") {
		t.Fatalf("reserved policy route = %d %s, want 501 NOT_IMPLEMENTED", st, body)
	}
}
