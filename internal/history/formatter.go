package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// FormatShort formats a commit into a single line output (similar to git log --oneline).
func FormatShort(id types.CommitID, commit *object.Commit) string {
	msg := strings.Split(commit.Message, "\n")[0]
	return fmt.Sprintf("[%s] %s", id.String()[:7], msg)
}

// FormatFull formats a commit with full details.
func FormatFull(id types.CommitID, commit *object.Commit) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("commit %s\n", id.String()))
	if len(commit.Parents) > 0 {
		var pStrings []string
		for _, p := range commit.Parents {
			pStrings = append(pStrings, p.String()[:7])
		}
		sb.WriteString(fmt.Sprintf("parents: %s\n", strings.Join(pStrings, ", ")))
	}
	sb.WriteString(fmt.Sprintf("author: %s\n", commit.Author))
	
	t := time.Unix(commit.Timestamp, 0).UTC()
	sb.WriteString(fmt.Sprintf("date:   %s\n", t.Format(time.RFC1123Z)))
	sb.WriteString("\n")
	
	for _, line := range strings.Split(commit.Message, "\n") {
		sb.WriteString(fmt.Sprintf("    %s\n", line))
	}
	sb.WriteString("\n")
	
	return sb.String()
}
