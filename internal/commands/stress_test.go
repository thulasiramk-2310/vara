package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// TestStressRemote builds a large linear history and times the remote
// operations against it, reporting real numbers. It is skipped unless
// VARA_STRESS is set, so it never slows normal CI:
//
//	VARA_STRESS=1 VARA_STRESS_N=10000 go test ./internal/commands/ \
//	    -run TestStressRemote -v -timeout 30m
//
// Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS (3 objects per commit):
//
//	N       build    clone     verify   incr-fetch(100)  live-heap
//	1000    4.0s     40.1s     17.3s    3.7s             ~52 MiB
//	2000    8.0s     61.8s     48.3s    4.0s             ~52 MiB
//
// Clone scales linearly (~22 ms/commit + ~18 s fixed), NOT quadratically —
// the cost is NTFS per-object file operations (~5–7 ms each), the same limit
// documented for add/status. Live heap is flat regardless of N, so reading the
// whole pack into memory (ReadPack) is not a bottleneck at these sizes.
// Incremental fetch is ~constant: it tracks new commits, not total history.
func TestStressRemote(t *testing.T) {
	if os.Getenv("VARA_STRESS") == "" {
		t.Skip("set VARA_STRESS=1 to run (optionally VARA_STRESS_N=<commits>)")
	}
	n := 2000
	if v := os.Getenv("VARA_STRESS_N"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed
		}
	}
	t.Logf("building %d-commit history...", n)

	server := filepath.Join(t.TempDir(), "server")
	if _, err := repository.Init(server); err != nil {
		t.Fatal(err)
	}
	serverVara := filepath.Join(server, repository.VaraDir)
	store := object.NewStore(serverVara)
	res := refs.NewFSResolver(serverVara)

	// Each commit changes one file so trees/blobs differ commit to commit —
	// a realistic (worst-ish) case for object count.
	build := func(count int, startParent types.CommitID, startIdx int) types.CommitID {
		parent := startParent
		for i := range count {
			content := fmt.Sprintf("line %d\n", startIdx+i)
			blob, _ := store.Write(object.NewBlob([]byte(content)))
			tree, _ := store.Write(object.NewTree([]object.TreeEntry{
				{Mode: 0o100644, Hash: blob, Name: "f.txt"},
			}))
			var parents []types.CommitID
			if (parent != types.CommitID{}) {
				parents = []types.CommitID{parent}
			}
			id, err := store.Write(&object.Commit{
				TreeHash: types.TreeID(tree), Parents: parents,
				Author: "T <t@e.com>", Message: content,
			})
			if err != nil {
				t.Fatalf("build commit %d: %v", i, err)
			}
			parent = types.CommitID(id)
		}
		return parent
	}

	t0 := time.Now()
	head := build(n, types.CommitID{}, 0)
	res.Update("refs/heads/main", head)
	res.Symbolic("HEAD", "refs/heads/main")
	t.Logf("build:            %v (%d objects)", time.Since(t0).Round(time.Millisecond), 3*n)

	// --- full clone, measuring wall time and peak heap during the transfer ---
	dst := filepath.Join(t.TempDir(), "clone")
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	t0 = time.Now()
	if _, err := RunClone(server, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloneDur := time.Since(t0)
	runtime.ReadMemStats(&m1)
	t.Logf("clone (%d cmts):  %v", n, cloneDur.Round(time.Millisecond))
	t.Logf("clone alloc:      %.1f MiB total, heap peak ~%.1f MiB",
		float64(m1.TotalAlloc-m0.TotalAlloc)/(1<<20), float64(m1.HeapSys)/(1<<20))

	// --- verify the clone ---
	t0 = time.Now()
	rep, err := verify.Verify(filepath.Join(dst, repository.VaraDir))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	t.Logf("verify:           %v (healthy=%v)", time.Since(t0).Round(time.Millisecond), rep.Healthy)

	// --- incremental fetch: server adds 100 commits, client fetches ---
	head2 := build(100, head, n)
	res.Update("refs/heads/main", head2)
	ctx := ctxFor(t, dst)
	t0 = time.Now()
	if _, err := RunFetch(ctx, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	t.Logf("incr fetch (100): %v", time.Since(t0).Round(time.Millisecond))

	// --- gc on the clone (should find nothing to remove) ---
	t0 = time.Now()
	gcOut, err := RunGC(ctx, false)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	t.Logf("gc dry-run:       %v\n%s", time.Since(t0).Round(time.Millisecond), gcOut)
}
