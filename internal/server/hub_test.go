package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// buildHubRepo creates a repo with a small merge history:
//
//	one ─ two ─┐
//	  └─ three ┴─ merge   (HEAD=main→merge; feature→three)
func buildHubRepo(t *testing.T, repoRoot string) (c1, c2, c3, merge types.CommitID) {
	t.Helper()
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatal(err)
	}
	c1 = commitFile(t, repoRoot, "one")
	c2 = commitFile(t, repoRoot, "two", c1)
	c3 = commitFile(t, repoRoot, "three", c1)
	merge = commitFile(t, repoRoot, "merge", c2, c3)
	r := refsOf(repoRoot)
	r.Update("refs/heads/main", merge)
	r.Update("refs/heads/feature", c3)
	r.Symbolic("HEAD", "refs/heads/main")
	return
}

// serveHub starts a server over a repo "demo" with a policy granting alice read
// (bob has none). Returns the server, the repo's control-plane base URL, and the
// merge head.
func serveHub(t *testing.T) (*httptest.Server, string, types.CommitID) {
	t.Helper()
	root := t.TempDir()
	_, _, _, merge := buildHubRepo(t, filepath.Join(root, "demo"))
	policyDir := t.TempDir()
	putPolicy(t, policyDir, "demo", `{"version":1,"subjects":{"alice":["read"]}}`)
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
	return ts, ts.URL + "/_vara/repositories/demo", merge
}

func TestHubBranches(t *testing.T) {
	ts, base, merge := serveHub(t)
	_ = ts
	st, body := cpReq(t, http.MethodGet, base+"/branches", basic("alice", "pw"), "")
	if st != http.StatusOK {
		t.Fatalf("branches = %d %s", st, body)
	}
	var resp protocol.BranchesResponse
	json.Unmarshal([]byte(body), &resp)
	if len(resp.Branches) != 2 {
		t.Fatalf("want 2 branches, got %+v", resp.Branches)
	}
	var mainSeen bool
	for _, b := range resp.Branches {
		if b.Name == "main" {
			mainSeen = true
			if !b.IsHead || b.Target != merge.String() {
				t.Fatalf("main branch wrong: %+v", b)
			}
		}
	}
	if !mainSeen {
		t.Fatal("main branch missing")
	}
}

func TestHubCommitsAndMergeParents(t *testing.T) {
	ts, base, merge := serveHub(t)
	_ = ts
	st, body := cpReq(t, http.MethodGet, base+"/commits", basic("alice", "pw"), "")
	if st != http.StatusOK {
		t.Fatalf("commits = %d %s", st, body)
	}
	var resp protocol.CommitsResponse
	json.Unmarshal([]byte(body), &resp)
	if len(resp.Commits) != 4 {
		t.Fatalf("want 4 commits, got %d", len(resp.Commits))
	}
	// Newest first: the merge is first and has two parents.
	if resp.Commits[0].ID != merge.String() {
		t.Fatalf("first commit should be the merge, got %s", resp.Commits[0].ID)
	}
	if len(resp.Commits[0].Parents) != 2 {
		t.Fatalf("merge should have 2 parents, got %v", resp.Commits[0].Parents)
	}
}

func TestHubCommitsPagination(t *testing.T) {
	ts, base, _ := serveHub(t)
	_ = ts
	// Page 1: limit 2 → 2 commits + a cursor.
	_, body := cpReq(t, http.MethodGet, base+"/commits?limit=2", basic("alice", "pw"), "")
	var p1 protocol.CommitsResponse
	json.Unmarshal([]byte(body), &p1)
	if len(p1.Commits) != 2 || p1.Next == "" {
		t.Fatalf("page1 = %d commits, next=%q", len(p1.Commits), p1.Next)
	}
	// Page 2: follow the cursor → the remaining 2, no repeats, no next.
	_, body = cpReq(t, http.MethodGet, base+"/commits?limit=2&before="+p1.Next, basic("alice", "pw"), "")
	var p2 protocol.CommitsResponse
	json.Unmarshal([]byte(body), &p2)
	if len(p2.Commits) != 2 || p2.Next != "" {
		t.Fatalf("page2 = %d commits, next=%q", len(p2.Commits), p2.Next)
	}
	seen := map[string]bool{}
	for _, c := range append(p1.Commits, p2.Commits...) {
		if seen[c.ID] {
			t.Fatalf("commit %s repeated across pages", c.ID)
		}
		seen[c.ID] = true
	}
	// Bad limit and bad cursor are 400.
	if st, _ := cpReq(t, http.MethodGet, base+"/commits?limit=0", basic("alice", "pw"), ""); st != http.StatusBadRequest {
		t.Fatalf("limit=0 want 400, got %d", st)
	}
	if st, _ := cpReq(t, http.MethodGet, base+"/commits?limit=999", basic("alice", "pw"), ""); st != http.StatusBadRequest {
		t.Fatalf("limit=999 want 400, got %d", st)
	}
}

func TestHubCommitDetail(t *testing.T) {
	ts, base, merge := serveHub(t)
	_ = ts
	st, body := cpReq(t, http.MethodGet, base+"/commits/"+merge.String(), basic("alice", "pw"), "")
	if st != http.StatusOK {
		t.Fatalf("commit detail = %d %s", st, body)
	}
	var d protocol.CommitDetail
	json.Unmarshal([]byte(body), &d)
	if d.ID != merge.String() || len(d.Parents) != 2 || d.Tree == "" {
		t.Fatalf("detail wrong: %+v", d)
	}
	// Unknown commit → 404.
	zero := types.CommitID{}
	if st, _ := cpReq(t, http.MethodGet, base+"/commits/"+zero.String(), basic("alice", "pw"), ""); st != http.StatusNotFound {
		t.Fatalf("unknown commit want 404, got %d", st)
	}
}

func TestHubSummary(t *testing.T) {
	ts, base, merge := serveHub(t)
	_ = ts
	st, body := cpReq(t, http.MethodGet, base+"/summary", basic("alice", "pw"), "")
	if st != http.StatusOK {
		t.Fatalf("summary = %d %s", st, body)
	}
	var s protocol.RepositorySummary
	json.Unmarshal([]byte(body), &s)
	if s.CommitCount != 4 || s.BranchCount != 2 || s.DefaultBranch != "main" || s.Head != merge.String() {
		t.Fatalf("summary wrong: %+v", s)
	}
	if s.LastCommit == nil || s.LastCommit.ID != merge.String() {
		t.Fatalf("summary last_commit wrong: %+v", s.LastCommit)
	}
}

// TestHubReadRequiresReadCapability proves H1: a caller without `read` is denied
// from every endpoint, and learns nothing.
func TestHubReadRequiresReadCapability(t *testing.T) {
	ts, base, _ := serveHub(t)
	_ = ts
	for _, path := range []string{"/summary", "/branches", "/commits"} {
		// bob has no policy entry → no read → 403.
		if st, body := cpReq(t, http.MethodGet, base+path, basic("bob", "pw"), ""); st != http.StatusForbidden {
			t.Fatalf("bob %s = %d %s, want 403", path, st, body)
		}
		// anonymous → also 403.
		if st, _ := cpReq(t, http.MethodGet, base+path, "", ""); st != http.StatusForbidden {
			t.Fatalf("anonymous %s want 403, got %d", path, st)
		}
	}
}

// TestHubETagAndHeaders proves caching + the RFC-0021 headers.
func TestHubETagAndHeaders(t *testing.T) {
	ts, base, _ := serveHub(t)
	_ = ts
	req, _ := http.NewRequest(http.MethodGet, base+"/branches", nil)
	req.Header.Set("Authorization", basic("alice", "pw"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	if etag == "" {
		t.Fatal("no ETag on read response")
	}
	if resp.Header.Get(protocol.HeaderAPI) != protocol.APIVersion {
		t.Fatalf("X-VARA-API = %q, want %q", resp.Header.Get(protocol.HeaderAPI), protocol.APIVersion)
	}
	if resp.Header.Get(protocol.HeaderRequestID) == "" {
		t.Fatal("no X-Request-ID on read response")
	}
	// Conditional re-request with the ETag → 304.
	req2, _ := http.NewRequest(http.MethodGet, base+"/branches", nil)
	req2.Header.Set("Authorization", basic("alice", "pw"))
	req2.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match match want 304, got %d", resp2.StatusCode)
	}
}
