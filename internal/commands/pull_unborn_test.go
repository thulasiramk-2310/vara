package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// emptyServerAndClone makes an empty server repository and a clone of it, the way two people
// start a project: both clone before anyone has committed.
func emptyServerAndClone(t *testing.T) (server, clone string) {
	t.Helper()
	server = filepath.Join(t.TempDir(), "server")
	initRepo(t, server)
	clone = filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, clone); err != nil {
		t.Fatalf("clone of an empty repository: %v", err)
	}
	return server, clone
}

// A branch with no commits has nothing to merge into, so taking the remote's first commit is a
// fast-forward from nothing. It used to fail with "HEAD is not a valid commit".
func TestPullIntoAnEmptyCloneTakesTheFirstCommit(t *testing.T) {
	server, clone := emptyServerAndClone(t)
	c1 := commitInRepo(t, server, "a.txt", "hello")
	setMain(t, server, c1)

	if _, err := RunPull(ctxFor(t, clone), "origin", "main"); err != nil {
		t.Fatalf("pull into an empty clone: %v", err)
	}
	if got := resolveRef(t, clone, "refs/heads/main"); got != c1 {
		t.Fatalf("main after the first pull = %s, want the remote's first commit", got.String()[:7])
	}
	b, err := os.ReadFile(filepath.Join(clone, "a.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("the file should be checked out, got %q, %v", b, err)
	}
}

// The first pull used to leave the clone unable to pull again ("find merge base: reference not found").
func TestPullAfterTheFirstPullStillWorksWhenHistoriesDiverge(t *testing.T) {
	server, clone := emptyServerAndClone(t)
	c1 := commitInRepo(t, server, "a.txt", "hello")
	setMain(t, server, c1)
	if _, err := RunPull(ctxFor(t, clone), "origin", "main"); err != nil {
		t.Fatal(err)
	}

	c2 := commitInRepo(t, server, "b.txt", "theirs", c1)
	setMain(t, server, c2)

	if err := devidentity.SetRepo(filepath.Join(clone, repository.VaraDir), devidentity.Identity{Name: "T", Email: "t@example.com"}); err != nil {
		t.Fatal(err)
	}
	writeWorktree(t, clone, "local.txt", "mine")
	ctx := ctxFor(t, clone)
	if err := RunAdd(ctx, []string{"local.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunCommit(ctx, "local work"); err != nil {
		t.Fatal(err)
	}

	if _, err := RunPull(ctxFor(t, clone), "origin", "main"); err != nil {
		t.Fatalf("a diverged pull after the first pull must merge, got %v", err)
	}
	// commitInRepo records a commit holding only the one file it is given, so the remote's second
	// commit adds b.txt and drops a.txt; the merge must carry that and keep the local file.
	for _, f := range []string{"b.txt", "local.txt"} {
		if _, err := os.Stat(filepath.Join(clone, f)); err != nil {
			t.Fatalf("%s should exist after the merge: %v", f, err)
		}
	}
	head := resolveRef(t, clone, "refs/heads/main")
	obj, err := object.NewStore(filepath.Join(clone, repository.VaraDir)).Read(types.ObjectID(head))
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := obj.(*object.Commit); !ok || len(c.Parents) != 2 {
		t.Fatalf("the result should be a two-parent merge commit")
	}
}

// Your own untracked file is never overwritten silently.
func TestPullIntoAnEmptyCloneRefusesToOverwriteALocalFile(t *testing.T) {
	server, clone := emptyServerAndClone(t)
	c1 := commitInRepo(t, server, "a.txt", "hello")
	setMain(t, server, c1)
	writeWorktree(t, clone, "a.txt", "mine, not theirs")

	_, err := RunPull(ctxFor(t, clone), "origin", "main")
	if err == nil || !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("want an error naming a.txt, got %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(clone, "a.txt"))
	if string(b) != "mine, not theirs" {
		t.Fatalf("the local file must be untouched, got %q", b)
	}
	if _, rerr := os.Stat(filepath.Join(clone, repository.VaraDir, "refs", "heads", "main")); rerr == nil {
		t.Fatal("a refused pull must not create the branch")
	}
}
