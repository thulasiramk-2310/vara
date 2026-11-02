package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeJournal(t *testing.T, varaDir, file, id string, state JournalState) {
	t.Helper()
	dir := filepath.Join(varaDir, "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("{\n  \"transaction\": %q,\n  \"state\": %q,\n  \"command\": \"merge\"\n}\n", id, state)
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Only transactions that never finished need recovery. A finished one succeeded, and
// a FAILED one was rolled back or already recovered; reporting either as pending made
// a single recovery block every later undo.
func TestScanJournalReturnsOnlyUnfinishedTransactions(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, "0001.json", "done", StateComplete)
	writeJournal(t, dir, "0002.json", "rolled-back", StateFailed)
	writeJournal(t, dir, "0003.json", "begun", StateBegin)
	writeJournal(t, dir, "0004.json", "executing", StateExecute)
	writeJournal(t, dir, "0005.json", "verifying", StateVerify)
	writeJournal(t, dir, "0006.json", "committing", StateCommit)
	if err := os.WriteFile(filepath.Join(dir, "journal", "0007.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal", "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ScanJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	want := []string{"begun", "executing", "verifying", "committing"} // oldest first
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("pending = %v, want %v", ids, want)
	}
}

func TestScanJournalWithNoJournalIsEmpty(t *testing.T) {
	got, err := ScanJournal(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("no journal folder: got %v, %v", got, err)
	}
}
