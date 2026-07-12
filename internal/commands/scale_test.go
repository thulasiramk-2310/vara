package commands

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/server"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// TestScaleValidation drives the full lifecycle over an AUTHENTICATED HTTP server
// against a RICH repository — wide/deep trees, text + binary blobs, and
// branch/merge topology — measuring time, memory, and on-disk size, and
// exercising concurrency (parallel clones + concurrent divergent pushes). It is
// skipped unless VARA_SCALE is set:
//
//	VARA_SCALE=1 VARA_SCALE_N=1000 go test ./internal/commands/ \
//	    -run TestScaleValidation -v -timeout 30m
//
// Tunables: VARA_SCALE_N (commits), VARA_SCALE_FILES (working-set files),
// VARA_SCALE_CLONES (parallel clones).
func TestScaleValidation(t *testing.T) {
	if os.Getenv("VARA_SCALE") == "" {
		t.Skip("set VARA_SCALE=1 to run (VARA_SCALE_N / _FILES / _CLONES optional)")
	}
	n := envInt("VARA_SCALE_N", 300)
	nFiles := envInt("VARA_SCALE_FILES", 64)
	nClones := envInt("VARA_SCALE_CLONES", 4)
	rng := rand.New(rand.NewSource(1))

	root := t.TempDir()
	serverRepo := filepath.Join(root, "demo")
	if _, err := repository.Init(serverRepo); err != nil {
		t.Fatal(err)
	}
	vd := filepath.Join(serverRepo, repository.VaraDir)
	store := object.NewStore(vd)
	res := refs.NewFSResolver(vd)

	// --- rich working set: nFiles spread across a wide/deep directory tree,
	//     a quarter of them binary. ---
	files := map[string][]byte{}
	for i := range nFiles {
		path := fmt.Sprintf("src/pkg%d/mod%d/file%d.dat", i%5, i%11, i)
		if i%4 == 0 {
			b := make([]byte, 256+rng.Intn(1024)) // binary
			cryptorand.Read(b)
			files[path] = b
		} else {
			files[path] = fmt.Appendf(nil, "package p\n// file %d rev 0\n", i)
		}
	}
	paths := sortedKeys(files)

	commit := func(parents []types.CommitID, msg string) types.CommitID {
		tree, err := buildTreeFromMap(store, files, "")
		if err != nil {
			t.Fatalf("build tree: %v", err)
		}
		id, err := store.Write(&object.Commit{
			TreeHash: types.TreeID(tree), Parents: parents,
			Author: "Scale <s@e.com>", Message: msg,
		})
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		return types.CommitID(id)
	}

	t0 := time.Now()
	var head types.CommitID
	merges := 0
	for i := range n {
		// Mutate a few files each commit (realistic churn).
		for range 3 {
			p := paths[rng.Intn(len(paths))]
			files[p] = fmt.Appendf(nil, "package p\n// %s rev %d\n", p, i)
		}
		var parents []types.CommitID
		if (head != types.CommitID{}) {
			parents = []types.CommitID{head}
		}
		head = commit(parents, fmt.Sprintf("commit %d", i))

		// Every 50 commits: fork a short branch and merge it back (2-parent
		// commit) so the history has real merge topology, not just a line.
		if i > 0 && i%50 == 0 {
			branch := head
			for b := range 5 {
				p := paths[rng.Intn(len(paths))]
				files[p] = fmt.Appendf(nil, "branch work %d-%d\n", i, b)
				branch = commit([]types.CommitID{branch}, fmt.Sprintf("branch %d.%d", i, b))
			}
			head = commit([]types.CommitID{head, branch}, fmt.Sprintf("merge at %d", i))
			merges++
		}
	}
	res.Update("refs/heads/main", head)
	res.Symbolic("HEAD", "refs/heads/main")
	buildDur := time.Since(t0)

	objCount := countLooseObjects(vd)
	sizeMiB := float64(dirSize(vd)) / (1 << 20)
	t.Logf("== generation ==")
	t.Logf("commits=%d files=%d merges=%d build=%v", n, nFiles, merges, buildDur.Round(time.Millisecond))
	t.Logf("objects=%d  .vara=%.1f MiB", objCount, sizeMiB)

	// --- stand up an authenticated HTTP server (identity + authz + repo mgmt) ---
	policyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(policyDir, "demo.json"),
		[]byte(`{"version":1,"subjects":{"dev":["read","create-ref","push","force-push"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	acctDir := t.TempDir()
	mgr, err := identity.NewAccountManager(acctDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.CreateAccount("dev", "devpassword"); err != nil {
		t.Fatal(err)
	}
	opts := server.Options{
		Identity: &identity.Multi{Sources: []identity.IdentitySource{mgr.BasicSource(), mgr.BearerSource()}, AllowAnonymous: true},
		Authz:    authz.NewEnforcer(authz.NewStore(policyDir), nil),
		Methods:  []string{"auth-basic"},
	}
	ts := httptest.NewServer(server.HandlerWithOptions(root, opts))
	defer ts.Close()
	authURL := fmt.Sprintf("http://dev:devpassword@%s/demo", ts.Listener.Addr().String())

	// --- authenticated clone over HTTP, measuring wall time + heap ---
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	dst := filepath.Join(t.TempDir(), "clone")
	t0 = time.Now()
	if _, err := RunClone(authURL, dst); err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloneDur := time.Since(t0)
	runtime.ReadMemStats(&m1)
	t.Logf("== lifecycle (HTTP + auth) ==")
	t.Logf("clone:      %v  (+%.1f MiB alloc, heap ~%.1f MiB)", cloneDur.Round(time.Millisecond),
		float64(m1.TotalAlloc-m0.TotalAlloc)/(1<<20), float64(m1.HeapSys)/(1<<20))

	// --- verify the clone (deep object/DAG audit) ---
	t0 = time.Now()
	rep, err := verify.Verify(filepath.Join(dst, repository.VaraDir))
	if err != nil || !rep.Healthy {
		t.Fatalf("verify: healthy=%v err=%v", rep.Healthy, err)
	}
	t.Logf("verify:     %v (healthy)", time.Since(t0).Round(time.Millisecond))

	// --- history + status on the clone ---
	cctx := ctxFor(t, dst)
	t0 = time.Now()
	if _, err := RunHistory(cctx); err != nil {
		t.Fatalf("history: %v", err)
	}
	t.Logf("history:    %v", time.Since(t0).Round(time.Millisecond))
	t0 = time.Now()
	if _, err := RunStatus(cctx); err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status:     %v", time.Since(t0).Round(time.Millisecond))

	// --- incremental fetch: server adds 50 commits ---
	for i := range 50 {
		files[paths[i%len(paths)]] = fmt.Appendf(nil, "post-clone %d\n", i)
		head = commit([]types.CommitID{head}, fmt.Sprintf("post %d", i))
	}
	res.Update("refs/heads/main", head)
	t0 = time.Now()
	if _, err := RunFetch(cctx, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	t.Logf("incr-fetch: %v (+50 commits)", time.Since(t0).Round(time.Millisecond))

	// --- authenticated push: commit on the clone atop the fetched server head
	//     (a legitimate fast-forward) and push it back. ---
	cloneVd := filepath.Join(dst, repository.VaraDir)
	cstore := object.NewStore(cloneVd)
	cres := refs.NewFSResolver(cloneVd)
	base, err := cres.Resolve("refs/remotes/origin/main")
	if err != nil {
		t.Fatalf("resolve origin/main after fetch: %v", err)
	}
	files["src/pkg0/pushed.txt"] = []byte("pushed from clone\n")
	ptree, _ := buildTreeFromMap(cstore, files, "")
	pid, _ := cstore.Write(&object.Commit{TreeHash: types.TreeID(ptree), Parents: []types.CommitID{base}, Author: "dev <d@e.com>", Message: "client push"})
	cres.Update("refs/heads/main", types.CommitID(pid))
	t0 = time.Now()
	if _, err := RunPush(cctx, "origin", "main", false); err != nil {
		t.Fatalf("push: %v", err)
	}
	t.Logf("push:       %v", time.Since(t0).Round(time.Millisecond))
	if got, _ := res.Resolve("refs/heads/main"); got != types.CommitID(pid) {
		t.Fatalf("server did not accept the push")
	}

	// --- gc on the server (nothing unreferenced expected) ---
	sctx := ctxFor(t, serverRepo)
	t0 = time.Now()
	if _, err := RunGC(sctx, false); err != nil {
		t.Fatalf("gc: %v", err)
	}
	t.Logf("gc dry-run: %v", time.Since(t0).Round(time.Millisecond))

	// --- concurrency: N parallel authenticated clones ---
	t0 = time.Now()
	var wg sync.WaitGroup
	errs := make([]error, nClones)
	for i := range nClones {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := filepath.Join(t.TempDir(), fmt.Sprintf("pc%d", i))
			_, errs[i] = RunClone(authURL, d)
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("parallel clone %d: %v", i, e)
		}
	}
	t.Logf("== concurrency ==")
	t.Logf("%d parallel clones: %v (all OK)", nClones, time.Since(t0).Round(time.Millisecond))
}

// buildTreeFromMap writes the tree for every file whose path begins with prefix
// (prefix is "" or ends with "/") and returns its ID. Unchanged subtrees/blobs
// dedupe by content hash, so rebuilding the whole tree each commit stays bounded
// on disk while producing realistic per-commit churn.
func buildTreeFromMap(store *object.Store, files map[string][]byte, prefix string) (types.ObjectID, error) {
	seen := map[string]bool{}
	var entries []object.TreeEntry
	for path, content := range files {
		if len(path) < len(prefix) || path[:len(prefix)] != prefix {
			continue
		}
		rest := path[len(prefix):]
		slash := -1
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				slash = i
				break
			}
		}
		if slash >= 0 {
			d := rest[:slash]
			if seen[d] {
				continue
			}
			seen[d] = true
			sub, err := buildTreeFromMap(store, files, prefix+d+"/")
			if err != nil {
				return types.ObjectID{}, err
			}
			entries = append(entries, object.TreeEntry{Mode: 0o040000, Hash: sub, Name: d})
		} else {
			blob, err := store.Write(object.NewBlob(content))
			if err != nil {
				return types.ObjectID{}, err
			}
			entries = append(entries, object.TreeEntry{Mode: 0o100644, Hash: blob, Name: rest})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return store.Write(object.NewTree(entries))
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dirSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return def
}
