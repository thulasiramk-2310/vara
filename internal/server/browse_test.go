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
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// These tests exercise the RFC-0022 browse API over the full HTTP stack. The
// load-bearing one is TestBrowseRawIsInert (B5): repository bytes — including a
// hand-crafted .html — are served inert on the Hub origin.

func writeObj(t *testing.T, s *object.Store, o object.Object) types.ObjectID {
	t.Helper()
	id, err := s.Write(o)
	if err != nil {
		t.Fatalf("write object: %v", err)
	}
	return id
}

// Fixture blob contents.
var (
	htmlContent = []byte("<html><script>alert('xss')</script></html>")
	binContent  = []byte{0x00, 0x01, 0x02, 'A', 0x00} // contains NUL → binary
	latin1Bytes = []byte{0xff, 0xfe, 'h', 'i'}        // NUL-free but invalid UTF-8
)

// buildBrowseRepo creates a repo with nested trees and a three-commit history:
//
//	c1: README.md=v1, src/main.go=v1                      (introduces both)
//	c2: README.md=v2, src/main.go=v1                      (changes README only)
//	c3: README.md=v2, src/main.go=v2, + page.html/data.bin/latin1.txt (changes main + adds files)
//
// HEAD=main→c3. So README history is [c2,c1] and src/main.go history is [c3,c1].
func buildBrowseRepo(t *testing.T, repoRoot string) (c1, c2, c3 types.CommitID) {
	t.Helper()
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatal(err)
	}
	s := storeOf(repoRoot)

	readmeV1 := writeObj(t, s, object.NewBlob([]byte("readme one")))
	readmeV2 := writeObj(t, s, object.NewBlob([]byte("readme two")))
	mainV1 := writeObj(t, s, object.NewBlob([]byte("package main // v1")))
	mainV2 := writeObj(t, s, object.NewBlob([]byte("package main // v2")))
	html := writeObj(t, s, object.NewBlob(htmlContent))
	bin := writeObj(t, s, object.NewBlob(binContent))
	latin1 := writeObj(t, s, object.NewBlob(latin1Bytes))

	srcV1 := writeObj(t, s, object.NewTree([]object.TreeEntry{{Mode: 0o100644, Hash: mainV1, Name: "main.go"}}))
	srcV2 := writeObj(t, s, object.NewTree([]object.TreeEntry{{Mode: 0o100644, Hash: mainV2, Name: "main.go"}}))

	root1 := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readmeV1, Name: "README.md"},
		{Mode: 0o040000, Hash: srcV1, Name: "src"},
	}))
	root2 := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readmeV2, Name: "README.md"},
		{Mode: 0o040000, Hash: srcV1, Name: "src"},
	}))
	root3 := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readmeV2, Name: "README.md"},
		{Mode: 0o040000, Hash: srcV2, Name: "src"},
		{Mode: 0o100644, Hash: html, Name: "page.html"},
		{Mode: 0o100644, Hash: bin, Name: "data.bin"},
		{Mode: 0o100644, Hash: latin1, Name: "latin1.txt"},
	}))

	c1 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root1), Author: "T", Message: "c1"}))
	c2 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root2), Parents: []types.CommitID{c1}, Author: "T", Message: "c2"}))
	c3 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root3), Parents: []types.CommitID{c2}, Author: "T", Message: "c3"}))

	r := refsOf(repoRoot)
	r.Update("refs/heads/main", c3)
	r.Symbolic("HEAD", "refs/heads/main")
	return
}

// serveBrowse starts a server over repo "demo" with alice granted read, bob none.
func serveBrowse(t *testing.T) (base string, c1, c2, c3 types.CommitID) {
	t.Helper()
	root := t.TempDir()
	c1, c2, c3 = buildBrowseRepo(t, filepath.Join(root, "demo"))
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

func TestBrowseTreeListing(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	// Root listing: five entries in name order, with correct type/mode.
	st, body := cpReq(t, http.MethodGet, base+"/tree/", basic("alice", "pw"), "")
	if st != http.StatusOK {
		t.Fatalf("tree root = %d %s", st, body)
	}
	var root protocol.TreeResponse
	json.Unmarshal([]byte(body), &root)
	want := []struct{ name, typ, mode string }{
		{"README.md", "file", "100644"},
		{"data.bin", "file", "100644"},
		{"latin1.txt", "file", "100644"},
		{"page.html", "file", "100644"},
		{"src", "dir", "40000"},
	}
	if len(root.Entries) != len(want) {
		t.Fatalf("root entries = %d, want %d: %+v", len(root.Entries), len(want), root.Entries)
	}
	for i, w := range want {
		e := root.Entries[i]
		if e.Name != w.name || e.Type != w.typ || e.Mode != w.mode {
			t.Fatalf("entry %d = %+v, want %v/%v/%v", i, e, w.name, w.typ, w.mode)
		}
	}
	// Sub-path listing.
	_, body = cpReq(t, http.MethodGet, base+"/tree/src", basic("alice", "pw"), "")
	var sub protocol.TreeResponse
	json.Unmarshal([]byte(body), &sub)
	if len(sub.Entries) != 1 || sub.Entries[0].Name != "main.go" {
		t.Fatalf("src listing wrong: %+v", sub.Entries)
	}
	// A file path to `tree` is 404.
	if st, _ := cpReq(t, http.MethodGet, base+"/tree/README.md", basic("alice", "pw"), ""); st != http.StatusNotFound {
		t.Fatalf("tree of a file want 404, got %d", st)
	}
}

func TestBrowseBlobTextBinaryTruncated(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	// Text file: inline utf-8 content.
	_, body := cpReq(t, http.MethodGet, base+"/blob/README.md", basic("alice", "pw"), "")
	var text protocol.BlobResponse
	json.Unmarshal([]byte(body), &text)
	if text.Binary || text.Truncated || text.Encoding != "utf-8" || text.Content != "readme two" {
		t.Fatalf("text blob wrong: %+v", text)
	}
	// Nested text file.
	_, body = cpReq(t, http.MethodGet, base+"/blob/src/main.go", basic("alice", "pw"), "")
	var nested protocol.BlobResponse
	json.Unmarshal([]byte(body), &nested)
	if nested.Content != "package main // v2" {
		t.Fatalf("nested blob content = %q", nested.Content)
	}
	// Binary (NUL) file: flagged, no inline content.
	_, body = cpReq(t, http.MethodGet, base+"/blob/data.bin", basic("alice", "pw"), "")
	var bin protocol.BlobResponse
	json.Unmarshal([]byte(body), &bin)
	if !bin.Binary || bin.Content != "" || bin.Encoding != "binary" {
		t.Fatalf("binary blob wrong: %+v", bin)
	}
	// NUL-free but invalid-UTF-8 file: also binary (must not corrupt JSON).
	_, body = cpReq(t, http.MethodGet, base+"/blob/latin1.txt", basic("alice", "pw"), "")
	var latin protocol.BlobResponse
	json.Unmarshal([]byte(body), &latin)
	if !latin.Binary || latin.Content != "" {
		t.Fatalf("invalid-utf8 blob should be binary: %+v", latin)
	}
	// A directory path to `blob` is 404.
	if st, _ := cpReq(t, http.MethodGet, base+"/blob/src", basic("alice", "pw"), ""); st != http.StatusNotFound {
		t.Fatalf("blob of a dir want 404, got %d", st)
	}
	// Over the inline cap → truncated, no content (200).
	old := maxInlineBytes
	maxInlineBytes = 4
	defer func() { maxInlineBytes = old }()
	st, body := cpReq(t, http.MethodGet, base+"/blob/README.md", basic("alice", "pw"), "")
	var trunc protocol.BlobResponse
	json.Unmarshal([]byte(body), &trunc)
	if st != http.StatusOK || !trunc.Truncated || trunc.Content != "" {
		t.Fatalf("over-cap blob want truncated: %d %+v", st, trunc)
	}
}

// TestBrowseRawIsInert is the load-bearing B5 test: repository bytes are served
// inert on the Hub origin — an .html is text/plain (never text/html), and binary
// downloads as an attachment; both carry nosniff.
func TestBrowseRawIsInert(t *testing.T) {
	base, _, _, _ := serveBrowse(t)

	htmlResp := rawGet(t, base+"/raw/page.html", basic("alice", "pw"))
	defer htmlResp.Body.Close()
	if ct := htmlResp.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("html served as %q, want text/plain — B5 VIOLATION", ct)
	}
	if htmlResp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("raw missing nosniff")
	}

	binResp := rawGet(t, base+"/raw/data.bin", basic("alice", "pw"))
	defer binResp.Body.Close()
	if ct := binResp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("binary served as %q, want octet-stream", ct)
	}
	if cd := binResp.Header.Get("Content-Disposition"); cd != "attachment" {
		t.Fatalf("binary Content-Disposition = %q, want attachment", cd)
	}
	if binResp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("raw binary missing nosniff")
	}
}

func TestBrowseRawSizeCeiling(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	old := maxRawBytes
	maxRawBytes = 4 // README.md ("readme two") is 10 bytes
	defer func() { maxRawBytes = old }()
	if st, _ := cpReq(t, http.MethodGet, base+"/raw/README.md", basic("alice", "pw"), ""); st != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-ceiling raw want 413, got %d", st)
	}
}

// TestBrowsePathSafety proves B4: traversal-looking paths reach no object outside
// the tree — they are just non-existent entry names.
func TestBrowsePathSafety(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	for _, p := range []string{
		"/blob/../../etc/passwd",
		"/blob//etc/passwd",   // leading slash → "etc/passwd", no such entry
		"/blob/README.md/x",   // non-directory mid-path
		"/tree/src/main.go/y", // file used as a path prefix
		"/blob/nope",
	} {
		if st, _ := cpReq(t, http.MethodGet, base+p, basic("alice", "pw"), ""); st != http.StatusNotFound {
			t.Fatalf("%s want 404, got %d", p, st)
		}
	}
}

// TestBrowseFileHistory proves the §5.4 path filter: only commits that changed the
// path, and identical to RFC-0021 without a path.
func TestBrowseFileHistory(t *testing.T) {
	base, c1, c2, c3 := serveBrowse(t)

	// README.md changed at c2 (v1→v2) and c1 (introduced) — not c3.
	_, body := cpReq(t, http.MethodGet, base+"/commits?path=README.md", basic("alice", "pw"), "")
	var readme protocol.CommitsResponse
	json.Unmarshal([]byte(body), &readme)
	if got := ids(readme.Commits); len(got) != 2 || got[0] != c2.String() || got[1] != c1.String() {
		t.Fatalf("README history = %v, want [c2 c1]", got)
	}

	// src/main.go changed at c3 (v1→v2) and c1 (introduced) — not c2.
	_, body = cpReq(t, http.MethodGet, base+"/commits?path=src/main.go", basic("alice", "pw"), "")
	var main protocol.CommitsResponse
	json.Unmarshal([]byte(body), &main)
	if got := ids(main.Commits); len(got) != 2 || got[0] != c3.String() || got[1] != c1.String() {
		t.Fatalf("main.go history = %v, want [c3 c1]", got)
	}

	// No path → full history (3 commits), unchanged from RFC-0021.
	_, body = cpReq(t, http.MethodGet, base+"/commits", basic("alice", "pw"), "")
	var all protocol.CommitsResponse
	json.Unmarshal([]byte(body), &all)
	if len(all.Commits) != 3 {
		t.Fatalf("full history = %d commits, want 3", len(all.Commits))
	}
}

// TestBrowseFileHistoryPaged proves the cursor rides the underlying walk under a
// path filter: paging README's history (limit 1) yields c2 then c1, no repeats.
func TestBrowseFileHistoryPaged(t *testing.T) {
	base, c1, c2, _ := serveBrowse(t)
	_, body := cpReq(t, http.MethodGet, base+"/commits?path=README.md&limit=1", basic("alice", "pw"), "")
	var p1 protocol.CommitsResponse
	json.Unmarshal([]byte(body), &p1)
	if len(p1.Commits) != 1 || p1.Commits[0].ID != c2.String() || p1.Next == "" {
		t.Fatalf("page1 = %+v", p1)
	}
	_, body = cpReq(t, http.MethodGet, base+"/commits?path=README.md&limit=1&before="+p1.Next, basic("alice", "pw"), "")
	var p2 protocol.CommitsResponse
	json.Unmarshal([]byte(body), &p2)
	if len(p2.Commits) != 1 || p2.Commits[0].ID != c1.String() {
		t.Fatalf("page2 = %+v, want [c1]", p2)
	}
}

// TestBrowseRequiresRead proves B1: no `read` → 403 from every browse endpoint.
func TestBrowseRequiresRead(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	for _, p := range []string{"/tree/", "/blob/README.md", "/raw/README.md"} {
		if st, _ := cpReq(t, http.MethodGet, base+p, basic("bob", "pw"), ""); st != http.StatusForbidden {
			t.Fatalf("bob %s want 403, got %d", p, st)
		}
		if st, _ := cpReq(t, http.MethodGet, base+p, "", ""); st != http.StatusForbidden {
			t.Fatalf("anonymous %s want 403, got %d", p, st)
		}
	}
}

// TestBrowseETag proves §9: tree/blob/raw carry a content-addressed ETag that
// re-validates to 304.
func TestBrowseETag(t *testing.T) {
	base, _, _, _ := serveBrowse(t)
	for _, p := range []string{"/tree/src", "/blob/README.md", "/raw/README.md"} {
		resp := rawGet(t, base+p, basic("alice", "pw"))
		etag := resp.Header.Get("ETag")
		resp.Body.Close()
		if etag == "" {
			t.Fatalf("%s: no ETag", p)
		}
		req, _ := http.NewRequest(http.MethodGet, base+p, nil)
		req.Header.Set("Authorization", basic("alice", "pw"))
		req.Header.Set("If-None-Match", etag)
		resp2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: If-None-Match want 304, got %d", p, resp2.StatusCode)
		}
	}
}

func ids(cs []protocol.CommitSummary) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func rawGet(t *testing.T, url, auth string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("%s = %d", url, resp.StatusCode)
	}
	return resp
}
