package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/locking"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

func twoCommits(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	makeCommit(t, ctx, "A")
	writeFile(t, dir, "b.txt", "b\n")
	makeCommit(t, ctx, "B")
	return ctx, dir
}

func plantJournal(t *testing.T, ctx *commands.Context, state string) {
	t.Helper()
	dir := filepath.Join(ctx.Repository.VaraDir, "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\n  \"transaction\": \"txn-crashed\",\n  \"state\": \"" + state + "\",\n  \"command\": \"merge\"\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "0001.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Layer 1, then layer 2: an interrupted transaction is cleared first, and the next
// undo must go on to undo the last commit. (It used to find the entry it had just
// marked FAILED, and repeat layer 1 for ever.)
func TestUndoAfterCrashRecoveryGoesOnToTheReflog(t *testing.T) {
	ctx, dir := twoCommits(t)
	plantJournal(t, ctx, "EXECUTE")

	out, err := commands.RunUndo(ctx, true)
	if err != nil || !strings.Contains(out, "layer 1") || !strings.Contains(out, "txn-crashed") {
		t.Fatalf("first undo should recover the transaction: %v %q", err, out)
	}
	if !fileExists(dir, "b.txt") {
		t.Fatal("crash recovery must not touch the files")
	}

	out, err = commands.RunUndo(reloadCtx(t, ctx), true)
	if err != nil || !strings.Contains(out, "layer 2") {
		t.Fatalf("second undo should undo the commit (layer 2): %v %q", err, out)
	}
	if fileExists(dir, "b.txt") {
		t.Fatal("undoing commit B should remove b.txt")
	}
	if !fileExists(dir, "a.txt") {
		t.Fatal("commit A's file must remain")
	}
}

// A transaction that was rolled back leaves a FAILED entry. It is finished business.
func TestARolledBackTransactionDoesNotBlockUndo(t *testing.T) {
	ctx, dir := twoCommits(t)
	txn, err := transaction.Begin(ctx.Repository.VaraDir, "merge", locking.NameRefs)
	if err != nil {
		t.Fatal(err)
	}
	txn.Rollback()

	out, err := commands.RunUndo(reloadCtx(t, ctx), true)
	if err != nil || !strings.Contains(out, "layer 2") {
		t.Fatalf("undo after a rollback should reach the reflog: %v %q", err, out)
	}
	if fileExists(dir, "b.txt") {
		t.Fatal("commit B should have been undone")
	}
}

// Layer 3: no commits, no history, only a saved snapshot of the working folder.
func TestUndoRestoresASnapshotWhenNothingElseApplies(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "notes.txt", "version one\n")
	if _, err := snapshot.Create(ctx.Repository.VaraDir, ctx.Repository.RootDir, "test", ""); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	writeFile(t, dir, "notes.txt", "version two, which should be undone\n")

	out, err := commands.RunUndo(reloadCtx(t, ctx), true)
	if err != nil || !strings.Contains(out, "layer 3") {
		t.Fatalf("with only a snapshot to go on, undo should use layer 3: %v %q", err, out)
	}
	if got := readFile(dir, "notes.txt"); got != "version one\n" {
		t.Fatalf("snapshot restore: notes.txt = %q", got)
	}
}

func TestUndoWithNothingToUndoSaysSo(t *testing.T) {
	ctx, _ := setupRepo(t)
	if _, err := commands.RunUndo(ctx, true); err == nil || !strings.Contains(err.Error(), "nothing to undo") {
		t.Fatalf("an empty repository has nothing to undo: %v", err)
	}
}
