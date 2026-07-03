package transport

import (
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/transfer"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// makeRepo initializes a repository under a temp dir and returns its root.
func makeRepo(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if _, err := repository.Init(root); err != nil {
		t.Fatalf("init %s: %v", name, err)
	}
	return root
}

func storeOf(root string) *object.Store {
	// Objects live under the .vara root directly (see local.go note).
	return object.NewStore(filepath.Join(root, repository.VaraDir))
}

func refsOf(root string) *refs.FSResolver {
	return refs.NewFSResolver(filepath.Join(root, repository.VaraDir))
}

// commitFile writes a one-file commit and returns its ID.
func commitFile(t *testing.T, root, content string, parents ...types.CommitID) types.CommitID {
	t.Helper()
	s := storeOf(root)
	blob, err := s.Write(object.NewBlob([]byte(content)))
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	tree, err := s.Write(object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: blob, Name: "file.txt"},
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

func TestParsePath(t *testing.T) {
	if _, err := Open("https://example.com/repo"); err == nil {
		t.Fatal("expected https scheme to be rejected")
	}
	abs, err := ParsePath("./some/path")
	if err != nil || !filepath.IsAbs(abs) {
		t.Fatalf("ParsePath relative: %q %v", abs, err)
	}
}

func TestOpenRejectsNonRepo(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open on a non-repo dir should fail")
	}
}

func TestListRefsAndFetch(t *testing.T) {
	server := makeRepo(t, "server")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	if err := refsOf(server).Update("refs/heads/main", c2); err != nil {
		t.Fatalf("set server main: %v", err)
	}

	tr, err := Open(server)
	if err != nil {
		t.Fatalf("open server: %v", err)
	}
	defer tr.Close()

	advs, err := tr.ListRefs()
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(advs) != 1 || advs[0].Name != "refs/heads/main" || advs[0].Target != c2 {
		t.Fatalf("advertisements = %+v", advs)
	}

	// Fetch full history into a fresh client store.
	stream, err := tr.FetchPack([]types.CommitID{c2}, nil)
	if err != nil {
		t.Fatalf("fetch pack: %v", err)
	}
	defer stream.Close()

	client := makeRepo(t, "client")
	if _, err := transfer.ReadPack(storeOf(client), stream); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	// Client must resolve the full closure of c2.
	if _, err := storeOf(client).Read(types.ObjectID(c1)); err != nil {
		t.Fatalf("client missing ancestor c1: %v", err)
	}
	if _, err := storeOf(client).Read(types.ObjectID(c2)); err != nil {
		t.Fatalf("client missing tip c2: %v", err)
	}
}

func TestReceivePackFastForward(t *testing.T) {
	server := makeRepo(t, "server")
	c1 := commitFile(t, server, "one")
	refsOf(server).Update("refs/heads/main", c1)

	// Client builds c2 on top of c1 (in the SAME store for test simplicity),
	// then pushes it back to the server via the transport.
	c2 := commitFile(t, server, "two", c1)

	tr, _ := Open(server)
	defer tr.Close()

	// Fast-forward c1 -> c2: allowed.
	stream, _ := tr.FetchPack([]types.CommitID{c2}, []types.CommitID{c1})
	res, err := tr.ReceivePack(stream, []RefUpdate{
		{Name: "refs/heads/main", Old: c1, New: c2},
	})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if len(res) != 1 || !res[0].OK {
		t.Fatalf("fast-forward should succeed: %+v", res)
	}
	if got, _ := refsOf(server).Resolve("refs/heads/main"); got != c2 {
		t.Fatalf("server main = %s, want c2", got.String()[:7])
	}
}

func TestReceivePackRejectsNonFastForward(t *testing.T) {
	server := makeRepo(t, "server")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	refsOf(server).Update("refs/heads/main", c2) // server is at c2

	// A divergent commit c1' that is NOT a descendant of c2.
	cDiv := commitFile(t, server, "divergent", c1)

	tr, _ := Open(server)
	defer tr.Close()

	stream, _ := tr.FetchPack([]types.CommitID{cDiv}, []types.CommitID{c1})
	// Caller wrongly claims Old=c2; new=cDiv is not a descendant of c2.
	res, err := tr.ReceivePack(stream, []RefUpdate{
		{Name: "refs/heads/main", Old: c2, New: cDiv},
	})
	if err != nil {
		t.Fatalf("receive returned transport error: %v", err)
	}
	if len(res) != 1 || res[0].OK {
		t.Fatalf("non-fast-forward should be rejected: %+v", res)
	}
	// Server ref must be unchanged.
	if got, _ := refsOf(server).Resolve("refs/heads/main"); got != c2 {
		t.Fatalf("server main moved despite rejection: %s", got.String()[:7])
	}
}

func TestReceivePackStaleCAS(t *testing.T) {
	server := makeRepo(t, "server")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	refsOf(server).Update("refs/heads/main", c2) // actually at c2

	tr, _ := Open(server)
	defer tr.Close()

	// Caller believes Old=c1 (stale) but server is at c2 → CAS must fail.
	stream, _ := tr.FetchPack([]types.CommitID{c2}, nil)
	res, err := tr.ReceivePack(stream, []RefUpdate{
		{Name: "refs/heads/main", Old: c1, New: c2},
	})
	if err != nil {
		t.Fatalf("receive error: %v", err)
	}
	if res[0].OK {
		t.Fatal("stale CAS should be rejected")
	}
}
