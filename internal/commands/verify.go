package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// ErrCorruption is returned with the report when verify finds a damaged repository, so that
// callers can print the report and still exit non-zero: a health check that exits 0 on a
// corrupted repository is worse than none.
var ErrCorruption = errors.New("repository is corrupted")

// RunVerify executes `vara verify` and returns a human-readable integrity report. When the
// repository is damaged the report is returned together with ErrCorruption.
func RunVerify(ctx *Context) (string, error) {
	report, err := verify.Verify(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("verify: %w", err)
	}
	out := formatReport(report)
	if !report.Healthy {
		return out, ErrCorruption
	}
	return out, nil
}

func formatReport(r verify.Report) string {
	var sb strings.Builder
	sb.WriteString("Repository Integrity Report\n")
	sb.WriteString(strings.Repeat("─", 40) + "\n")

	writeCategory(&sb, "Objects", r.Objects)
	writeCategory(&sb, "Trees", r.Trees)
	writeCategory(&sb, "Commits", r.Commits)
	writeCategory(&sb, "Refs", r.Refs)
	writeCategory(&sb, "Index", r.Index)
	writeCategory(&sb, "Journal", r.Journal)
	writeCategory(&sb, "Snapshots", r.Snapshots)
	writeCategory(&sb, "Graph (DAG)", r.Graph)

	sb.WriteString(strings.Repeat("─", 40) + "\n")
	if r.Healthy {
		sb.WriteString("Result    Repository Healthy\n")
	} else {
		sb.WriteString("Result    CORRUPTION DETECTED\n")
	}
	return sb.String()
}

func writeCategory(sb *strings.Builder, name string, c verify.CategoryResult) {
	if len(c.Errors) == 0 && c.Verified == 0 {
		// Nothing was in this category, so nothing was checked: do not tick it.
		sb.WriteString(fmt.Sprintf("%-12s - nothing to check\n", name))
	} else if len(c.Errors) == 0 {
		sb.WriteString(fmt.Sprintf("%-12s ✔ %d verified\n", name, c.Verified))
	} else {
		sb.WriteString(fmt.Sprintf("%-12s ✘ %d verified, %d error(s)\n", name, c.Verified, len(c.Errors)))
		for _, e := range c.Errors {
			sb.WriteString(fmt.Sprintf("             %s: %s\n", e.Name, e.Problem))
		}
	}
}
