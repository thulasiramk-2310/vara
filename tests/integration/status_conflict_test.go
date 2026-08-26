package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
)

// TestStatusSurfacesContentConflict: during a conflicted merge, `vara status`
// reports the path as unmerged ("both modified") and shows the merge banner,
// sourced from the sidecar rather than a marker scan.
func TestStatusSurfacesContentConflict(t *testing.T) {
	ctx, _ := makeConflict(t) // edit/edit on shared.txt

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "You have unmerged paths.") {
		t.Fatalf("expected merge banner, got:\n%s", out)
	}
	if !strings.Contains(out, "both modified:") || !strings.Contains(out, "shared.txt") {
		t.Fatalf("expected shared.txt as both-modified, got:\n%s", out)
	}
	// It must NOT also appear as an ordinary "modified" change (no double-listing).
	if strings.Contains(out, "Changes not staged for commit") {
		t.Fatalf("conflicted path must not be listed as an ordinary modification:\n%s", out)
	}
}

// TestStatusSurfacesModifyDelete: a marker-less modify/delete conflict — which the
// working-tree scanner cannot detect — must still surface, classified by side.
func TestStatusSurfacesModifyDelete(t *testing.T) {
	ctx, _ := makeModifyDeleteConflict(t) // ours modified doc.txt, theirs deleted it

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "You have unmerged paths.") {
		t.Fatalf("expected merge banner, got:\n%s", out)
	}
	// Ours present, theirs absent → "deleted by them".
	if !strings.Contains(out, "deleted by them:") || !strings.Contains(out, "doc.txt") {
		t.Fatalf("expected doc.txt as deleted-by-them, got:\n%s", out)
	}
}

// TestStatusShowsResolvedThenClean: after resolving, the path moves to the
// "resolved / staged for the merge commit" section and the banner flips to the
// all-fixed-still-merging state.
func TestStatusShowsResolvedThenClean(t *testing.T) {
	ctx, _ := makeConflict(t)

	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "All conflicts fixed but you are still merging.") {
		t.Fatalf("expected all-fixed banner, got:\n%s", out)
	}
	if !strings.Contains(out, "Resolved (staged for the merge commit):") || !strings.Contains(out, "resolved:") {
		t.Fatalf("expected resolved section, got:\n%s", out)
	}
	if strings.Contains(out, "Unmerged paths:") {
		t.Fatalf("no path should remain unmerged after resolving:\n%s", out)
	}
}

// TestStatusNoMergeIsUnaffected: with no merge in progress, status shows the
// ordinary working-tree view and none of the merge sections.
func TestStatusNoMergeIsUnaffected(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	makeCommit(t, ctx, "base")
	writeFile(t, dir, "a.txt", "changed\n")

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(out, "unmerged") || strings.Contains(out, "still merging") {
		t.Fatalf("no merge sections expected without a merge:\n%s", out)
	}
	if !strings.Contains(out, "modified:") || !strings.Contains(out, "a.txt") {
		t.Fatalf("expected ordinary modified a.txt, got:\n%s", out)
	}
}
