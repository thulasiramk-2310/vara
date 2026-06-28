package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/refs"
)

// setupRepo initialises a VARA repository in a temp dir and returns a
// fully wired Context with an empty index. Use makeCommit to add commits.
func setupRepo(t *testing.T) (*commands.Context, string) {
	t.Helper()
	tmpDir := t.TempDir()
	repo, err := repository.Init(tmpDir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ctx := &commands.Context{
		Repository: repo,
		Index:      index.New(),
	}
	return ctx, tmpDir
}

// makeCommit stages all files in the working directory and creates a commit.
func makeCommit(t *testing.T, ctx *commands.Context, message string) {
	t.Helper()
	if err := commands.RunAdd(ctx, []string{"."}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := commands.RunCommit(ctx, message); err != nil {
		t.Fatalf("commit %q: %v", message, err)
	}
}

// writeFile writes content to a relative path inside the repo root.
func writeFile(t *testing.T, rootDir, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(rootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// readFile reads the content of a relative path inside the repo root.
// Returns "" if the file does not exist.
func readFile(rootDir, relPath string) string {
	data, err := os.ReadFile(filepath.Join(rootDir, relPath))
	if err != nil {
		return ""
	}
	return string(data)
}

// TestSwitch_BasicBranchChange verifies that switching moves HEAD and
// checks out the correct files.
func TestSwitch_BasicBranchChange(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Commit on main: file_a.txt.
	writeFile(t, dir, "file_a.txt", "hello from main")
	makeCommit(t, ctx, "add file_a")

	// Create feature branch.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	// Switch to feature.
	out, err := commands.RunSwitch(ctx, "feature")
	if err != nil {
		t.Fatalf("switch to feature: %v", err)
	}
	if !strings.Contains(out, "Switched to branch 'feature'") {
		t.Errorf("unexpected output: %q", out)
	}

	// HEAD must now point to refs/heads/feature.
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	target, err := resolver.ResolveSymbolic("HEAD")
	if err != nil {
		t.Fatalf("resolve HEAD: %v", err)
	}
	if target != "refs/heads/feature" {
		t.Errorf("HEAD target = %q, want refs/heads/feature", target)
	}

	// file_a.txt must still be present (same commit as main).
	if got := readFile(dir, "file_a.txt"); got != "hello from main" {
		t.Errorf("file_a.txt = %q, want %q", got, "hello from main")
	}
}

// TestSwitch_FilesChangeBetweenBranches verifies that files unique to one
// branch appear/disappear when switching.
func TestSwitch_FilesChangeBetweenBranches(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Commit on main: file_a.txt.
	writeFile(t, dir, "file_a.txt", "main content")
	makeCommit(t, ctx, "initial")

	// Create and switch to feature.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch to feature: %v", err)
	}

	// On feature: add file_b.txt and commit.
	writeFile(t, dir, "file_b.txt", "feature content")
	makeCommit(t, ctx, "add file_b on feature")

	// Verify file_b.txt exists.
	if readFile(dir, "file_b.txt") == "" {
		t.Error("file_b.txt should exist on feature branch")
	}

	// Switch back to main.
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back to main: %v", err)
	}

	// file_b.txt must be gone; file_a.txt must be present.
	if readFile(dir, "file_b.txt") != "" {
		t.Error("file_b.txt should not exist on main branch")
	}
	if got := readFile(dir, "file_a.txt"); got != "main content" {
		t.Errorf("file_a.txt = %q, want %q", got, "main content")
	}

	// HEAD must point to main.
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	target, _ := resolver.ResolveSymbolic("HEAD")
	if target != "refs/heads/main" {
		t.Errorf("HEAD = %q, want refs/heads/main", target)
	}
}

// TestSwitch_AlreadyOnBranch verifies that switching to the current branch
// is a no-op and returns the "Already on" message.
func TestSwitch_AlreadyOnBranch(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "data")
	makeCommit(t, ctx, "initial")

	out, err := commands.RunSwitch(ctx, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Already on 'main'") {
		t.Errorf("expected 'Already on' message, got: %q", out)
	}
}

// TestSwitch_NonExistentBranch verifies that switching to a missing branch
// returns a descriptive error.
func TestSwitch_NonExistentBranch(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "data")
	makeCommit(t, ctx, "initial")

	_, err := commands.RunSwitch(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message should mention 'not found': %v", err)
	}
}

// TestSwitch_InvalidBranchName verifies that RFC-0004 §3 validation fires.
func TestSwitch_InvalidBranchName(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "data")
	makeCommit(t, ctx, "initial")

	invalidNames := []string{
		"has space",
		"has~tilde",
		"has^caret",
		"has:colon",
		".leading-dot",
		"trailing-dot.",
		"double..dot",
	}

	for _, name := range invalidNames {
		_, err := commands.RunSwitch(ctx, name)
		if err == nil {
			t.Errorf("expected validation error for %q, got nil", name)
		}
	}
}

// TestSwitch_SnapshotCreated verifies that a snapshot is created in
// .vara/snapshots/ before the switch.
func TestSwitch_SnapshotCreated(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "data")
	makeCommit(t, ctx, "initial")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	snapshotsDir := filepath.Join(ctx.Repository.VaraDir, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		t.Fatalf("read snapshots dir: %v", err)
	}
	snapCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.zst") {
			snapCount++
		}
	}
	if snapCount == 0 {
		t.Error("expected at least one snapshot to be created before switch")
	}
}

// TestSwitch_ReflogUpdated verifies that HEAD's reflog records the switch.
func TestSwitch_ReflogUpdated(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "data")
	makeCommit(t, ctx, "initial")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(ctx.Repository.VaraDir, "logs", "HEAD"))
	if err != nil {
		t.Fatalf("read HEAD reflog: %v", err)
	}
	if !strings.Contains(string(logData), "switch: moving from main to feature") {
		t.Errorf("reflog missing switch entry:\n%s", string(logData))
	}
}

// TestSwitch_IndexUpdated verifies that the on-disk index file reflects
// the new branch's tracked files after switching.
func TestSwitch_IndexUpdated(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "a.txt", "A")
	makeCommit(t, ctx, "initial on main")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch to feature: %v", err)
	}

	writeFile(t, dir, "b.txt", "B")
	makeCommit(t, ctx, "add b on feature")

	// Switch back to main.
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back to main: %v", err)
	}

	// Reload index from disk to confirm it was persisted correctly.
	idxBytes, err := os.ReadFile(filepath.Join(ctx.Repository.VaraDir, "index"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	idx, err := index.Deserialize(idxBytes)
	if err != nil {
		t.Fatalf("deserialize index: %v", err)
	}

	for _, e := range idx.Entries {
		if e.Path == "b.txt" {
			t.Error("index should not contain b.txt after switching back to main")
		}
	}

	found := false
	for _, e := range idx.Entries {
		if e.Path == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Error("index should contain a.txt on main branch")
	}
}
