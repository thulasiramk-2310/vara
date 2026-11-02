package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/locking"
)

func newTxn(t *testing.T) *Transaction {
	t.Helper()
	vd := filepath.Join(t.TempDir(), ".vara")
	if err := os.MkdirAll(filepath.Join(vd, "locks"), 0o755); err != nil {
		t.Fatal(err)
	}
	txn, err := Begin(vd, "test", locking.NameRefs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(txn.Rollback)
	return txn
}

// Crash recovery skips a journal entry it cannot parse, so a half-written journal is an
// interrupted operation that can never be recovered. The journal must only ever change by an
// atomic replace: a reader sees the old complete file or the new complete file, never a torn one.
func TestJournalIsNeverSeenHalfWritten(t *testing.T) {
	txn := newTxn(t)

	var stop atomic.Bool
	var reads, torn atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			b, err := os.ReadFile(txn.journalPath)
			if err != nil {
				continue
			}
			reads.Add(1)
			if !json.Valid(b) {
				torn.Add(1)
			}
		}
	}()

	states := []State{StateExecute, StateVerify, StateExecute, StateVerify}
	for i := 0; i < 4000; i++ {
		if err := txn.SetState(states[i%len(states)]); err != nil {
			t.Fatal(err)
		}
	}
	stop.Store(true)
	wg.Wait()

	if torn.Load() != 0 {
		t.Fatalf("the journal was read half-written %d times out of %d reads", torn.Load(), reads.Load())
	}
}

func TestJournalLeavesNoTemporaryFile(t *testing.T) {
	txn := newTxn(t)
	for _, s := range []State{StateExecute, StateVerify, StateCommit} {
		if err := txn.SetState(s); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(txn.journalPath))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			t.Fatalf("unexpected file in the journal directory: %s", e.Name())
		}
	}
	b, err := os.ReadFile(txn.journalPath)
	if err != nil || !json.Valid(b) || !strings.Contains(string(b), `"COMMIT"`) {
		t.Fatalf("the journal should hold the last state: %s, %v", b, err)
	}
}
