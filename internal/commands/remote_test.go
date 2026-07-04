package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// --- helpers ---

func initRepo(t *testing.T, path string) *repository.Repository {
	t.Helper()
	repo, err := repository.Init(path)
	if err != nil {
		t.Fatalf("init %s: %v", path, err)
	}
	return repo
}

func ctxFor(t *testing.T, root string) *Context {
	t.Helper()
	repo, err := repository.Discover(root)
	if err != nil {
		t.Fatalf("discover %s: %v", root, err)
	}
	idx := reloadIndex(repo.VaraDir)
	if idx == nil {
		idx = index.New()
	}
	return &Context{Repository: repo, Index: idx}
}

// commitInRepo writes a single-file commit into root's store and returns its ID.
func commitInRepo(t *testing.T, root, filename, content string, parents ...types.CommitID) types.CommitID {
	t.Helper()
	s := object.NewStore(filepath.Join(root, repository.VaraDir))
	blob, err := s.Write(object.NewBlob([]byte(content)))
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	tree, err := s.Write(object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: blob, Name: filename},
	}))
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	id, err := s.Write(&object.Commit{
		TreeHash: types.TreeID(tree),
		Parents:  parents,
		Author:   "Test <t@example.com>",
		Message:  content,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return types.CommitID(id)
}

func setMain(t *testing.T, root string, id types.CommitID) {
	t.Helper()
	r := refs.NewFSResolver(filepath.Join(root, repository.VaraDir))
	if err := r.Update("refs/heads/main", id); err != nil {
		t.Fatalf("set main: %v", err)
	}
	if err := r.Symbolic("HEAD", "refs/heads/main"); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
}

func resolveRef(t *testing.T, root, name string) types.CommitID {
	t.Helper()
	r := refs.NewFSResolver(filepath.Join(root, repository.VaraDir))
	id, err := r.Resolve(name)
	if err != nil {
		t.Fatalf("resolve %s in %s: %v", name, root, err)
	}
	return id
}

// makeServer builds a server repo with one commit on main and returns its root.
func makeServer(t *testing.T) (root string, c1 types.CommitID) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "server")
	initRepo(t, root)
	c1 = commitInRepo(t, root, "a.txt", "hello")
	setMain(t, root, c1)
	return root, c1
}

// --- tests ---

func TestCloneCopiesEverything(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")

	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Working tree file materialized.
	if b, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(b) != "hello" {
		t.Fatalf("clone working tree a.txt = %q err=%v", b, err)
	}
	// Local branch and tracking ref both point at c1.
	if got := resolveRef(t, dst, "refs/heads/main"); got != c1 {
		t.Fatalf("clone main = %s want c1", got.String()[:7])
	}
	if got := resolveRef(t, dst, "refs/remotes/origin/main"); got != c1 {
		t.Fatalf("clone tracking ref = %s want c1", got.String()[:7])
	}
	// origin remote recorded.
	ctx := ctxFor(t, dst)
	out, err := RunRemote(ctx, nil)
	if err != nil || out == "" {
		t.Fatalf("remote list after clone: %q err=%v", out, err)
	}
}

func TestCloneRollbackOnFailure(t *testing.T) {
	// A server whose main points at a commit that does not exist in its store.
	// ListRefs succeeds (it only parses the ref), but the object transfer fails,
	// which must trigger rollback of the partial destination.
	server := filepath.Join(t.TempDir(), "server")
	initRepo(t, server)
	var bogus types.CommitID
	for i := range bogus {
		bogus[i] = 0xAB
	}
	setMain(t, server, bogus)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err == nil {
		t.Fatal("clone from a dangling ref should fail")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("clone left a partial destination behind: stat err = %v", err)
	}
}

func TestFetchUpdatesTrackingNotLocal(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Server advances.
	c2 := commitInRepo(t, server, "a.txt", "hello\nworld", c1)
	setMain(t, server, c2)

	ctx := ctxFor(t, dst)
	if _, err := RunFetch(ctx, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// Tracking ref moved, local branch did NOT.
	if got := resolveRef(t, dst, "refs/remotes/origin/main"); got != c2 {
		t.Fatalf("tracking ref = %s want c2", got.String()[:7])
	}
	if got := resolveRef(t, dst, "refs/heads/main"); got != c1 {
		t.Fatalf("local main moved on fetch: %s want c1", got.String()[:7])
	}
}

func TestPullFastForward(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}
	c2 := commitInRepo(t, server, "a.txt", "hello\nworld", c1)
	setMain(t, server, c2)

	ctx := ctxFor(t, dst)
	if _, err := RunPull(ctx, "origin", "main"); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if got := resolveRef(t, dst, "refs/heads/main"); got != c2 {
		t.Fatalf("local main after ff pull = %s want c2", got.String()[:7])
	}
}

func TestPushFastForward(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}
	// Clone builds a new commit on top of c1 (directly in clone's store).
	c2 := commitInRepo(t, dst, "a.txt", "hello\nfrom clone", c1)
	setMain(t, dst, c2)

	ctx := ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", false); err != nil {
		t.Fatalf("push: %v", err)
	}
	// Server main advanced to c2, and the object is present there.
	if got := resolveRef(t, server, "refs/heads/main"); got != c2 {
		t.Fatalf("server main after push = %s want c2", got.String()[:7])
	}
	ss := object.NewStore(filepath.Join(server, repository.VaraDir))
	if _, err := ss.Read(types.ObjectID(c2)); err != nil {
		t.Fatalf("server missing pushed commit: %v", err)
	}
}

func TestPushNonFastForwardRejected(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Both sides diverge from c1.
	sDiv := commitInRepo(t, server, "s.txt", "server work", c1)
	setMain(t, server, sDiv)
	cDiv := commitInRepo(t, dst, "c.txt", "clone work", c1)
	setMain(t, dst, cDiv)

	ctx := ctxFor(t, dst)
	_, err := RunPush(ctx, "origin", "main", false)
	if err == nil {
		t.Fatal("non-fast-forward push should be rejected")
	}
	// Server ref must be unchanged.
	if got := resolveRef(t, server, "refs/heads/main"); got != sDiv {
		t.Fatalf("server main moved despite rejection: %s", got.String()[:7])
	}
}

func TestPushForceOverwrites(t *testing.T) {
	server, c1 := makeServer(t)
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}
	sDiv := commitInRepo(t, server, "s.txt", "server work", c1)
	setMain(t, server, sDiv)
	cDiv := commitInRepo(t, dst, "c.txt", "clone work", c1)
	setMain(t, dst, cDiv)

	ctx := ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", true); err != nil {
		t.Fatalf("forced push: %v", err)
	}
	if got := resolveRef(t, server, "refs/heads/main"); got != cDiv {
		t.Fatalf("forced push did not overwrite: %s want cDiv", got.String()[:7])
	}
}
