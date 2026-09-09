// Package graphindex_bench measures the CURRENT commit-graph index
// (pkg/graphindex, RFC-0013) at scale and pits it against a pointer-free
// flat-slice reader, so the decision to add an mmap layer is driven by numbers.
//
// Three load strategies are compared, all reading the same .vara/graph.idx:
//
//   - readonly — os.ReadFile only. The I/O + single-buffer floor. Any load
//     strategy pays at least this; the interesting cost is what each adds on top.
//   - current  — graphindex.Load(): parses into map[CommitID]*Entry + []*Entry.
//     The delta over readonly is the map/pointer allocation the mmap idea targets.
//   - flat     — Option B: parse into a contiguous []flatEntry of VALUE types plus
//     a sorted index for binary-search lookup. Keeps os.ReadFile; no mmap, no
//     build tags, no Windows work. This is the "is mmap even necessary?" baseline.
//   - mmap     — Option C stub (skipped): a zero-copy reader over the mmapped file.
//     Needs a build-tagged unix/windows abstraction; wired here so the comparison
//     is one command once RFC-0013 v2 authorizes it.
//
// Everything lives under benchmarks/ and uses only the public API, so it touches
// nothing in the frozen engine and needs no freeze exception.
package graphindex_bench

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Building a repo means one loose-object write per commit — slow on NTFS — so
// each size is built once and the immutable .vara dir is shared across all
// benchmarks (they only read graph.idx). TestMain removes the dirs at the end.
var (
	repoMu    sync.Mutex
	repoCache = map[int]string{}
	tempDirs  []string
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, d := range tempDirs {
		os.RemoveAll(d)
	}
	os.Exit(code)
}

// benchSizes are the commit counts to probe. Add 100000 to stress the OOM/GC
// claim on low-RAM VMs; setup is one loose-object write per commit, so large
// sizes are slow on NTFS and kept out of the committed default.
var benchSizes = []int{1000, 10000}

const mergeEvery = 14 // ~7% of commits are merges (realistic, not uniform-random)

// buildRepo creates a repo with n commits: mostly linear, with ~7% merge commits
// whose second parent is a handful of commits back (shallow fanout, like real
// history — not a deep random parent that would make traversal numbers lie). It
// writes HEAD + refs/heads/main and builds graph.idx, then returns the .vara dir.
// This is all setup; callers keep it outside the timed region.
func buildRepo(tb testing.TB, n int) string {
	tb.Helper()
	repoMu.Lock()
	defer repoMu.Unlock()
	if d, ok := repoCache[n]; ok {
		return d
	}

	root, err := os.MkdirTemp("", "vara-graphidx-bench-")
	if err != nil {
		tb.Fatal(err)
	}
	tempDirs = append(tempDirs, root)
	varaDir := filepath.Join(root, ".vara")
	if err := os.MkdirAll(filepath.Join(varaDir, "refs", "heads"), 0o755); err != nil {
		tb.Fatal(err)
	}
	store := object.NewStore(varaDir)

	treeID, err := store.Write(object.NewTree(nil)) // one shared empty tree
	if err != nil {
		tb.Fatal(err)
	}
	tree := types.TreeID(treeID)

	ids := make([]types.CommitID, 0, n)
	var prev types.CommitID
	for i := range n {
		var parents []types.CommitID
		if i > 0 {
			parents = []types.CommitID{prev}
			if i > 20 && i%mergeEvery == 0 {
				back := 2 + (i % 18) // shallow: 2..19 commits back
				parents = append(parents, ids[i-back])
			}
		}
		id, err := store.Write(&object.Commit{
			TreeHash:  tree,
			Parents:   parents,
			Author:    "bench <b@example.com>",
			Message:   fmt.Sprintf("commit %d", i),
			Timestamp: int64(i),
		})
		if err != nil {
			tb.Fatal(err)
		}
		prev = types.CommitID(id)
		ids = append(ids, prev)
	}

	if err := os.WriteFile(filepath.Join(varaDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(varaDir, "refs", "heads", "main"), []byte(prev.String()+"\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := graphindex.Build(varaDir, store); err != nil {
		tb.Fatal(err)
	}
	repoCache[n] = varaDir
	return varaDir
}

// ---- Load benchmarks: isolate I/O from parse/allocation cost --------------

// BenchmarkLoad_ReadOnly is the floor: read the graph.idx bytes and nothing else.
func BenchmarkLoad_ReadOnly(b *testing.B) {
	forEachSize(b, func(b *testing.B, varaDir string, _ int) {
		path := filepath.Join(varaDir, "graph.idx")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("empty index")
			}
		}
	})
}

// BenchmarkLoad_Current is the current path: read + parse into maps/pointers.
// Its allocs minus ReadOnly's are what a pointer-free reader would remove.
func BenchmarkLoad_Current(b *testing.B) {
	forEachSize(b, func(b *testing.B, varaDir string, n int) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			idx, err := graphindex.Load(varaDir)
			if err != nil {
				b.Fatal(err)
			}
			if idx.Len() != n {
				b.Fatalf("loaded %d commits, want %d", idx.Len(), n)
			}
		}
	})
}

// BenchmarkLoad_Flat is Option B: read + parse into a contiguous value slice plus
// a sorted index. No map, no per-commit pointers, no strings materialised.
func BenchmarkLoad_Flat(b *testing.B) {
	forEachSize(b, func(b *testing.B, varaDir string, n int) {
		path := filepath.Join(varaDir, "graph.idx")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fi, err := loadFlat(path)
			if err != nil {
				b.Fatal(err)
			}
			if len(fi.entries) != n {
				b.Fatalf("flat loaded %d commits, want %d", len(fi.entries), n)
			}
		}
	})
}

// ---- Traversal benchmarks: current map-walk vs flat index-walk ------------

func BenchmarkWalk_Current(b *testing.B) {
	forEachSize(b, func(b *testing.B, varaDir string, _ int) {
		idx, err := graphindex.Load(varaDir)
		if err != nil {
			b.Fatal(err)
		}
		tip := highestGeneration(idx)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if walkCurrent(idx, tip) == 0 {
				b.Fatal("walk visited nothing")
			}
		}
	})
}

func BenchmarkWalk_Flat(b *testing.B) {
	forEachSize(b, func(b *testing.B, varaDir string, _ int) {
		fi, err := loadFlat(filepath.Join(varaDir, "graph.idx"))
		if err != nil {
			b.Fatal(err)
		}
		tip := fi.highestGenerationPos()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if fi.walk(tip) == 0 {
				b.Fatal("walk visited nothing")
			}
		}
	})
}

// BenchmarkWalk_Mmap is the Option C stub: a zero-copy walk over the mmapped
// file. Skipped until the build-tagged mmap reader exists (RFC-0013 v2).
func BenchmarkWalk_Mmap(b *testing.B) {
	b.Skip("mmap reader not implemented — pending RFC-0013 v2 freeze exception")
}

// ---- current-index traversal helpers --------------------------------------

func highestGeneration(idx *graphindex.Index) types.CommitID {
	var tip *graphindex.Entry
	for _, e := range idx.All() {
		if tip == nil || e.Generation > tip.Generation {
			tip = e
		}
	}
	return tip.ID
}

func walkCurrent(idx *graphindex.Index, from types.CommitID) int {
	visited := make(map[types.CommitID]bool)
	stack := []types.CommitID{from}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[id] {
			continue
		}
		visited[id] = true
		if e := idx.Lookup(id); e != nil {
			stack = append(stack, e.Parents...)
		}
	}
	return len(visited)
}

// ---- Option B: pointer-free flat reader ------------------------------------
//
// graph.idx layout (RFC-0013, big-endian), re-read here independently:
//   header (16B): magic "VAGI" | version u32 | count u32 | reserved u32
//   commit table: count × 56B { id[32] | gen u32 | ts i64 | parentOff u32 |
//                               parentCount u32 | metaOff u32 }
//   parent array: totalParents × 32B (commit IDs, indexed by parentOff)
//   meta block:   author\0message\0 per commit (NOT read here — traversal
//                 doesn't need it, and skipping it avoids materialising strings)
//   checksum:     sha256(everything above), 32B

const (
	flatHeaderSize = 16
	flatEntrySize  = 56
	flatHashSize   = 32
)

// flatEntry is a value type (no pointers) so a slice of them is one contiguous
// allocation the GC never has to scan for pointers.
type flatEntry struct {
	id           [32]byte
	generation   uint32
	parentOffset uint32
	parentCount  uint32
}

type flatIndex struct {
	entries []flatEntry // file order
	parents [][32]byte  // flat parent array
	sorted  []uint32    // positions into entries, sorted by id (binary search)
}

func loadFlat(path string) (*flatIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < flatHeaderSize+flatHashSize {
		return nil, fmt.Errorf("flat: file too short")
	}
	body := data[:len(data)-flatHashSize]
	if string(body[0:4]) != "VAGI" {
		return nil, fmt.Errorf("flat: bad magic")
	}
	count := int(binary.BigEndian.Uint32(body[8:12]))

	entries := make([]flatEntry, count)
	var totalParents uint32
	for i := range count {
		base := flatHeaderSize + i*flatEntrySize
		if base+flatEntrySize > len(body) {
			return nil, fmt.Errorf("flat: truncated commit table")
		}
		copy(entries[i].id[:], body[base:base+32])
		entries[i].generation = binary.BigEndian.Uint32(body[base+32 : base+36])
		entries[i].parentOffset = binary.BigEndian.Uint32(body[base+44 : base+48])
		entries[i].parentCount = binary.BigEndian.Uint32(body[base+48 : base+52])
		totalParents += entries[i].parentCount
	}

	parentBase := flatHeaderSize + count*flatEntrySize
	parents := make([][32]byte, totalParents)
	for i := range int(totalParents) {
		off := parentBase + i*flatHashSize
		if off+flatHashSize > len(body) {
			return nil, fmt.Errorf("flat: truncated parent array")
		}
		copy(parents[i][:], body[off:off+flatHashSize])
	}

	sorted := make([]uint32, count)
	for i := range sorted {
		sorted[i] = uint32(i)
	}
	sort.Slice(sorted, func(a, b int) bool {
		return bytes.Compare(entries[sorted[a]].id[:], entries[sorted[b]].id[:]) < 0
	})

	return &flatIndex{entries: entries, parents: parents, sorted: sorted}, nil
}

// lookup returns the entry position for an id via binary search, or -1.
func (f *flatIndex) lookup(id [32]byte) int {
	lo, hi := 0, len(f.sorted)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch bytes.Compare(f.entries[f.sorted[mid]].id[:], id[:]) {
		case -1:
			lo = mid + 1
		case 1:
			hi = mid
		default:
			return int(f.sorted[mid])
		}
	}
	return -1
}

func (f *flatIndex) highestGenerationPos() int {
	best := 0
	for i := range f.entries {
		if f.entries[i].generation > f.entries[best].generation {
			best = i
		}
	}
	return best
}

// walk visits every ancestor of the entry at fromPos. visited is one []bool sized
// to the commit count; the walk allocates nothing per parent (binary-search lookup
// touches only the contiguous slices).
func (f *flatIndex) walk(fromPos int) int {
	visited := make([]bool, len(f.entries))
	stack := []int{fromPos}
	n := 0
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[p] {
			continue
		}
		visited[p] = true
		n++
		e := f.entries[p]
		for j := uint32(0); j < e.parentCount; j++ {
			if pp := f.lookup(f.parents[e.parentOffset+j]); pp >= 0 && !visited[pp] {
				stack = append(stack, pp)
			}
		}
	}
	return n
}

// forEachSize runs fn as a sub-benchmark per size, building the repo once outside
// the timed region.
func forEachSize(b *testing.B, fn func(b *testing.B, varaDir string, n int)) {
	b.Helper()
	for _, n := range benchSizes {
		varaDir := buildRepo(b, n)
		b.Run(fmt.Sprintf("commits=%d", n), func(b *testing.B) {
			fn(b, varaDir, n)
		})
	}
}
