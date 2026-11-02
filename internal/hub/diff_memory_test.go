package hub

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
)

// lcsLen is the textbook O(n*m) longest common subsequence length, used as the
// reference: the minimal edit script has n + m - 2*LCS insertions and deletions.
func lcsLen(a, b []string) int {
	prev := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
			} else if prev[j] >= cur[j-1] {
				cur[j] = prev[j]
			} else {
				cur[j] = cur[j-1]
			}
		}
		prev = cur
	}
	return prev[len(b)]
}

func randLines(r *rand.Rand, n, alphabet int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line-%d", r.Intn(alphabet))
	}
	return out
}

// The trimmed backtracking table must give exactly the diff the full table did:
// an edit script that rebuilds the new text from the old, and is minimal.
func TestMyersStaysCorrectAndMinimal(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 400; i++ {
		a := randLines(r, r.Intn(40), 1+r.Intn(8))
		b := randLines(r, r.Intn(40), 1+r.Intn(8))
		ops, ok := myersLines(a, b, 0)
		if !ok {
			t.Fatalf("case %d: no result for %d x %d lines", i, len(a), len(b))
		}
		var rebuilt, consumed []string
		edits := 0
		for _, op := range ops {
			switch op.kind {
			case '=':
				consumed = append(consumed, op.text)
				rebuilt = append(rebuilt, op.text)
			case '-':
				consumed = append(consumed, op.text)
				edits++
			case '+':
				rebuilt = append(rebuilt, op.text)
				edits++
			}
		}
		if fmt.Sprint(consumed) != fmt.Sprint(a) || fmt.Sprint(rebuilt) != fmt.Sprint(b) {
			t.Fatalf("case %d: edit script does not turn old into new", i)
		}
		if want := len(a) + len(b) - 2*lcsLen(a, b); edits != want {
			t.Fatalf("case %d: %d edits, minimal is %d", i, edits, want)
		}
	}
}

// One request for a fully rewritten 6,000-line file used 1.9 GB before the
// backtracking table was bounded, and a handful at once killed the server.
func TestMyersMemoryIsBounded(t *testing.T) {
	a := make([]string, 6000)
	b := make([]string, 6000)
	for i := range a {
		a[i] = fmt.Sprintf("old line %d", i)
		b[i] = fmt.Sprintf("new line %d", i)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	_, ok := myersLines(a, b, maxDiffEdits)
	runtime.ReadMemStats(&after)
	if ok {
		t.Fatal("a total rewrite should be reported as too different")
	}
	allocated := (after.TotalAlloc - before.TotalAlloc) >> 20
	if allocated > 400 {
		t.Fatalf("one diff allocated %d MiB, want under 400", allocated)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

// Many expensive diffs at once must queue, not multiply their memory.
func TestMyersConcurrencyIsLimited(t *testing.T) {
	a := make([]string, 3000)
	b := make([]string, 3000)
	for i := range a {
		a[i] = fmt.Sprintf("old %d", i)
		b[i] = fmt.Sprintf("new %d", i)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			myersLines(a, b, maxDiffEdits)
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("24 queued diffs did not finish")
	}
	if len(diffSlots) != 0 {
		t.Fatalf("%d slots still held after all diffs finished", len(diffSlots))
	}
}
