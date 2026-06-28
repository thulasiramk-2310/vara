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

	hasChanges := len(res.Modified)+len(res.Deleted)+len(res.Staged)+len(res.Untracked)+len(res.Conflicted) > 0

	if len(res.Conflicted) > 0 {
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

	return sb.String()
}
