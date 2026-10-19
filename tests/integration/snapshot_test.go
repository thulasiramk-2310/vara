package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

// TestSnapshotRestoreBringsBackWorkSavedByForcedUndo closes the loop of
// `vara undo --force`: the edit it overwrote can be restored.
func TestSnapshotRestoreBringsBackWorkSavedByForcedUndo(t *testing.T) {
	ctx, dir := cleanMerge(t)
	writeFile(t, dir, "a.txt", "hello\nfeature line\nMY UNSAVED WORK\n")
	if _, err := commands.RunUndo(ctx, true); err != nil {
		t.Fatalf("undo --force: %v", err)
	}
	snaps, _ := snapshot.List(ctx.Repository.VaraDir)
	var undoSnap string
	for _, s := range snaps {
		if strings.Contains(s, "-undo-") {
			undoSnap = s
		}
	}
	if undoSnap == "" {
		t.Fatalf("no undo snapshot in %v", snaps)
	}

	list, err := commands.RunSnapshot(reloadCtx(t, ctx), []string{"list"})
	if err != nil || !strings.Contains(list, undoSnap) || !strings.Contains(list, "before undo") {
		t.Fatalf("list = %q, %v", list, err)
	}
	out, err := commands.RunSnapshot(reloadCtx(t, ctx), []string{"restore", undoSnap})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(out, "Restored") {
		t.Fatalf("restore output = %q", out)
	}
	if got := readFile(dir, "a.txt"); !strings.Contains(got, "MY UNSAVED WORK") {
		t.Fatalf("a.txt after restore = %q", got)
	}
}

// TestSnapshotRestoreRefusesToOverwriteUncommittedWork mirrors undo's guard.
func TestSnapshotRestoreRefusesToOverwriteUncommittedWork(t *testing.T) {
	ctx, dir := cleanMerge(t)
	if _, err := commands.RunUndo(ctx, false); err != nil {
		t.Fatalf("undo: %v", err)
	}
	snaps, _ := snapshot.List(ctx.Repository.VaraDir)
	writeFile(t, dir, "a.txt", "EDITED\n")
	_, err := commands.RunSnapshot(reloadCtx(t, ctx), []string{"restore", snaps[len(snaps)-1]})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("restore over an edit: err = %v, want a refusal", err)
	}
	if got := readFile(dir, "a.txt"); got != "EDITED\n" {
		t.Fatalf("refused restore changed a.txt to %q", got)
	}
}

func TestSnapshotRestoreNeedsAKnownName(t *testing.T) {
	ctx, _ := cleanMerge(t)
	if _, err := commands.RunSnapshot(ctx, []string{"restore", "snap-nope"}); err == nil {
		t.Fatal("restoring an unknown snapshot should fail")
	}
	if _, err := commands.RunSnapshot(ctx, []string{"restore"}); err == nil {
		t.Fatal("restore without a name should fail")
	}
}
