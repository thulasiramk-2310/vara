package commands

import (
	"fmt"

	"github.com/thulasiramk-2310/vara/internal/undo"
	"github.com/thulasiramk-2310/vara/pkg/object"
)

// RunUndo executes `vara undo` (RFC-0009 §6).
// Follows the three-layer recovery model: Journal → Reflog → Snapshot.
func RunUndo(ctx *Context) (string, error) {
	store := object.NewStore(ctx.Repository.VaraDir)

	result, err := undo.Undo(undo.Inputs{
		VaraDir: ctx.Repository.VaraDir,
		RootDir: ctx.Repository.RootDir,
		Index:   ctx.Index,
		Store:   store,
	})
	if err != nil {
		return "", fmt.Errorf("undo: %w", err)
	}

	return fmt.Sprintf("[layer %d] %s\n", result.Layer, result.Message), nil
}
