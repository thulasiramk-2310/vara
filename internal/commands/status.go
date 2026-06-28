package commands

import (
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/status"
	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
)

// RunStatus executes the `vara status` pipeline.
func RunStatus(ctx *Context) (string, error) {
	wt, err := worktree.New(ctx.Repository.RootDir)
	if err != nil {
		return "", fmt.Errorf("failed to init worktree: %v", err)
	}

	s := scanner.New(ctx.Index)
	res, err := s.Scan(wt)
	if err != nil {
		return "", fmt.Errorf("scan failed: %v", err)
	}

	sr := status.FromScanner(res)
	branch := currentBranch(ctx)
	return status.FormatLong(sr, branch), nil
}

// currentBranch returns the name of the current branch, or empty string if
// HEAD is detached or the ref cannot be read.
func currentBranch(ctx *Context) string {
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	target, err := resolver.ResolveSymbolic("HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(target, "refs/heads/")
}
