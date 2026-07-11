package commands

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/server"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// These tests drive the real clone/fetch/push commands over HTTP, proving the
// command layer is oblivious to the transport: the same RunClone/RunPush that
// work over a local path work unchanged over an http:// URL (RFC-0016 §9). The
// only difference from the local-transport tests is the URL scheme.

// httpServe initializes repoName under a served root and returns the root, the
// repo working root, and the base URL for that repo.
func httpServe(t *testing.T, repoName string) (root, repoRoot, baseURL string) {
	t.Helper()
	root = t.TempDir()
	repoRoot = filepath.Join(root, repoName)
	initRepo(t, repoRoot)
	ts := httptest.NewServer(server.Handler(root))
	t.Cleanup(ts.Close)
	return root, repoRoot, ts.URL + "/" + repoName
}

func TestCloneOverHTTP(t *testing.T) {
	_, server, url := httpServe(t, "srv")
	c1 := commitInRepo(t, server, "a.txt", "hello")
	setMain(t, server, c1)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(url, dst); err != nil {
		t.Fatalf("clone over http: %v", err)
	}

	// Working tree materialized, and both the local branch and tracking ref
	// point at c1 — identical outcome to a local clone.
	if b, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(b) != "hello" {
		t.Fatalf("clone working tree a.txt = %q err=%v", b, err)
	}
	if got := resolveRef(t, dst, "refs/heads/main"); got != c1 {
		t.Fatalf("clone main = %s want c1", got.String()[:7])
	}
	if got := resolveRef(t, dst, "refs/remotes/origin/main"); got != c1 {
		t.Fatalf("clone tracking ref = %s want c1", got.String()[:7])
	}
}

func TestPushOverHTTP(t *testing.T) {
	_, srv, url := httpServe(t, "srv")
	c1 := commitInRepo(t, srv, "a.txt", "hello")
	setMain(t, srv, c1)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(url, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Build a new commit in the clone and push it back over HTTP.
	c2 := commitInRepo(t, dst, "a.txt", "hello\nfrom clone", c1)
	setMain(t, dst, c2)

	ctx := ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", false); err != nil {
		t.Fatalf("push over http: %v", err)
	}

	// Server advanced to c2 and the object actually transferred over the wire.
	if got := resolveRef(t, srv, "refs/heads/main"); got != c2 {
		t.Fatalf("server main after push = %s want c2", got.String()[:7])
	}
	ss := object.NewStore(filepath.Join(srv, repository.VaraDir))
	if _, err := ss.Read(types.ObjectID(c2)); err != nil {
		t.Fatalf("server missing pushed commit: %v", err)
	}
}

func TestPushOverHTTPNonFastForwardRejected(t *testing.T) {
	_, srv, url := httpServe(t, "srv")
	c1 := commitInRepo(t, srv, "a.txt", "hello")
	setMain(t, srv, c1)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(url, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Both sides diverge from c1.
	sDiv := commitInRepo(t, srv, "s.txt", "server work", c1)
	setMain(t, srv, sDiv)
	cDiv := commitInRepo(t, dst, "c.txt", "clone work", c1)
	setMain(t, dst, cDiv)

	ctx := ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", false); err == nil {
		t.Fatal("non-fast-forward push over http should be rejected")
	}
	if got := resolveRef(t, srv, "refs/heads/main"); got != sDiv {
		t.Fatalf("server main moved despite rejection: %s", got.String()[:7])
	}
}

func TestFetchOverHTTP(t *testing.T) {
	_, srv, url := httpServe(t, "srv")
	c1 := commitInRepo(t, srv, "a.txt", "hello")
	setMain(t, srv, c1)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(url, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Server advances; fetch must move the tracking ref but not the local branch.
	c2 := commitInRepo(t, srv, "a.txt", "hello\nworld", c1)
	setMain(t, srv, c2)

	ctx := ctxFor(t, dst)
	if _, err := RunFetch(ctx, "origin"); err != nil {
		t.Fatalf("fetch over http: %v", err)
	}
	if got := resolveRef(t, dst, "refs/remotes/origin/main"); got != c2 {
		t.Fatalf("tracking ref = %s want c2", got.String()[:7])
	}
	if got := resolveRef(t, dst, "refs/heads/main"); got != c1 {
		t.Fatalf("local main moved on fetch: %s want c1", got.String()[:7])
	}
}
