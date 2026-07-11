package server

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/transport"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/transfer"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// These tests exercise the FULL HTTP stack — HTTPTransport client ↔ net/http ↔
// server.Handler ↔ Local transport ↔ engine — and mirror the RFC-0014
// local-transport suite (internal/transport/local_test.go). If the HTTP path
// behaves identically to the local path, RFC-0016 has succeeded: HTTP is just
// another Transport implementation, and the concurrency guarantee (RFC-0016 §7)
// survived the network boundary.

// serve initializes a repository named repoName under a fresh served root, spins
// up an httptest server over that root, and returns the server, the repo's
// working root, and a client HTTPTransport pointed at the repo.
func serve(t *testing.T, repoName string) (*httptest.Server, string, *transport.HTTPTransport) {
	t.Helper()
	root := t.TempDir()
	repoRoot := filepath.Join(root, repoName)
	if _, err := repository.Init(repoRoot); err != nil {
		t.Fatalf("init %s: %v", repoName, err)
	}
	ts := httptest.NewServer(Handler(root))
	t.Cleanup(ts.Close)

	cli, err := transport.OpenHTTP(ts.URL + "/" + repoName)
	if err != nil {
		t.Fatalf("open http client: %v", err)
	}
	return ts, repoRoot, cli
}

func storeOf(root string) *object.Store {
	return object.NewStore(filepath.Join(root, repository.VaraDir))
}

func refsOf(root string) *refs.FSResolver {
	return refs.NewFSResolver(filepath.Join(root, repository.VaraDir))
}

// commitFile writes a one-file commit into root's store and returns its ID.
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

// emptyPack builds a valid VPCK stream carrying zero objects — enough for a
// receive whose objects already live in the server store (the ref-update tests
// isolate the CAS path, not object transfer).
func emptyPack(t *testing.T, root string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := transfer.WritePack(storeOf(root), nil, &buf); err != nil {
		t.Fatalf("write empty pack: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestHTTPListRefsAndFetch(t *testing.T) {
	_, server, cli := serve(t, "srv")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	if err := refsOf(server).Update("refs/heads/main", c2); err != nil {
		t.Fatalf("set main: %v", err)
	}

	advs, err := cli.ListRefs()
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(advs) != 1 || advs[0].Name != "refs/heads/main" || advs[0].Target != c2 {
		t.Fatalf("advertisements = %+v", advs)
	}
	if head, _ := cli.HeadTarget(); head != "refs/heads/main" {
		t.Fatalf("head target = %q", head)
	}

	// Fetch the full closure over HTTP into a fresh client store.
	stream, err := cli.FetchPack([]types.CommitID{c2}, nil)
	if err != nil {
		t.Fatalf("fetch pack: %v", err)
	}
	defer stream.Close()

	client := filepath.Join(t.TempDir(), "client")
	if _, err := repository.Init(client); err != nil {
		t.Fatalf("init client: %v", err)
	}
	if _, err := transfer.ReadPack(storeOf(client), stream); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	for _, id := range []types.CommitID{c1, c2} {
		if _, err := storeOf(client).Read(types.ObjectID(id)); err != nil {
			t.Fatalf("client missing %s: %v", id.String()[:7], err)
		}
	}
}

func TestHTTPReceivePackFastForward(t *testing.T) {
	_, server, cli := serve(t, "srv")
	c1 := commitFile(t, server, "one")
	refsOf(server).Update("refs/heads/main", c1)
	c2 := commitFile(t, server, "two", c1) // objects already in server store

	res, err := cli.ReceivePack(emptyPack(t, server), []transport.RefUpdate{
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

func TestHTTPReceivePackRejectsNonFastForward(t *testing.T) {
	_, server, cli := serve(t, "srv")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	refsOf(server).Update("refs/heads/main", c2) // server at c2
	cDiv := commitFile(t, server, "divergent", c1)

	res, err := cli.ReceivePack(emptyPack(t, server), []transport.RefUpdate{
		{Name: "refs/heads/main", Old: c2, New: cDiv},
	})
	if err != nil {
		t.Fatalf("receive returned transport error: %v", err)
	}
	if len(res) != 1 || res[0].OK {
		t.Fatalf("non-fast-forward should be rejected: %+v", res)
	}
	if got, _ := refsOf(server).Resolve("refs/heads/main"); got != c2 {
		t.Fatalf("server main moved despite rejection: %s", got.String()[:7])
	}
}

func TestHTTPReceivePackStaleCAS(t *testing.T) {
	_, server, cli := serve(t, "srv")
	c1 := commitFile(t, server, "one")
	c2 := commitFile(t, server, "two", c1)
	refsOf(server).Update("refs/heads/main", c2) // actually at c2

	// Caller believes Old=c1 (stale) → CAS must fail.
	res, err := cli.ReceivePack(emptyPack(t, server), []transport.RefUpdate{
		{Name: "refs/heads/main", Old: c1, New: c2},
	})
	if err != nil {
		t.Fatalf("receive error: %v", err)
	}
	if res[0].OK {
		t.Fatal("stale CAS should be rejected")
	}
}

// TestHTTPConcurrentPushOneWins is the RFC-0016 acceptance proof: two divergent
// pushes race at one server ref over HTTP; exactly one wins and the ref ends at
// a real winner. If this passes, the Refs-lock CAS in Local.ReceivePack held
// across the network boundary — the concurrency contract (§7) is intact.
func TestHTTPConcurrentPushOneWins(t *testing.T) {
	ts, server, _ := serve(t, "srv")
	c1 := commitFile(t, server, "base")
	cA := commitFile(t, server, "branchA", c1)
	cB := commitFile(t, server, "branchB", c1)
	if err := refsOf(server).Update("refs/heads/main", c1); err != nil {
		t.Fatalf("set main: %v", err)
	}

	news := []types.CommitID{cA, cB}
	var wg sync.WaitGroup
	results := make([]bool, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cli, err := transport.OpenHTTP(ts.URL + "/srv")
			if err != nil {
				errs[i] = err
				return
			}
			defer cli.Close()
			res, err := cli.ReceivePack(emptyPack(t, server), []transport.RefUpdate{
				{Name: "refs/heads/main", Old: c1, New: news[i]},
			})
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = res[0].OK
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("push %d transport error: %v", i, err)
		}
	}
	wins := 0
	for _, ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner over HTTP, got %d (results=%v)", wins, results)
	}
	if got, _ := refsOf(server).Resolve("refs/heads/main"); got != cA && got != cB {
		t.Fatalf("server main = %s, expected cA or cB", got.String()[:7])
	}
}
