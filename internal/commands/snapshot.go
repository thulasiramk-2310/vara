// Package commands — vara snapshot (RFC-0009 §5).
//
// `vara snapshot list` shows the working-tree archives VARA saves before
// commands that rewrite files (switch, merge, undo). `vara snapshot restore
// <name>` writes one back. Restore changes files only: HEAD, branches and the
// index stay as they are, so `vara status` then shows what the snapshot brought
// back.
package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/undo"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

// RunSnapshot dispatches `vara snapshot <list|restore> [...]`.
func RunSnapshot(ctx *Context, args []string) (string, error) {
	if len(args) == 0 || args[0] == "list" {
		return snapshotList(ctx)
	}
	if args[0] == "restore" {
		var name string
		force := false
		for _, a := range args[1:] {
			if a == "--force" {
				force = true
			} else if name == "" {
				name = a
			} else {
				return "", fmt.Errorf("restore takes one snapshot name")
			}
		}
		if name == "" {
			return "", fmt.Errorf("restore needs a snapshot name; see 'vara snapshot list'")
		}
		return snapshotRestore(ctx, name, force)
	}
	return "", fmt.Errorf("unknown subcommand %q (want list|restore)", args[0])
}

func snapshotList(ctx *Context) (string, error) {
	names, err := snapshot.List(ctx.Repository.VaraDir)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "No snapshots yet. VARA saves one before switch, merge and undo.\n", nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // newest first
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		if when, command, ok := describeSnapshot(n); ok {
			fmt.Fprintf(&b, "  (%s, before %s)", when, command)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// describeSnapshot reads snap-YYYYMMDD-HHMMSS-<command>-<commit>.tar.zst.
func describeSnapshot(name string) (when, command string, ok bool) {
	base := strings.TrimSuffix(strings.TrimPrefix(name, "snap-"), ".tar.zst")
	parts := strings.Split(base, "-")
	if len(parts) < 4 || len(parts[0]) != 8 || len(parts[1]) != 6 {
		return "", "", false
	}
	d, t := parts[0], parts[1]
	when = fmt.Sprintf("%s-%s-%s %s:%s:%s", d[:4], d[4:6], d[6:], t[:2], t[2:4], t[4:])
	command = strings.Join(parts[2:len(parts)-1], "-")
	return when, command, true
}

func snapshotRestore(ctx *Context, name string, force bool) (string, error) {
	names, err := snapshot.List(ctx.Repository.VaraDir)
	if err != nil {
		return "", err
	}
	var match []string
	for _, n := range names {
		if n == name || strings.HasPrefix(n, name) {
			match = append(match, n)
		}
	}
	switch {
	case len(match) == 0:
		return "", fmt.Errorf("no snapshot named %q; see 'vara snapshot list'", name)
	case len(match) > 1 && !contains(match, name):
		return "", fmt.Errorf("%q matches %d snapshots; give more of the name", name, len(match))
	case contains(match, name):
		match = []string{name}
	}

	dirty, err := uncommittedPaths(ctx)
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 && !force {
		return "", fmt.Errorf("you have uncommitted changes that the snapshot may overwrite:\n\n  %s\n\n"+
			"Commit them first, or add --force (they are saved in a new snapshot first).",
			strings.Join(dirty, "\n  "))
	}
	if err := snapshotBefore(ctx, "restore", len(dirty) > 0); err != nil {
		return "", err
	}
	n, err := undo.RestoreSnapshot(ctx.Repository.RootDir, filepath.Join(ctx.Repository.VaraDir, "snapshots", match[0]))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Restored %d file(s) from %s.\nHEAD and branches are unchanged; 'vara status' shows what differs.\n", n, match[0]), nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
