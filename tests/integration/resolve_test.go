package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// makeConflict builds the standard two-sided conflict on shared.txt and returns
// the context after RunMerge has left markers + MERGE_HEAD in place.
func makeConflict(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nFEATURE\nline3\n")
	makeCommit(t, ctx, "feature change")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nMAIN\nline3\n")
	makeCommit(t, ctx, "main change")

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Automatic merge failed") {
		t.Fatalf("expected conflict, got: %q", out)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("MERGE_HEAD should exist after conflict")
	}
	return ctx, dir
}

// TestResolveTheirsThenCommitCompletesMerge is the headline flow: resolve with a
// strategy, commit, and confirm a real two-parent merge commit is produced and
// MERGE_HEAD is cleared.
func TestResolveTheirsThenCommitCompletesMerge(t *testing.T) {
	ctx, dir := makeConflict(t)

	out, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.Contains(out, "Resolved 1 file") {
		t.Fatalf("resolve output: %q", out)
	}

	content := readFile(dir, "shared.txt")
	if strings.Contains(content, "<<<<<<<") {
		t.Fatalf("markers remain after resolve: %q", content)
	}
	if content != "line1\nFEATURE\nline3\n" {
		t.Fatalf("theirs content = %q", content)
	}

	// Reload the index from disk (resolve wrote it) before committing.
	ctx = reloadCtx(t, ctx)
	mergeCommit, err := commands.RunCommit(ctx, "merge feature")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("MERGE_HEAD should be cleared after committing the merge")
	}

	// The merge commit must have TWO parents.
	store := object.NewStore(ctx.Repository.VaraDir)
	obj, err := store.Read(types.ObjectID(mergeCommit))
	if err != nil {
		t.Fatalf("read merge commit: %v", err)
	}
	c := obj.(*object.Commit)
	if len(c.Parents) != 2 {
		t.Fatalf("merge commit has %d parents, want 2", len(c.Parents))
	}
}

// TestCommitRefusedWhileConflictsUnresolved proves a commit is blocked while a
// tracked file still contains conflict markers.
func TestCommitRefusedWhileConflictsUnresolved(t *testing.T) {
	ctx, _ := makeConflict(t)
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "premature"); err == nil {
		t.Fatal("commit should be refused while conflicts are unresolved")
	} else if !strings.Contains(err.Error(), "unresolved conflicts") {
		t.Fatalf("error should mention unresolved conflicts, got: %v", err)
	}
}

// TestResolveUnionKeepsBothSides checks the union strategy.
func TestResolveUnionKeepsBothSides(t *testing.T) {
	ctx, dir := makeConflict(t)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Union}); err != nil {
		t.Fatalf("resolve union: %v", err)
	}
	content := readFile(dir, "shared.txt")
	if content != "line1\nMAIN\nFEATURE\nline3\n" {
		t.Fatalf("union content = %q", content)
	}
}

// TestResolveListChangesNothing proves --list is read-only.
func TestResolveListChangesNothing(t *testing.T) {
	ctx, dir := makeConflict(t)
	before := readFile(dir, "shared.txt")
	out, err := commands.RunResolve(ctx, commands.ResolveArgs{List: true})
	if err != nil {
		t.Fatalf("resolve --list: %v", err)
	}
	if !strings.Contains(out, "shared.txt") {
		t.Fatalf("list should mention shared.txt: %q", out)
	}
	if readFile(dir, "shared.txt") != before {
		t.Fatal("--list must not modify the file")
	}
}

// reloadCtx reuses the repository but reloads the on-disk index, so a follow-up
// command sees what the previous one persisted.
func reloadCtx(t *testing.T, ctx *commands.Context) *commands.Context {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ctx.Repository.VaraDir, "index"))
	var i *index.Index
	switch {
	case err != nil || len(data) == 0:
		i = index.New()
	default:
		i, err = index.Deserialize(data)
		if err != nil {
			t.Fatalf("reload index: %v", err)
		}
	}
	return &commands.Context{Repository: ctx.Repository, Index: i}
}
