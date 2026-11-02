package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// corruptOneObject damages the first object file it finds under root's .vara.
// Objects are stored read-only, so it unlocks the file and its folder first, the way
// a failing disk would not ask.
func corruptOneObject(t *testing.T, root string) string {
	t.Helper()
	var victim string
	entries, _ := os.ReadDir(filepath.Join(root, ".vara"))
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 2 {
			continue
		}
		dir := filepath.Join(root, ".vara", e.Name())
		files, _ := os.ReadDir(dir)
		if len(files) == 0 {
			continue
		}
		victim = filepath.Join(dir, files[0].Name())
		_ = os.Chmod(dir, 0o755)
		break
	}
	if victim == "" {
		t.Fatal("no object found to damage")
	}
	_ = os.Chmod(victim, 0o644)
	if err := os.WriteFile(victim, []byte("not a valid object"), 0o644); err != nil {
		t.Fatal(err)
	}
	return victim
}

func TestVerifyHealthyRepoReturnsNoError(t *testing.T) {
	root, _ := makeServer(t)
	out, err := RunVerify(ctxFor(t, root))
	if err != nil {
		t.Fatalf("healthy repository must not return an error, got %v", err)
	}
	if !strings.Contains(out, "Repository Healthy") {
		t.Fatalf("report should say healthy:\n%s", out)
	}
}

// A report that says CORRUPTION DETECTED while the command exits 0 hides the problem from
// every script and health check, so a damaged repository must come back as an error.
func TestVerifyCorruptRepoReturnsErrCorruption(t *testing.T) {
	root, _ := makeServer(t)
	corruptOneObject(t, root)
	out, err := RunVerify(ctxFor(t, root))
	if !errors.Is(err, ErrCorruption) {
		t.Fatalf("want ErrCorruption, got %v", err)
	}
	if !strings.Contains(out, "CORRUPTION DETECTED") {
		t.Fatalf("the report must still be returned so it can be printed:\n%s", out)
	}
}

// A category with nothing in it was not checked, and a green tick would say it was.
func TestEmptyCategoryDoesNotClaimToBeVerified(t *testing.T) {
	var sb strings.Builder
	writeCategory(&sb, "Commits", verify.CategoryResult{})
	if strings.Contains(sb.String(), "✔") {
		t.Fatalf("a category with nothing checked must not show a tick: %q", sb.String())
	}
	sb.Reset()
	writeCategory(&sb, "Objects", verify.CategoryResult{Verified: 4})
	if !strings.Contains(sb.String(), "✔ 4 verified") {
		t.Fatalf("a category that was checked keeps its tick: %q", sb.String())
	}
}
