package commands

import (
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunBranch executes the `vara branch` command.
// If name is empty, it lists branches.
func RunBranch(ctx *Context, name string) (string, error) {
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)

	if name == "" {
		list, err := resolver.List()
		if err != nil {
			return "", fmt.Errorf("failed to list branches: %v", err)
		}

		headTarget, _ := resolver.ResolveSymbolic("HEAD")

		var sb strings.Builder
		for _, ref := range list {
			if !strings.HasPrefix(ref.Name, "refs/heads/") {
				continue
			}
			branchName := strings.TrimPrefix(ref.Name, "refs/heads/")
			
			if headTarget == ref.Name {
				sb.WriteString(fmt.Sprintf("* %s\n", branchName))
			} else {
				sb.WriteString(fmt.Sprintf("  %s\n", branchName))
			}
		}
		return sb.String(), nil
	}

	// Create a new branch pointing to HEAD
	headCommit, err := resolver.Resolve("HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot create branch: HEAD is not a valid commit")
	}

	branchRef := "refs/heads/" + name
	err = resolver.Create(branchRef, headCommit)
	if err != nil {
		return "", fmt.Errorf("cannot create branch '%s': %v", name, err)
	}

	// Update reflog for the new branch
	author := "User <user@example.com>" // Hardcoded for now
	rm := reflog.NewManager(ctx.Repository.VaraDir)
	rm.Append(branchRef, types.CommitID{}, headCommit, author, "branch: Created from HEAD")

	return "", nil
}
