package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func fileExists(rootDir, relPath string) bool {
	_, err := os.Stat(filepath.Join(rootDir, relPath))
	return err == nil
}

// makeModifyDeleteConflict sets up: base has doc.txt; the feature branch DELETES
// it; main MODIFIES it. Merging feature into main is a modify/delete conflict
// (ours modified, theirs deleted). Returns the post-merge context and repo dir.
func makeModifyDeleteConflict(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "doc.txt", "original\n")
	writeFile(t, dir, "keep.txt", "keep\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	// Delete via the real command, then commit — the user-reachable path.
	if _, err := commands.RunRm(ctx, []string{"doc.txt"}); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if _, err := commands.RunCommit(ctx, "delete doc on feature"); err != nil {
		t.Fatalf("commit deletion: %v", err)
	}

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "doc.txt", "MAIN edit\n")
	makeCommit(t, ctx, "modify doc on main")

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Automatic merge failed") {
		t.Fatalf("expected conflict, got %q", out)
	}
	// The sidecar must classify it as modify/delete with theirs absent.
	st, ok, _ := mergestate.Read(ctx.Repository.VaraDir)
	if !ok {
		t.Fatal("sidecar missing")
	}
	e, ok := st.Lookup("doc.txt")
	if !ok || e.Kind != mergestate.KindModifyDelete || e.Theirs.Present || !e.Ours.Present {
		t.Fatalf("bad classification: %+v (ok=%v)", e, ok)
	}
	return ctx, dir
}

// TestModifyDeleteTheirsHonorsDeletion is the core correctness fix: taking the
// side that deleted the file must DELETE it — not invent a zero-byte file — and
// the deletion must be baked into the merge commit's tree.
func TestModifyDeleteTheirsHonorsDeletion(t *testing.T) {
	ctx, dir := makeModifyDeleteConflict(t)

	out, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("resolve --theirs: %v", err)
	}
	if !strings.Contains(out, "modify/delete") {
		t.Fatalf("resolve output should mention modify/delete: %q", out)
	}

	// Working tree: the file must be gone, NOT an empty file.
	if fileExists(dir, "doc.txt") {
		t.Fatalf("doc.txt should be deleted, but it exists with %q", readFile(dir, "doc.txt"))
	}

	ctx = reloadCtx(t, ctx)
	mergeCommit, err := commands.RunCommit(ctx, "merge feature")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The deletion must be in history: the merge commit's tree lacks doc.txt.
	paths := treePaths(t, ctx, mergeCommit)
	if paths["doc.txt"] {
		t.Fatal("doc.txt must be absent from the merge commit tree (deletion lost)")
	}
	if !paths["keep.txt"] {
		t.Fatal("keep.txt should still be present")
	}
}

// TestModifyDeleteOursKeepsOurVersion: taking our side keeps our modified file.
func TestModifyDeleteOursKeepsOurVersion(t *testing.T) {
	ctx, dir := makeModifyDeleteConflict(t)

	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Ours}); err != nil {
		t.Fatalf("resolve --ours: %v", err)
	}
	if got := readFile(dir, "doc.txt"); got != "MAIN edit\n" {
		t.Fatalf("doc.txt should keep our version, got %q", got)
	}

	ctx = reloadCtx(t, ctx)
	mergeCommit, err := commands.RunCommit(ctx, "merge feature")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !treePaths(t, ctx, mergeCommit)["doc.txt"] {
		t.Fatal("doc.txt should be present in the merge commit tree")
	}
}

// TestModifyDeleteHandResolvedByRm proves the user-reachable hand-resolution:
// `vara rm` the conflicted file records the deletion as the resolution (marks the
// sidecar entry resolved), and commit then completes the merge with the file gone.
func TestModifyDeleteHandResolvedByRm(t *testing.T) {
	ctx, dir := makeModifyDeleteConflict(t)

	out, err := commands.RunRm(ctx, []string{"doc.txt"})
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if !strings.Contains(out, "doc.txt") {
		t.Fatalf("rm output: %q", out)
	}
	if fileExists(dir, "doc.txt") {
		t.Fatal("doc.txt should be removed from the working tree")
	}
	// The sidecar entry must now be resolved.
	st, _, _ := mergestate.Read(ctx.Repository.VaraDir)
	if e, _ := st.Lookup("doc.txt"); !e.Resolved {
		t.Fatal("`vara rm` of a conflicted file should mark it resolved")
	}

	ctx = reloadCtx(t, ctx)
	mergeCommit, err := commands.RunCommit(ctx, "merge feature (deleted doc)")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if treePaths(t, ctx, mergeCommit)["doc.txt"] {
		t.Fatal("doc.txt should be absent from the merge commit tree")
	}
}

// TestModifyDeleteRefusedByAutoAndUnion: auto/union cannot settle modify/delete;
// they must leave it unresolved, and commit must stay refused.
func TestModifyDeleteRefusedByAutoAndUnion(t *testing.T) {
	for _, strat := range []conflict.Strategy{conflict.Auto, conflict.Union} {
		t.Run(string(strat), func(t *testing.T) {
			ctx, dir := makeModifyDeleteConflict(t)

			out, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: strat})
			if err != nil {
				t.Fatalf("resolve %s: %v", strat, err)
			}
			if !strings.Contains(out, "need an explicit choice") {
				t.Fatalf("%s should refuse modify/delete, got: %q", strat, out)
			}
			// File untouched (still our modified version), nothing invented.
			if got := readFile(dir, "doc.txt"); got != "MAIN edit\n" {
				t.Fatalf("file should be untouched, got %q", got)
			}

			// Commit must remain refused: no markers exist, so this proves the
			// refusal comes from the sidecar's Resolved flag, not a marker scan.
			ctx = reloadCtx(t, ctx)
			if _, err := commands.RunCommit(ctx, "premature"); err == nil {
				t.Fatal("commit must be refused while modify/delete is unresolved")
			} else if !strings.Contains(err.Error(), "unresolved conflicts") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestModifyDeleteCommitRefusedWithoutMarkers is the sharp regression: a
// modify/delete conflict leaves NO markers in the tree, yet commit must still
// refuse until it is resolved (the old marker-scan model let it through).
func TestModifyDeleteCommitRefusedWithoutMarkers(t *testing.T) {
	ctx, dir := makeModifyDeleteConflict(t)

	if strings.Contains(readFile(dir, "doc.txt"), "<<<<<<<") {
		t.Fatal("precondition: modify/delete should have no markers")
	}
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "sneak it in"); err == nil {
		t.Fatal("commit must be refused for an unresolved modify/delete with no markers")
	}
}

// treePaths returns the set of file paths in a commit's tree.
func treePaths(t *testing.T, ctx *commands.Context, commitID types.CommitID) map[string]bool {
	t.Helper()
	store := object.NewStore(ctx.Repository.VaraDir)
	obj, err := store.Read(types.ObjectID(commitID))
	if err != nil {
		t.Fatalf("read commit: %v", err)
	}
	c := obj.(*object.Commit)
	out := map[string]bool{}
	walkTree(t, store, types.ObjectID(c.TreeHash), "", out)
	return out
}

func walkTree(t *testing.T, store *object.Store, id types.ObjectID, prefix string, out map[string]bool) {
	t.Helper()
	obj, err := store.Read(id)
	if err != nil {
		t.Fatalf("read tree: %v", err)
	}
	tree := obj.(*object.Tree)
	for _, e := range tree.Entries {
		rel := prefix + e.Name
		if e.Mode == 0o040000 {
			walkTree(t, store, e.Hash, rel+"/", out)
			continue
		}
		out[rel] = true
	}
}
