package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/status"
	"github.com/thulasiramk-2310/vara/internal/undo"
	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

// RunUndo executes `vara undo` (RFC-0009 §6).
// Follows the three-layer recovery model: Journal → Reflog → Snapshot.
//
// Undo rewrites tracked files, so uncommitted edits to them would otherwise be
// lost with no way back. It refuses while there are any, unless force is set,
// and it always snapshots the working tree first (RFC-0009 §5), so the changes
// survive in .vara/snapshots even when forced.
func RunUndo(ctx *Context, force bool) (string, error) {
	dirty, err := uncommittedPaths(ctx)
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 && !force {
		return "", fmt.Errorf("you have uncommitted changes that undo would overwrite:\n\n  %s\n\n"+
			"Commit them first, or run 'vara undo --force' (they are saved in .vara/snapshots first).",
			strings.Join(dirty, "\n  "))
	}
	if err := snapshotBefore(ctx, "undo", len(dirty) > 0); err != nil {
		return "", err
	}

	store := object.NewStore(ctx.Repository.VaraDir)
	result, err := undo.Undo(undo.Inputs{
		VaraDir: ctx.Repository.VaraDir,
		RootDir: ctx.Repository.RootDir,
		Index:   ctx.Index,
		Store:   store,
		Actor:   reflogActor(ctx.Repository.VaraDir),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("[layer %d] %s\n", result.Layer, result.Message), nil
}

// uncommittedPaths lists tracked files whose changes are not committed: staged,
// modified or deleted. Untracked files are not included.
func uncommittedPaths(ctx *Context) ([]string, error) {
	wt, err := worktree.New(ctx.Repository.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to init worktree: %v", err)
	}
	res, err := scanner.New(ctx.Index).Scan(wt)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %v", err)
	}
	sr := status.FromScanner(res)
	var dirty []string
	dirty = append(dirty, sr.Staged...)
	dirty = append(dirty, sr.Modified...)
	dirty = append(dirty, sr.Deleted...)
	return dirty, nil
}

// snapshotBefore saves the working tree before a command that rewrites it
// (RFC-0009 §5). When there are uncommitted changes the snapshot is their only
// copy, so failing to write it stops the command; otherwise it only warns.
func snapshotBefore(ctx *Context, command string, mustKeep bool) error {
	headHex := strings.Repeat("0", 64)
	if cid, cidErr := refs.NewFSResolver(ctx.Repository.VaraDir).Resolve("HEAD"); cidErr == nil {
		headHex = cid.String()
	}
	if _, snapErr := snapshot.Create(ctx.Repository.VaraDir, ctx.Repository.RootDir, command, headHex); snapErr != nil {
		if mustKeep {
			return fmt.Errorf("could not save a snapshot of your changes, nothing was changed: %v", snapErr)
		}
		fmt.Fprintf(os.Stderr, "vara: warning: snapshot failed: %v\n", snapErr)
	}
	return nil
}
