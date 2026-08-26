package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// TestMergeContinueConcludesResolvedMerge: after resolving, `vara merge --continue`
// seals a two-parent merge commit and clears the merge-in-progress state.
func TestMergeContinueConcludesResolvedMerge(t *testing.T) {
	ctx, _ := makeConflict(t)

	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunMergeContinue(ctx, "")
	if err != nil {
		t.Fatalf("merge --continue: %v", err)
	}
	if !strings.Contains(out, "Merge branch 'feature'") {
		t.Fatalf("expected default merge message, got: %q", out)
	}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("merge should be concluded (MERGE_HEAD cleared)")
	}
}

// TestMergeContinueRefusesUnresolved: continue must refuse while a conflict is
// unresolved, naming the file, and leave the merge in progress.
func TestMergeContinueRefusesUnresolved(t *testing.T) {
	ctx, _ := makeConflict(t)

	ctx = reloadCtx(t, ctx)
	_, err := commands.RunMergeContinue(ctx, "")
	if err == nil {
		t.Fatal("continue must refuse while a conflict is unresolved")
	}
	if !strings.Contains(err.Error(), "shared.txt") {
		t.Fatalf("error must name the unresolved file, got: %v", err)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("refused continue must leave the merge in progress")
	}
}

// TestMergeContinueWithNoMergeErrors: continue with nothing in progress errors.
func TestMergeContinueWithNoMergeErrors(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunMergeContinue(ctx, ""); err == nil {
		t.Fatal("continue with no merge should error")
	} else if !strings.Contains(err.Error(), "no merge in progress") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMergeContinueCustomMessage: an explicit message overrides the default.
func TestMergeContinueCustomMessage(t *testing.T) {
	ctx, _ := makeConflict(t)
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Ours}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunMergeContinue(ctx, "my merge message")
	if err != nil {
		t.Fatalf("merge --continue: %v", err)
	}
	if !strings.Contains(out, "my merge message") {
		t.Fatalf("expected custom message, got: %q", out)
	}
}
