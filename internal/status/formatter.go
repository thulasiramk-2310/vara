package status

import (
	"fmt"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/color"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
)

// StatusResult cleanly separates scanner state from presentation.
type StatusResult struct {
	Clean      []string
	Modified   []string
	Staged     []string
	Deleted    []string
	Untracked  []string
	Conflicted []string
	// Merge, when non-nil, describes an in-progress merge surfaced from the
	// conflict sidecar (.vara/CONFLICTS) — the authoritative source that the
	// working-tree scanner cannot see (marker-less modify/delete and add/add
	// conflicts leave the worktree looking merely "modified" or clean).
	Merge *MergeStatus
}

// MergeStatus describes an in-progress merge for `vara status`.
type MergeStatus struct {
	InProgress bool
	Unmerged   []ConflictLine // conflicts still needing resolution
	Resolved   []string       // resolved paths staged for the merge commit
}

// ConflictLine is one unmerged path with its display label and short code,
// classified by conflict kind (both modified / both added / deleted by us|them).
type ConflictLine struct {
	Path  string
	Long  string // e.g. "both modified", "deleted by them"
	Short string // e.g. "UU", "UD" (git-style two-letter code)
}

// FromScanner builds a StatusResult from the output of the repository scanner.
func FromScanner(res *scanner.Result) *StatusResult {
	sr := &StatusResult{}
	for path, state := range res.Files {
		switch state {
		case scanner.StatusClean:
			sr.Clean = append(sr.Clean, path)
		case scanner.StatusModified:
			sr.Modified = append(sr.Modified, path)
		case scanner.StatusDeleted:
			sr.Deleted = append(sr.Deleted, path)
		case scanner.StatusUntracked:
			sr.Untracked = append(sr.Untracked, path)
		}
	}

	// Sort for deterministic output
	sort.Strings(sr.Clean)
	sort.Strings(sr.Modified)
	sort.Strings(sr.Staged)
	sort.Strings(sr.Deleted)
	sort.Strings(sr.Untracked)
	sort.Strings(sr.Conflicted)

	return sr
}

// FormatLong returns a human-readable status report with ANSI color when the
// output is a terminal. branch is the current branch name (may be empty).
func FormatLong(res *StatusResult, branch string) string {
	var sb strings.Builder

	if branch != "" {
		sb.WriteString(fmt.Sprintf("On branch %s\n", color.Bold(branch)))
	} else {
		sb.WriteString("HEAD detached\n")
	}

	mergeUnmerged, mergeResolved := 0, 0
	if res.Merge != nil {
		mergeUnmerged = len(res.Merge.Unmerged)
		mergeResolved = len(res.Merge.Resolved)
	}
	hasChanges := len(res.Modified)+len(res.Deleted)+len(res.Staged)+len(res.Untracked)+len(res.Conflicted)+mergeUnmerged+mergeResolved > 0

	// In-progress merge banner (sidecar-driven).
	if res.Merge != nil && res.Merge.InProgress {
		if mergeUnmerged > 0 {
			sb.WriteString("You have unmerged paths.\n")
			sb.WriteString("  (fix conflicts and run \"vara commit\")\n")
			sb.WriteString("  (use \"vara merge --abort\" to abort the merge)\n")
		} else {
			sb.WriteString("All conflicts fixed but you are still merging.\n")
			sb.WriteString("  (use \"vara commit\" to conclude the merge)\n")
		}
	}

	if mergeResolved > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.Bold("Resolved (staged for the merge commit):") + "\n\n")
		for _, p := range res.Merge.Resolved {
			sb.WriteString(fmt.Sprintf("        %s\n", color.Green("resolved:   "+p)))
		}
	}

	if mergeUnmerged > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.BoldRed("Unmerged paths:") + "\n")
		sb.WriteString("  (use \"vara resolve\", \"vara add\", or \"vara rm\" as appropriate)\n\n")
		for _, c := range res.Merge.Unmerged {
			sb.WriteString(fmt.Sprintf("        %s\n", color.BoldRed(c.Long+":   "+c.Path)))
		}
	}

	// Legacy/direct path: a caller that populated Conflicted without a Merge
	// overlay (e.g. the short-format golden path) still renders here.
	if res.Merge == nil && len(res.Conflicted) > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.BoldRed("Unresolved conflicts:") + "\n")
		sb.WriteString("  (fix conflicts then run \"vara commit\")\n\n")
		for _, p := range res.Conflicted {
			sb.WriteString(fmt.Sprintf("        %s\n", color.BoldRed("both modified:   "+p)))
		}
	}

	if len(res.Staged) > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.Bold("Changes staged for commit:") + "\n\n")
		for _, p := range res.Staged {
			sb.WriteString(fmt.Sprintf("        %s\n", color.Green("new file:   "+p)))
		}
	}

	if len(res.Modified) > 0 || len(res.Deleted) > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.Bold("Changes not staged for commit:") + "\n")
		sb.WriteString("  (use \"vara add <file>...\" to stage changes)\n\n")
		for _, p := range res.Modified {
			sb.WriteString(fmt.Sprintf("        %s\n", color.Yellow("modified:   "+p)))
		}
		for _, p := range res.Deleted {
			sb.WriteString(fmt.Sprintf("        %s\n", color.Red("deleted:    "+p)))
		}
	}

	if len(res.Untracked) > 0 {
		sb.WriteString("\n")
		sb.WriteString(color.Bold("Untracked files:") + "\n")
		sb.WriteString("  (use \"vara add <file>...\" to include in next commit)\n\n")
		for _, p := range res.Untracked {
			sb.WriteString(fmt.Sprintf("        %s\n", color.Red(p)))
		}
	}

	if !hasChanges {
		sb.WriteString("\n")
		sb.WriteString(color.Dim("nothing to commit, working tree clean") + "\n")
	}

	return sb.String()
}

// FormatShort returns the output in a concise, machine-readable format.
func FormatShort(res *StatusResult) string {
	var sb strings.Builder

	for _, p := range res.Staged {
		sb.WriteString(fmt.Sprintf("S  %s\n", p))
	}
	for _, p := range res.Modified {
		sb.WriteString(fmt.Sprintf(" M %s\n", p))
	}
	for _, p := range res.Deleted {
		sb.WriteString(fmt.Sprintf(" D %s\n", p))
	}
	for _, p := range res.Untracked {
		sb.WriteString(fmt.Sprintf("?? %s\n", p))
	}
	for _, p := range res.Conflicted {
		sb.WriteString(fmt.Sprintf("UU %s\n", p))
	}
	if res.Merge != nil {
		for _, c := range res.Merge.Unmerged {
			sb.WriteString(fmt.Sprintf("%s %s\n", c.Short, c.Path))
		}
		for _, p := range res.Merge.Resolved {
			sb.WriteString(fmt.Sprintf("M  %s\n", p)) // resolved, staged for the merge commit
		}
	}

	return sb.String()
}
