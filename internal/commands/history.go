package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunHistory executes `vara history` (RFC-0012 §2).
//
// Fast path (RFC-0013): load graph.idx, walk in-memory — zero object-store reads.
// Slow path (fallback): build graph.idx from the object store then use fast path.
func RunHistory(ctx *Context) (string, error) {
	headID, err := resolveHEAD(ctx)
	if err != nil {
		return "", err
	}

	store := object.NewStore(ctx.Repository.VaraDir)

	idx, err := graphindex.LoadOrBuild(ctx.Repository.VaraDir, store)
	if err != nil {
		// Final fallback: raw BFS (pre-RFC-0013 behaviour, handles corrupt index).
		return historyFromStore(ctx.Repository.VaraDir, headID, store)
	}

	return historyFromIndex(idx, headID)
}

// historyFromIndex traverses the graph index in-memory starting at headID.
// Commits are returned newest-first: higher generation first, then newer timestamp.
func historyFromIndex(idx *graphindex.Index, headID types.CommitID) (string, error) {
	visited := make(map[types.CommitID]bool)
	queue := []types.CommitID{headID}
	visited[headID] = true
	var ordered []*graphindex.Entry

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		e := idx.Lookup(id)
		if e == nil {
			continue
		}
		ordered = append(ordered, e)
		for _, p := range e.Parents {
			if !visited[p] {
				visited[p] = true
				queue = append(queue, p)
			}
		}
	}

	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Generation != ordered[j].Generation {
			return ordered[i].Generation > ordered[j].Generation
		}
		return ordered[i].Timestamp > ordered[j].Timestamp
	})

	var sb strings.Builder
	for _, e := range ordered {
		sb.WriteString(formatIndexEntry(e))
	}
	return sb.String(), nil
}

// historyFromStore is the pre-RFC-0013 fallback: walks commit objects directly.
func historyFromStore(_ string, headID types.CommitID, store *object.Store) (string, error) {
	visited := make(map[types.CommitID]bool)
	queue := []types.CommitID{headID}
	visited[headID] = true
	var sb strings.Builder

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		obj, err := store.Read(types.ObjectID(currID))
		if err != nil {
			return "", fmt.Errorf("history: read %s: %w", currID.String()[:7], err)
		}
		c, ok := obj.(*object.Commit)
		if !ok {
			return "", fmt.Errorf("history: %s is not a commit", currID.String()[:7])
		}
		sb.WriteString(formatCommit(currID, c))
		for _, p := range c.Parents {
			if !visited[p] {
				visited[p] = true
				queue = append(queue, p)
			}
		}
	}
	return sb.String(), nil
}

// resolveHEAD resolves HEAD to a CommitID, following symbolic refs.
func resolveHEAD(ctx *Context) (types.CommitID, error) {
	headPath := filepath.Join(ctx.Repository.VaraDir, "HEAD")
	headData, err := os.ReadFile(headPath)
	if err != nil || len(headData) == 0 {
		return types.CommitID{}, fmt.Errorf("repository has no commits yet")
	}
	content := strings.TrimSpace(string(headData))
	if strings.HasPrefix(content, "ref: ") {
		refPath := strings.TrimPrefix(content, "ref: ")
		refData, err := os.ReadFile(filepath.Join(ctx.Repository.VaraDir, filepath.FromSlash(refPath)))
		if err != nil || len(refData) < 64 {
			return types.CommitID{}, fmt.Errorf("repository has no commits yet")
		}
		base, err := types.ParseHex(strings.TrimSpace(string(refData)))
		if err != nil {
			return types.CommitID{}, fmt.Errorf("invalid commit ID: %w", err)
		}
		return types.CommitID(base), nil
	}
	if len(content) >= 64 {
		base, err := types.ParseHex(content[:64])
		if err != nil {
			return types.CommitID{}, fmt.Errorf("invalid HEAD: %w", err)
		}
		return types.CommitID(base), nil
	}
	return types.CommitID{}, fmt.Errorf("repository has no commits yet")
}

func formatIndexEntry(e *graphindex.Entry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("commit %s\n", e.ID.String()))
	if len(e.Parents) > 0 {
		parts := make([]string, len(e.Parents))
		for i, p := range e.Parents {
			parts[i] = p.String()[:7]
		}
		sb.WriteString(fmt.Sprintf("parents: %s\n", strings.Join(parts, ", ")))
	}
	sb.WriteString(fmt.Sprintf("author: %s\n", e.Author))
	sb.WriteString(fmt.Sprintf("date:   %s\n", time.Unix(e.Timestamp, 0).UTC().Format(time.RFC1123Z)))
	sb.WriteString("\n")
	for _, line := range strings.Split(e.Message, "\n") {
		sb.WriteString(fmt.Sprintf("    %s\n", line))
	}
	sb.WriteString("\n")
	return sb.String()
}

func formatCommit(id types.CommitID, c *object.Commit) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("commit %s\n", id.String()))
	if len(c.Parents) > 0 {
		parts := make([]string, len(c.Parents))
		for i, p := range c.Parents {
			parts[i] = p.String()[:7]
		}
		sb.WriteString(fmt.Sprintf("parents: %s\n", strings.Join(parts, ", ")))
	}
	sb.WriteString(fmt.Sprintf("author: %s\n", c.Author))
	sb.WriteString(fmt.Sprintf("date:   %s\n", time.Unix(c.Timestamp, 0).UTC().Format(time.RFC1123Z)))
	sb.WriteString("\n")
	for _, line := range strings.Split(c.Message, "\n") {
		sb.WriteString(fmt.Sprintf("    %s\n", line))
	}
	sb.WriteString("\n")
	return sb.String()
}
