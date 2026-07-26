package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/hub"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// buildSearchRepo creates a three-commit linear history over a small tree:
//
//	c1 (root):  "initial commit"          author Alice
//	c2 (c1):    "add feature TODO"         author Bob
//	c3 (c2):    "fix bug in helper"        author Alice   (HEAD=main)
//
// The HEAD tree:
//
//	README.md        "hello world\nTODO: write docs\n"
//	notes.txt        "nothing to see\n"
//	data.bin         "SECRETBIN\x00MARKER"  (binary — NUL)
//	docs/guide.md    "guide\nsome content\n"
//	src/main.go      "package main\n\nfunc main() { // TODO wire up\n}\n"
//	src/util.go      "package main\n\nfunc helper() {}\n"
func buildSearchRepo(t *testing.T, repoRoot string) (c1, c2, c3 types.CommitID) {
	t.Helper()
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatal(err)
	}
	s := storeOf(repoRoot)

	readme := writeObj(t, s, object.NewBlob([]byte("hello world\nTODO: write docs\n")))
	notes := writeObj(t, s, object.NewBlob([]byte("nothing to see\n")))
	binDat := writeObj(t, s, object.NewBlob([]byte("SECRETBIN\x00MARKER")))
	guide := writeObj(t, s, object.NewBlob([]byte("guide\nsome content\n")))
	mainGo := writeObj(t, s, object.NewBlob([]byte("package main\n\nfunc main() { // TODO wire up\n}\n")))
	utilGo := writeObj(t, s, object.NewBlob([]byte("package main\n\nfunc helper() {}\n")))

	srcTree := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: mainGo, Name: "main.go"},
		{Mode: 0o100644, Hash: utilGo, Name: "util.go"},
	}))
	docsTree := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: guide, Name: "guide.md"},
	}))
	root := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readme, Name: "README.md"},
		{Mode: 0o100644, Hash: notes, Name: "notes.txt"},
		{Mode: 0o100644, Hash: binDat, Name: "data.bin"},
		{Mode: 0o040000, Hash: docsTree, Name: "docs"},
		{Mode: 0o040000, Hash: srcTree, Name: "src"},
	}))

	c1 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root), Author: "Alice", Message: "initial commit"}))
	c2 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root), Parents: []types.CommitID{c1}, Author: "Bob", Message: "add feature TODO"}))
	c3 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root), Parents: []types.CommitID{c2}, Author: "Alice", Message: "fix bug in helper"}))

	r := refsOf(repoRoot)
	r.Update("refs/heads/main", c3)
	r.Symbolic("HEAD", "refs/heads/main")
	return
}

func serveSearch(t *testing.T) (base string, c1, c2, c3 types.CommitID) {
	t.Helper()
	root := t.TempDir()
	c1, c2, c3 = buildSearchRepo(t, filepath.Join(root, "demo"))
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
	return ts.URL + "/_vara/repositories/demo", c1, c2, c3
}

func getSearchCommits(t *testing.T, base, query string) protocol.SearchCommitsResponse {
	t.Helper()
	_, body := cpReq(t, http.MethodGet, base+"/search/commits?q="+query, basic("alice", "pw"), "")
	var r protocol.SearchCommitsResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, body)
	}
	return r
}

func idSet(cs []protocol.CommitSummary) map[string]bool {
	m := map[string]bool{}
	for _, c := range cs {
		m[c.ID] = true
	}
	return m
}

// TestSearchCommits proves §5.1: matching by message and author, `in` field
// scoping, empty result, and limit-based paging.
func TestSearchCommits(t *testing.T) {
	base, c1, c2, c3 := serveSearch(t)

	// Default (both fields, insensitive): "TODO" → only c2's message.
	got := getSearchCommits(t, base, "TODO")
	if len(got.Matches) != 1 || got.Matches[0].ID != c2.String() {
		t.Fatalf("q=TODO want [c2], got %+v", got.Matches)
	}
	if got.RefCommit != c3.String() {
		t.Fatalf("ref_commit = %s, want HEAD c3 %s", got.RefCommit, c3.String())
	}

	// Author match: "Alice" → c1 and c3 (default both fields still matches author).
	got = getSearchCommits(t, base, "Alice")
	if s := idSet(got.Matches); len(s) != 2 || !s[c1.String()] || !s[c3.String()] {
		t.Fatalf("q=Alice want {c1,c3}, got %+v", got.Matches)
	}

	// in=message only: "Alice" appears in no message → empty.
	_, body := cpReq(t, http.MethodGet, base+"/search/commits?q=Alice&in=message", basic("alice", "pw"), "")
	var msgOnly protocol.SearchCommitsResponse
	json.Unmarshal([]byte(body), &msgOnly)
	if len(msgOnly.Matches) != 0 {
		t.Fatalf("in=message q=Alice want empty, got %+v", msgOnly.Matches)
	}

	// No match → 200 empty.
	if got = getSearchCommits(t, base, "zzznope"); len(got.Matches) != 0 || got.Truncated {
		t.Fatalf("no-match want empty untruncated, got %+v trunc=%v", got.Matches, got.Truncated)
	}

	// limit=1 over the two Alice matches → one match + a next cursor (not truncated).
	_, body = cpReq(t, http.MethodGet, base+"/search/commits?q=Alice&limit=1", basic("alice", "pw"), "")
	var page1 protocol.SearchCommitsResponse
	json.Unmarshal([]byte(body), &page1)
	if len(page1.Matches) != 1 || page1.Next == "" || page1.Truncated {
		t.Fatalf("limit=1 page1 want 1 match + next, got %+v next=%q trunc=%v", page1.Matches, page1.Next, page1.Truncated)
	}
	_, body = cpReq(t, http.MethodGet, base+"/search/commits?q=Alice&limit=1&before="+page1.Next, basic("alice", "pw"), "")
	var page2 protocol.SearchCommitsResponse
	json.Unmarshal([]byte(body), &page2)
	all := map[string]bool{}
	for _, c := range append(page1.Matches, page2.Matches...) {
		all[c.ID] = true
	}
	if !all[c1.String()] || !all[c3.String()] {
		t.Fatalf("paged Alice search dropped a match: %v", all)
	}
}

// TestSearchCommitsBudget proves the traversal budget (§7): a tiny walk budget
// truncates and yields a `next` that resumes without skipping or repeating.
func TestSearchCommitsBudget(t *testing.T) {
	base, c1, _, c3 := serveSearch(t)
	old := hub.SetMaxSearchWalk(1) // examine one commit per request
	defer hub.SetMaxSearchWalk(old)

	seen := map[string]bool{}
	before := ""
	pages := 0
	for {
		url := base + "/search/commits?q=Alice"
		if before != "" {
			url += "&before=" + before
		}
		_, body := cpReq(t, http.MethodGet, url, basic("alice", "pw"), "")
		var r protocol.SearchCommitsResponse
		json.Unmarshal([]byte(body), &r)
		for _, c := range r.Matches {
			if seen[c.ID] {
				t.Fatalf("commit %s repeated across budget pages", c.ID)
			}
			seen[c.ID] = true
		}
		if r.Next == "" {
			break
		}
		before = r.Next
		if pages++; pages > 20 {
			t.Fatal("budget paging did not terminate")
		}
	}
	if !seen[c1.String()] || !seen[c3.String()] || len(seen) != 2 {
		t.Fatalf("budget paging want exactly {c1,c3}, got %v", seen)
	}
}

// TestSearchPaths proves §5.2: file-name matching over the whole tree (nested
// included), directory matches, the returned fields, and sorted order.
func TestSearchPaths(t *testing.T) {
	base, _, _, c3 := serveSearch(t)

	_, body := cpReq(t, http.MethodGet, base+"/search/paths?q=main", basic("alice", "pw"), "")
	var r protocol.SearchPathsResponse
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 1 || r.Matches[0].Path != "src/main.go" || r.Matches[0].IsDir {
		t.Fatalf("q=main want [src/main.go file], got %+v", r.Matches)
	}
	if r.Matches[0].Mode != "100644" || r.Matches[0].Blob == "" {
		t.Fatalf("path match fields wrong: %+v", r.Matches[0])
	}
	if r.RefCommit != c3.String() {
		t.Fatalf("ref_commit = %s, want c3", r.RefCommit)
	}

	// "src" matches the directory and both files under it, sorted by path.
	_, body = cpReq(t, http.MethodGet, base+"/search/paths?q=src", basic("alice", "pw"), "")
	json.Unmarshal([]byte(body), &r)
	var paths []string
	var sawDir bool
	for _, m := range r.Matches {
		paths = append(paths, m.Path)
		if m.Path == "src" && m.IsDir {
			sawDir = true
		}
	}
	want := []string{"src", "src/main.go", "src/util.go"}
	if len(paths) != 3 || paths[0] != want[0] || paths[1] != want[1] || paths[2] != want[2] {
		t.Fatalf("q=src want %v in order, got %v", want, paths)
	}
	if !sawDir {
		t.Fatal("q=src should match the src directory itself")
	}
}

// TestSearchContent proves §5.3: line matching with 1-based numbers, binary skip
// (S4), and sorted order.
func TestSearchContent(t *testing.T) {
	base, _, _, _ := serveSearch(t)

	_, body := cpReq(t, http.MethodGet, base+"/search/content?q=TODO", basic("alice", "pw"), "")
	var r protocol.SearchContentResponse
	json.Unmarshal([]byte(body), &r)
	byPath := map[string][]protocol.ContentLine{}
	for _, m := range r.Matches {
		byPath[m.Path] = m.Lines
	}
	if len(r.Matches) != 2 {
		t.Fatalf("q=TODO want 2 files, got %+v", r.Matches)
	}
	if r.Matches[0].Path != "README.md" || r.Matches[1].Path != "src/main.go" {
		t.Fatalf("content matches not sorted by path: %+v", r.Matches)
	}
	if ls := byPath["README.md"]; len(ls) != 1 || ls[0].Line != 2 || ls[0].Content != "TODO: write docs" {
		t.Fatalf("README match wrong: %+v", ls)
	}
	if ls := byPath["src/main.go"]; len(ls) != 1 || ls[0].Line != 3 {
		t.Fatalf("main.go match line wrong: %+v", ls)
	}

	// Binary skip: "SECRETBIN" lives only in data.bin (has a NUL) → never scanned.
	_, body = cpReq(t, http.MethodGet, base+"/search/content?q=SECRETBIN", basic("alice", "pw"), "")
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 0 {
		t.Fatalf("binary content must be skipped, got %+v", r.Matches)
	}
}

// TestSearchContentBounds proves §7: an oversize file is skipped (not binary), and
// the result cap / bytes budget set truncated.
func TestSearchContentBounds(t *testing.T) {
	base, _, _, _ := serveSearch(t)

	// Per-file cap below every file → all skipped, no matches, no error.
	oldFile := hub.SetMaxSearchFileBytes(4)
	_, body := cpReq(t, http.MethodGet, base+"/search/content?q=package", basic("alice", "pw"), "")
	hub.SetMaxSearchFileBytes(oldFile)
	var r protocol.SearchContentResponse
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 0 {
		t.Fatalf("tiny per-file cap should skip all files, got %+v", r.Matches)
	}

	// Bytes budget of 1 → first scanned file overflows → truncated.
	oldScan := hub.SetMaxSearchScanBytes(1)
	st, body := cpReq(t, http.MethodGet, base+"/search/content?q=package", basic("alice", "pw"), "")
	hub.SetMaxSearchScanBytes(oldScan)
	json.Unmarshal([]byte(body), &r)
	if st != http.StatusOK || !r.Truncated {
		t.Fatalf("over bytes budget want truncated 200, got %d trunc=%v", st, r.Truncated)
	}
}

// TestSearchQuerySemantics proves §6: literal substring (not regex), case
// sensitivity, and the 400s.
func TestSearchQuerySemantics(t *testing.T) {
	base, _, _, _ := serveSearch(t)

	// Literal: "TODO." (dot) does not occur ("TODO:" does); a regex would match.
	_, body := cpReq(t, http.MethodGet, base+"/search/content?q=TODO.", basic("alice", "pw"), "")
	var r protocol.SearchContentResponse
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 0 {
		t.Fatalf("literal q=TODO. should not match TODO:, got %+v", r.Matches)
	}

	// Case-sensitive "todo" matches nothing (files use uppercase TODO)...
	_, body = cpReq(t, http.MethodGet, base+"/search/content?q=todo&case=sensitive", basic("alice", "pw"), "")
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 0 {
		t.Fatalf("case=sensitive q=todo want 0, got %+v", r.Matches)
	}
	// ...but case-insensitive (default) matches both.
	_, body = cpReq(t, http.MethodGet, base+"/search/content?q=todo", basic("alice", "pw"), "")
	json.Unmarshal([]byte(body), &r)
	if len(r.Matches) != 2 {
		t.Fatalf("case-insensitive q=todo want 2, got %+v", r.Matches)
	}

	// 400s: empty q, unknown case, unknown in.
	for _, p := range []string{"/search/commits?q=", "/search/paths?q=x&case=weird", "/search/commits?q=x&in=nope"} {
		if st, _ := cpReq(t, http.MethodGet, base+p, basic("alice", "pw"), ""); st != http.StatusBadRequest {
			t.Fatalf("%s want 400, got %d", p, st)
		}
	}
}

// TestSearchRequiresRead proves S1: every search endpoint gates on `read`.
func TestSearchRequiresRead(t *testing.T) {
	base, _, _, _ := serveSearch(t)
	for _, p := range []string{"/search/commits?q=x", "/search/paths?q=x", "/search/content?q=x"} {
		if st, _ := cpReq(t, http.MethodGet, base+p, basic("bob", "pw"), ""); st != http.StatusForbidden {
			t.Fatalf("bob %s want 403, got %d", p, st)
		}
		if st, _ := cpReq(t, http.MethodGet, base+p, "", ""); st != http.StatusForbidden {
			t.Fatalf("anonymous %s want 403, got %d", p, st)
		}
	}
}

// TestSearchETag proves §9: content-addressed ETag + If-None-Match → 304, and a
// changed query changes the ETag.
func TestSearchETag(t *testing.T) {
	base, _, _, _ := serveSearch(t)
	for _, p := range []string{"/search/commits?q=fix", "/search/paths?q=go", "/search/content?q=package"} {
		etag := etagOf(t, base+p)
		if etag == "" {
			t.Fatalf("%s: no ETag", p)
		}
		req, _ := http.NewRequest(http.MethodGet, base+p, nil)
		req.Header.Set("Authorization", basic("alice", "pw"))
		req.Header.Set("If-None-Match", etag)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: If-None-Match want 304, got %d", p, resp.StatusCode)
		}
	}
	// A different query yields a different ETag (different response).
	if etagOf(t, base+"/search/content?q=package") == etagOf(t, base+"/search/content?q=helper") {
		t.Fatal("distinct queries must not share an ETag")
	}
}

func etagOf(t *testing.T, url string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", basic("alice", "pw"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.Header.Get("ETag")
}
