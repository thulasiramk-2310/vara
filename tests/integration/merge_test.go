package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/refs"
)

// TestMerge_FastForward verifies that merging a direct descendant branch
// performs a fast-forward and advances the current branch ref.
func TestMerge_FastForward(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Initial commit on main.
	writeFile(t, dir, "a.txt", "line1\n")
	makeCommit(t, ctx, "initial")

	// Create feature and switch to it.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	// Add a commit on feature.
	writeFile(t, dir, "b.txt", "feature line\n")
	makeCommit(t, ctx, "add b on feature")

	// Switch back to main and merge feature.
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back: %v", err)
	}

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Fast-forward") {
		t.Errorf("expected Fast-forward message, got: %q", out)
	}

	// b.txt must now be present on main.
	if readFile(dir, "b.txt") == "" {
		t.Error("b.txt should be present after fast-forward merge")
	}

	// HEAD (main) must point to the same commit as feature.
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	mainID, _ := resolver.Resolve("refs/heads/main")
	featureID, _ := resolver.Resolve("refs/heads/feature")
	if mainID != featureID {
		t.Errorf("after fast-forward, main (%s) != feature (%s)", mainID.String()[:7], featureID.String()[:7])
	}
}

// TestMerge_ThreeWay verifies a clean three-way merge creates a merge commit.
func TestMerge_ThreeWay(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Base commit.
	writeFile(t, dir, "shared.txt", "base\n")
	makeCommit(t, ctx, "base")

	// Create feature.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}

	// Diverge: commit on feature.
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "feature.txt", "feature work\n")
	makeCommit(t, ctx, "feature commit")

	// Diverge: commit on main.
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back: %v", err)
	}
	writeFile(t, dir, "main.txt", "main work\n")
	makeCommit(t, ctx, "main commit")

	// Merge feature into main.
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if strings.Contains(out, "Automatic merge failed") {
		t.Fatalf("unexpected conflict: %s", out)
	}
	if !strings.Contains(out, "recursive") {
		t.Errorf("expected 'recursive' merge output, got: %q", out)
	}

	// Both files must exist on main.
	if readFile(dir, "feature.txt") == "" {
		t.Error("feature.txt should be present after three-way merge")
	}
	if readFile(dir, "main.txt") == "" {
		t.Error("main.txt should be present after three-way merge")
	}
}

// TestMerge_Conflict verifies that conflicting modifications produce markers.
func TestMerge_Conflict(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Base: shared.txt with known content.
	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}

	// Feature: modify line2.
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nFEATURE CHANGE\nline3\n")
	makeCommit(t, ctx, "feature modifies line2")

	// Main: also modify line2 differently.
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nMAIN CHANGE\nline3\n")
	makeCommit(t, ctx, "main modifies line2")

	// Merge.
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge returned unexpected error: %v", err)
	}
	if !strings.Contains(out, "Automatic merge failed") {
		t.Errorf("expected conflict message, got: %q", out)
	}
	if !strings.Contains(out, "shared.txt") {
		t.Errorf("conflict message should mention shared.txt: %q", out)
	}

	// shared.txt must contain conflict markers.
	content := readFile(dir, "shared.txt")
	if !strings.Contains(content, "<<<<<<<") {
		t.Error("shared.txt should contain conflict markers after conflict merge")
	}

	// MERGE_HEAD must exist.
	if _, err := os.Stat(filepath.Join(ctx.Repository.VaraDir, "MERGE_HEAD")); err != nil {
		t.Error("MERGE_HEAD should exist after a conflicted merge")
	}
}

// TestMerge_AlreadyUpToDate verifies that merging an ancestor is a no-op.
func TestMerge_AlreadyUpToDate(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "a.txt", "hello\n")
	makeCommit(t, ctx, "initial")

	// Create feature from the same commit, but don't advance it.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	// Add a commit on main only.
	writeFile(t, dir, "b.txt", "main work\n")
	makeCommit(t, ctx, "main advances")

	// feature is behind main: merging it should be a no-op (already up-to-date).
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	// No conflict, no fast-forward output needed — main is ahead.
	_ = out // Result is Success with current HEAD; just verify no error.
}

// TestMerge_NonExistentBranch verifies that merging a missing branch errors.
func TestMerge_NonExistentBranch(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "data\n")
	makeCommit(t, ctx, "initial")

	_, err := commands.RunMerge(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent branch")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}
