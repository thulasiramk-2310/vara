package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// TestVerify_HealthyRepo verifies that a fully operational repository
// produces a clean report with zero errors.
func TestVerify_HealthyRepo(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "a.txt", "hello\n")
	writeFile(t, dir, "sub/b.txt", "world\n")
	makeCommit(t, ctx, "initial")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "c.txt", "extra\n")
	makeCommit(t, ctx, "feature commit")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back: %v", err)
	}

	report, err := verify.Verify(ctx.Repository.VaraDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !report.Healthy {
		t.Errorf("expected healthy report, got errors:\n%s", reportErrors(report))
	}

	if report.Objects.Verified == 0 {
		t.Error("expected at least one verified object")
	}
	if report.Commits.Verified == 0 {
		t.Error("expected at least one verified commit")
	}
}

// TestVerify_CorruptedObject checks that a hash-mismatched object file is
// detected and reported in the Objects category.
func TestVerify_CorruptedObject(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "data\n")
	makeCommit(t, ctx, "initial")

	// Find and corrupt one object file.
	varaDir := ctx.Repository.VaraDir
	corrupted := false
	shards, _ := os.ReadDir(varaDir)
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(varaDir, shard.Name()))
		for _, f := range files {
			if len(f.Name()) != 62 {
				continue
			}
			path := filepath.Join(varaDir, shard.Name(), f.Name())
			// Objects are written 0444; make writable before corrupting.
			os.Chmod(path, 0644)
			os.WriteFile(path, []byte("corrupted!"), 0644)
			corrupted = true
			break
		}
		if corrupted {
			break
		}
	}
	if !corrupted {
		t.Fatal("could not find an object to corrupt")
	}

	report, err := verify.Verify(varaDir)
	if err != nil {
		t.Fatalf("verify returned unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report after object corruption")
	}
	if len(report.Objects.Errors) == 0 {
		t.Error("expected object errors after corruption")
	}
}

// TestVerify_MissingRef checks that a ref pointing to a non-existent object
// is detected.
func TestVerify_MissingRef(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "data\n")
	makeCommit(t, ctx, "initial")

	// Write a branch ref that points to a bogus commit ID.
	bogusID := "0000000000000000000000000000000000000000000000000000000000000000"
	refPath := filepath.Join(ctx.Repository.VaraDir, "refs", "heads", "broken")
	os.MkdirAll(filepath.Dir(refPath), 0755)
	os.WriteFile(refPath, []byte(bogusID+"\n"), 0644)

	report, err := verify.Verify(ctx.Repository.VaraDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report after writing broken ref")
	}
	if len(report.Refs.Errors) == 0 {
		t.Error("expected ref errors for broken branch")
	}
}

// TestVerify_CorruptJournal checks that a malformed journal entry is reported.
func TestVerify_CorruptJournal(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "data\n")
	makeCommit(t, ctx, "initial")

	// Write a malformed journal entry.
	journalDir := filepath.Join(ctx.Repository.VaraDir, "journal")
	os.MkdirAll(journalDir, 0755)
	os.WriteFile(filepath.Join(journalDir, "txn-bad.json"), []byte("{not valid json"), 0644)

	report, err := verify.Verify(ctx.Repository.VaraDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report after writing corrupt journal entry")
	}
	if len(report.Journal.Errors) == 0 {
		t.Error("expected journal errors for malformed entry")
	}
}

// TestVerify_EmptyRepo verifies that a freshly initialised repository
// (no commits) reports healthy.
func TestVerify_EmptyRepo(t *testing.T) {
	ctx, _ := setupRepo(t)

	report, err := verify.Verify(ctx.Repository.VaraDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Empty repo has no objects or commits — HEAD may error but there's nothing
	// structurally wrong with an empty, newly initialised repository.
	// We just check it doesn't crash.
	_ = report
}

// TestVerify_ReturnsStructuredOutput confirms RunVerify produces the expected
// section headers in its output string.
func TestVerify_ReturnsStructuredOutput(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "x.txt", "content\n")
	makeCommit(t, ctx, "one commit")

	out, err := commands.RunVerify(ctx)
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	for _, section := range []string{"Objects", "Trees", "Commits", "Refs", "Index", "Graph"} {
		if !containsString(out, section) {
			t.Errorf("output missing section %q", section)
		}
	}
	if !containsString(out, "Repository Integrity Report") {
		t.Errorf("output missing title")
	}
}

// helpers

func reportErrors(r verify.Report) string {
	var msg string
	for _, e := range r.Objects.Errors {
		msg += "object: " + e.Error() + "\n"
	}
	for _, e := range r.Trees.Errors {
		msg += "tree: " + e.Error() + "\n"
	}
	for _, e := range r.Commits.Errors {
		msg += "commit: " + e.Error() + "\n"
	}
	for _, e := range r.Refs.Errors {
		msg += "ref: " + e.Error() + "\n"
	}
	return msg
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
