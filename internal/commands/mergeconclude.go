// Package commands — vara merge --continue.
//
// Continue concludes an in-progress merge as a two-parent merge commit, the
// counterpart to `vara merge --abort`. It is the Git-parity way to finish a
// conflicted merge once every conflict is resolved, so users are not left to
// discover that a plain `vara commit` is what seals the merge.
package commands

import (
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// RunMergeContinue concludes an in-progress merge. It refuses if no merge is in
// progress, or if any conflict is still unresolved (naming them, via the same
// sidecar-authoritative check `vara commit` uses). With an empty message it uses
// the conventional "Merge branch '<theirs>'", matching a clean `vara merge`.
func RunMergeContinue(ctx *Context, message string) (string, error) {
	varaDir := ctx.Repository.VaraDir
	if !recovery.MergeInProgress(varaDir) {
		return "", fmt.Errorf("no merge in progress; nothing to continue")
	}
	if unresolved := unresolvedConflicts(ctx); len(unresolved) > 0 {
		return "", fmt.Errorf(
			"cannot continue: unresolved conflicts in:\n\t%s\n\n"+
				"Resolve them with 'vara resolve', 'vara add', or 'vara rm', then run 'vara merge --continue'.",
			strings.Join(unresolved, "\n\t"))
	}

	if message == "" {
		message = defaultMergeMessage(varaDir)
	}

	// RunCommit performs the two-parent commit (MERGE_HEAD as second parent) and
	// clears MERGE_HEAD + the sidecar on success.
	commitID, err := RunCommit(ctx, message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[%s] %s\nMerge complete.\n", commitID.String()[:7], message), nil
}

// defaultMergeMessage builds the conventional merge message from the branch label
// recorded in the sidecar, falling back to a generic message.
func defaultMergeMessage(varaDir string) string {
	if st, ok, err := mergestate.Read(varaDir); err == nil && ok && st.TheirLabel != "" {
		return fmt.Sprintf("Merge branch '%s'", st.TheirLabel)
	}
	return "Merge commit"
}
