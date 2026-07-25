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

// buildDiffRepo creates a two-commit history exercising every change status:
//
//	c1 (root): README.md=A, src/main.go, bin.dat=A, gone.txt
//	c2 (c1):   README.md=B (modified), bin.dat=B (modified, binary), notes.txt (added),
//	           gone.txt removed (deleted), src/main.go unchanged
//
// HEAD=main→c2.
func buildDiffRepo(t *testing.T, repoRoot string) (c1, c2 types.CommitID) {
	t.Helper()
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatal(err)
	}
	s := storeOf(repoRoot)

	readmeA := writeObj(t, s, object.NewBlob([]byte("alpha\nbravo\ncharlie\n")))
	readmeB := writeObj(t, s, object.NewBlob([]byte("alpha\nbravo TWO\ncharlie\ndelta\n")))
	mainGo := writeObj(t, s, object.NewBlob([]byte("package main\n")))
	binA := writeObj(t, s, object.NewBlob([]byte{0, 'A', 0}))
	binB := writeObj(t, s, object.NewBlob([]byte{0, 'B', 0}))
	gone := writeObj(t, s, object.NewBlob([]byte("bye\n")))
	note := writeObj(t, s, object.NewBlob([]byte("note\n")))

	srcTree := writeObj(t, s, object.NewTree([]object.TreeEntry{{Mode: 0o100644, Hash: mainGo, Name: "main.go"}}))

	root1 := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readmeA, Name: "README.md"},
		{Mode: 0o100644, Hash: binA, Name: "bin.dat"},
		{Mode: 0o100644, Hash: gone, Name: "gone.txt"},
		{Mode: 0o040000, Hash: srcTree, Name: "src"},
	}))
	root2 := writeObj(t, s, object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: readmeB, Name: "README.md"},
		{Mode: 0o100644, Hash: binB, Name: "bin.dat"},
		{Mode: 0o100644, Hash: note, Name: "notes.txt"},
		{Mode: 0o040000, Hash: srcTree, Name: "src"},
	}))

	c1 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root1), Author: "T", Message: "c1"}))
	c2 = types.CommitID(writeObj(t, s, &object.Commit{TreeHash: types.TreeID(root2), Parents: []types.CommitID{c1}, Author: "T", Message: "c2"}))

	r := refsOf(repoRoot)
	r.Update("refs/heads/main", c2)
	r.Symbolic("HEAD", "refs/heads/main")
	return
}

func serveDiff(t *testing.T) (base string, c1, c2 types.CommitID) {
	t.Helper()
	root := t.TempDir()
	c1, c2 = buildDiffRepo(t, filepath.Join(root, "demo"))
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
	return ts.URL + "/_vara/repositories/demo", c1, c2
}

func statusByPath(files []protocol.DiffFileInfo) map[string]string {
	m := map[string]string{}
	for _, f := range files {
		m[f.Path] = f.Status
	}
	return m
}

func TestDiffSummary(t *testing.T) {
	base, c1, c2 := serveDiff(t)
	// base omitted → defaults to c2's first parent (c1).
	_, body := cpReq(t, http.MethodGet, base+"/diff?head="+c2.String(), basic("alice", "pw"), "")
	var sum protocol.DiffSummaryResponse
	json.Unmarshal([]byte(body), &sum)
	got := statusByPath(sum.Files)
	want := map[string]string{"README.md": "modified", "bin.dat": "modified", "gone.txt": "deleted", "notes.txt": "added"}
	if len(got) != len(want) {
		t.Fatalf("summary files = %+v, want %v", sum.Files, want)
	}
	for p, st := range want {
		if got[p] != st {
			t.Fatalf("%s status = %q, want %q", p, got[p], st)
		}
	}
	if _, ok := got["src/main.go"]; ok {
		t.Fatal("unchanged src/main.go should not appear in the summary")
	}
	if sum.BaseCommit != c1.String() || sum.HeadCommit != c2.String() {
		t.Fatalf("echoed commits wrong: base=%s head=%s", sum.BaseCommit, sum.HeadCommit)
	}
	// Explicit base == head → empty change set.
	_, body = cpReq(t, http.MethodGet, base+"/diff?head="+c2.String()+"&base="+c2.String(), basic("alice", "pw"), "")
	var empty protocol.DiffSummaryResponse
	json.Unmarshal([]byte(body), &empty)
	if len(empty.Files) != 0 {
		t.Fatalf("base==head should be empty, got %+v", empty.Files)
	}
}

func TestDiffCommitConvenience(t *testing.T) {
	base, c1, c2 := serveDiff(t)
	// /commits/{c2}/diff == /diff?head=c2.
	_, a := cpReq(t, http.MethodGet, base+"/commits/"+c2.String()+"/diff", basic("alice", "pw"), "")
	_, b := cpReq(t, http.MethodGet, base+"/diff?head="+c2.String(), basic("alice", "pw"), "")
	var da, db protocol.DiffSummaryResponse
	json.Unmarshal([]byte(a), &da)
	json.Unmarshal([]byte(b), &db)
	if len(da.Files) != len(db.Files) || da.BaseCommit != db.BaseCommit {
		t.Fatalf("commit-diff != /diff?head: %+v vs %+v", da, db)
	}
	// Root commit c1 → base is the empty tree, so every file is "added".
	_, body := cpReq(t, http.MethodGet, base+"/commits/"+c1.String()+"/diff", basic("alice", "pw"), "")
	var root protocol.DiffSummaryResponse
	json.Unmarshal([]byte(body), &root)
	if root.BaseCommit != "" {
		t.Fatalf("root base_commit should be empty, got %q", root.BaseCommit)
	}
	for _, f := range root.Files {
		if f.Status != "added" {
			t.Fatalf("root commit file %s = %q, want added", f.Path, f.Status)
		}
	}
	if len(root.Files) != 4 { // README.md, bin.dat, gone.txt, src/main.go
		t.Fatalf("root diff = %d files, want 4", len(root.Files))
	}
}

func TestDiffFileText(t *testing.T) {
	base, _, c2 := serveDiff(t)
	_, body := cpReq(t, http.MethodGet, base+"/diff/README.md?head="+c2.String(), basic("alice", "pw"), "")
	var fd protocol.FileDiffResponse
	json.Unmarshal([]byte(body), &fd)
	if fd.Binary || fd.Truncated || fd.Status != "modified" {
		t.Fatalf("README diff meta wrong: %+v", fd)
	}
	// bravo → bravo TWO (1 del + 1 add), delta added (1 add) = 2 adds, 1 del.
	if fd.Additions != 2 || fd.Deletions != 1 {
		t.Fatalf("counts = +%d/-%d, want +2/-1", fd.Additions, fd.Deletions)
	}
	var sawDel, sawAdd, sawDelta bool
	for _, hk := range fd.Hunks {
		for _, ln := range hk.Lines {
			if ln.Type == "del" && ln.Content == "bravo" {
				sawDel = true
			}
			if ln.Type == "add" && ln.Content == "bravo TWO" {
				sawAdd = true
			}
			if ln.Type == "add" && ln.Content == "delta" {
				sawDelta = true
			}
		}
	}
	if !sawDel || !sawAdd || !sawDelta {
		t.Fatalf("hunk lines missing expected changes: del=%v add=%v delta=%v", sawDel, sawAdd, sawDelta)
	}
}

func TestDiffFileAddedDeleted(t *testing.T) {
	base, _, c2 := serveDiff(t)
	// Added file: all lines add.
	_, body := cpReq(t, http.MethodGet, base+"/diff/notes.txt?head="+c2.String(), basic("alice", "pw"), "")
	var added protocol.FileDiffResponse
	json.Unmarshal([]byte(body), &added)
	if added.Status != "added" || added.Additions != 1 || added.Deletions != 0 {
		t.Fatalf("added file wrong: %+v", added)
	}
	// Deleted file: all lines del.
	_, body = cpReq(t, http.MethodGet, base+"/diff/gone.txt?head="+c2.String(), basic("alice", "pw"), "")
	var deleted protocol.FileDiffResponse
	json.Unmarshal([]byte(body), &deleted)
	if deleted.Status != "deleted" || deleted.Deletions != 1 || deleted.Additions != 0 {
		t.Fatalf("deleted file wrong: %+v", deleted)
	}
}

func TestDiffFileBinary(t *testing.T) {
	base, _, c2 := serveDiff(t)
	_, body := cpReq(t, http.MethodGet, base+"/diff/bin.dat?head="+c2.String(), basic("alice", "pw"), "")
	var fd protocol.FileDiffResponse
	json.Unmarshal([]byte(body), &fd)
	if !fd.Binary || len(fd.Hunks) != 0 {
		t.Fatalf("binary diff should have Binary set and no hunks: %+v", fd)
	}
}

func TestDiffFileNotInChangeSet(t *testing.T) {
	base, _, c2 := serveDiff(t)
	// Unchanged path → 404.
	if st, _ := cpReq(t, http.MethodGet, base+"/diff/src/main.go?head="+c2.String(), basic("alice", "pw"), ""); st != http.StatusNotFound {
		t.Fatalf("unchanged path want 404, got %d", st)
	}
	// Absent path → 404.
	if st, _ := cpReq(t, http.MethodGet, base+"/diff/nope?head="+c2.String(), basic("alice", "pw"), ""); st != http.StatusNotFound {
		t.Fatalf("absent path want 404, got %d", st)
	}
}

// TestDiffBounds proves D5: a hard-ceiling file diff is 413; a soft-cap one is
// truncated (not binary).
func TestDiffBounds(t *testing.T) {
	base, _, c2 := serveDiff(t)
	// Hard ceiling: README.md ("alpha\n..." both sides) exceeds a tiny hard cap → 413.
	oldHard := hub.SetMaxDiffInputHard(4)
	if st, _ := cpReq(t, http.MethodGet, base+"/diff/README.md?head="+c2.String(), basic("alice", "pw"), ""); st != http.StatusRequestEntityTooLarge {
		t.Fatalf("over hard ceiling want 413, got %d", st)
	}
	hub.SetMaxDiffInputHard(oldHard)
	// Soft cap: truncated with empty hunks, still 200.
	oldSoft := hub.SetMaxDiffInputSoft(4)
	st, body := cpReq(t, http.MethodGet, base+"/diff/README.md?head="+c2.String(), basic("alice", "pw"), "")
	hub.SetMaxDiffInputSoft(oldSoft)
	var fd protocol.FileDiffResponse
	json.Unmarshal([]byte(body), &fd)
	if st != http.StatusOK || !fd.Truncated || fd.Binary || len(fd.Hunks) != 0 {
		t.Fatalf("over soft cap want truncated 200 no hunks: %d %+v", st, fd)
	}
}

// TestDiffRequiresRead proves D1.
func TestDiffRequiresRead(t *testing.T) {
	base, _, c2 := serveDiff(t)
	for _, p := range []string{"/diff?head=" + c2.String(), "/diff/README.md?head=" + c2.String(), "/commits/" + c2.String() + "/diff"} {
		if st, _ := cpReq(t, http.MethodGet, base+p, basic("bob", "pw"), ""); st != http.StatusForbidden {
			t.Fatalf("bob %s want 403, got %d", p, st)
		}
		if st, _ := cpReq(t, http.MethodGet, base+p, "", ""); st != http.StatusForbidden {
			t.Fatalf("anonymous %s want 403, got %d", p, st)
		}
	}
}

func TestDiffHeadRequired(t *testing.T) {
	base, _, _ := serveDiff(t)
	if st, _ := cpReq(t, http.MethodGet, base+"/diff", basic("alice", "pw"), ""); st != http.StatusBadRequest {
		t.Fatalf("missing head want 400, got %d", st)
	}
	if st, _ := cpReq(t, http.MethodGet, base+"/diff/README.md", basic("alice", "pw"), ""); st != http.StatusBadRequest {
		t.Fatalf("file diff missing head want 400, got %d", st)
	}
}

func TestDiffETag(t *testing.T) {
	base, _, c2 := serveDiff(t)
	for _, p := range []string{"/diff?head=" + c2.String(), "/diff/README.md?head=" + c2.String()} {
		req, _ := http.NewRequest(http.MethodGet, base+p, nil)
		req.Header.Set("Authorization", basic("alice", "pw"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		etag := resp.Header.Get("ETag")
		resp.Body.Close()
		if etag == "" {
			t.Fatalf("%s: no ETag", p)
		}
		req2, _ := http.NewRequest(http.MethodGet, base+p, nil)
		req2.Header.Set("Authorization", basic("alice", "pw"))
		req2.Header.Set("If-None-Match", etag)
		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatal(err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: If-None-Match want 304, got %d", p, resp2.StatusCode)
		}
	}
}
