// Package commands — remote operations (RFC-0014).
//
// This file implements `vara remote`, `clone`, `fetch`, `pull`, and `push`.
// It orchestrates internal/transport (the peer-independent protocol surface),
// pkg/config (remote definitions), pkg/refs (remote-tracking refs), and reuses
// the checkout machinery in switch.go for materializing working trees.
package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/transport"
	"github.com/thulasiramk-2310/vara/pkg/config"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/transfer"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// --- config helpers ---

func configPath(varaDir string) string {
	return filepath.Join(varaDir, "config")
}

func loadConfig(varaDir string) (*config.Config, error) {
	return config.Load(configPath(varaDir))
}

func saveConfig(varaDir string, c *config.Config) error {
	return c.Save(configPath(varaDir))
}

// --- vara remote ---

// RunRemote implements `vara remote`, `vara remote add <name> <url>`, and
// `vara remote remove <name>`.
func RunRemote(ctx *Context, args []string) (string, error) {
	cfg, err := loadConfig(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("remote: load config: %w", err)
	}

	if len(args) == 0 {
		var sb strings.Builder
		for _, r := range cfg.Remotes() {
			fmt.Fprintf(&sb, "%s\t%s\n", r.Name, r.URL)
		}
		return sb.String(), nil
	}

	switch args[0] {
	case "add":
		if len(args) != 3 {
			return "", fmt.Errorf("usage: vara remote add <name> <url>")
		}
		name, url := args[1], args[2]
		if err := refs.ValidateName(name); err != nil {
			return "", fmt.Errorf("invalid remote name: %w", err)
		}
		if _, exists := cfg.Remote(name); exists {
			return "", fmt.Errorf("remote '%s' already exists", name)
		}
		cfg.AddRemote(name, url)
		if err := saveConfig(ctx.Repository.VaraDir, cfg); err != nil {
			return "", err
		}
		return "", nil

	case "remove", "rm":
		if len(args) != 2 {
			return "", fmt.Errorf("usage: vara remote remove <name>")
		}
		if !cfg.RemoveRemote(args[1]) {
			return "", fmt.Errorf("no such remote: %s", args[1])
		}
		if err := saveConfig(ctx.Repository.VaraDir, cfg); err != nil {
			return "", err
		}
		return "", nil

	default:
		return "", fmt.Errorf("unknown remote subcommand '%s'", args[0])
	}
}

// --- vara clone ---

// RunClone implements `vara clone <url> [<dir>]` (RFC-0014 §9.1).
func RunClone(url, dir string) (string, error) {
	if dir == "" {
		dir = transport.SuggestDir(url)
	}
	if dir == "" || dir == "." {
		return "", fmt.Errorf("clone: could not infer target directory from %q; specify one", url)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("clone: destination %q already exists and is not empty", dir)
	}

	tr, err := transport.Open(url)
	if err != nil {
		return "", fmt.Errorf("clone: %w", err)
	}
	defer tr.Close()

	advs, err := tr.ListRefs()
	if err != nil {
		return "", fmt.Errorf("clone: list refs: %w", err)
	}

	// Remember whether the destination already existed so a failed clone rolls
	// back exactly what it created and nothing more.
	_, statErr := os.Stat(dir)
	dirPreexisted := statErr == nil

	repo, err := repository.Init(dir)
	if err != nil {
		return "", fmt.Errorf("clone: init: %w", err)
	}

	// Roll back a partial clone on any failure past this point: an interrupted
	// or failed transfer must never leave a half-built repository behind
	// (RFC-0014 §10). Set success=true on the return paths that complete.
	success := false
	defer func() {
		if success {
			return
		}
		if dirPreexisted {
			os.RemoveAll(repo.VaraDir) // dir was the user's; only drop our .vara
		} else {
			os.RemoveAll(dir)
		}
	}()

	ctx := &Context{Repository: repo, Index: index.New()}

	// Record the origin remote.
	cfg := config.New()
	cfg.AddRemote("origin", url)
	if err := saveConfig(repo.VaraDir, cfg); err != nil {
		return "", fmt.Errorf("clone: write config: %w", err)
	}

	if len(advs) == 0 {
		success = true
		return fmt.Sprintf("Cloned empty repository into %s\n", dir), nil
	}

	// Fetch the full closure of every advertised branch.
	wants := make([]types.CommitID, 0, len(advs))
	for _, a := range advs {
		wants = append(wants, a.Target)
	}
	stream, err := tr.FetchPack(wants, nil)
	if err != nil {
		return "", fmt.Errorf("clone: fetch: %w", err)
	}
	defer stream.Close()

	store := object.NewStore(repo.VaraDir)
	if _, err := transfer.ReadPack(store, stream); err != nil {
		return "", fmt.Errorf("clone: ingest: %w", err)
	}

	// Create remote-tracking refs for every advertised branch.
	resolver := refs.NewFSResolver(repo.VaraDir)
	for _, a := range advs {
		short := strings.TrimPrefix(a.Name, "refs/heads/")
		if err := resolver.Update("refs/remotes/origin/"+short, a.Target); err != nil {
			return "", fmt.Errorf("clone: set tracking ref: %w", err)
		}
	}

	// Pick the default branch from the remote HEAD, falling back to the first
	// advertised branch.
	headRef, _ := tr.HeadTarget()
	defBranch := strings.TrimPrefix(headRef, "refs/heads/")
	var defTarget types.CommitID
	found := false
	for _, a := range advs {
		if strings.TrimPrefix(a.Name, "refs/heads/") == defBranch {
			defTarget, found = a.Target, true
			break
		}
	}
	if !found {
		defBranch = strings.TrimPrefix(advs[0].Name, "refs/heads/")
		defTarget = advs[0].Target
	}

	if err := resolver.Update("refs/heads/"+defBranch, defTarget); err != nil {
		return "", fmt.Errorf("clone: create local branch: %w", err)
	}
	if err := resolver.Symbolic("HEAD", "refs/heads/"+defBranch); err != nil {
		return "", fmt.Errorf("clone: set HEAD: %w", err)
	}

	if err := checkoutCommit(ctx, defTarget); err != nil {
		return "", fmt.Errorf("clone: checkout: %w", err)
	}
	graphindex.Invalidate(repo.VaraDir)

	success = true
	return fmt.Sprintf("Cloned into %s (branch '%s' at %s)\n", dir, defBranch, defTarget.String()[:7]), nil
}

// --- vara fetch ---

// RunFetch implements `vara fetch <remote>` (RFC-0014 §9.2). It updates
// refs/remotes/<remote>/* and never touches local branches.
func RunFetch(ctx *Context, remoteName string) (string, error) {
	if remoteName == "" {
		remoteName = "origin"
	}
	cfg, err := loadConfig(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("fetch: load config: %w", err)
	}
	remote, ok := cfg.Remote(remoteName)
	if !ok {
		return "", fmt.Errorf("no such remote: %s", remoteName)
	}

	tr, err := transport.Open(remote.URL)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer tr.Close()

	advs, err := tr.ListRefs()
	if err != nil {
		return "", fmt.Errorf("fetch: list refs: %w", err)
	}
	if len(advs) == 0 {
		return "Already up to date.\n", nil
	}

	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	haves := localCommitIDs(resolver)

	wants := make([]types.CommitID, 0, len(advs))
	for _, a := range advs {
		wants = append(wants, a.Target)
	}

	stream, err := tr.FetchPack(wants, haves)
	if err != nil {
		return "", fmt.Errorf("fetch: pack: %w", err)
	}
	defer stream.Close()

	store := object.NewStore(ctx.Repository.VaraDir)
	if _, err := transfer.ReadPack(store, stream); err != nil {
		return "", fmt.Errorf("fetch: ingest: %w", err)
	}

	var sb strings.Builder
	for _, a := range advs {
		short := strings.TrimPrefix(a.Name, "refs/heads/")
		trackRef := fmt.Sprintf("refs/remotes/%s/%s", remoteName, short)
		old, _ := resolver.Resolve(trackRef)
		if old == a.Target {
			continue
		}
		if err := resolver.Update(trackRef, a.Target); err != nil {
			return "", fmt.Errorf("fetch: update %s: %w", trackRef, err)
		}
		if (old == types.CommitID{}) {
			fmt.Fprintf(&sb, "  * [new branch]      %s -> %s/%s\n", short, remoteName, short)
		} else {
			fmt.Fprintf(&sb, "  %s..%s  %s -> %s/%s\n",
				old.String()[:7], a.Target.String()[:7], short, remoteName, short)
		}
	}
	graphindex.Invalidate(ctx.Repository.VaraDir)

	if sb.Len() == 0 {
		return "Already up to date.\n", nil
	}
	return fmt.Sprintf("From %s\n%s", remote.URL, sb.String()), nil
}

// --- vara pull ---

// RunPull implements `vara pull <remote> [<branch>]` (RFC-0014 §9.3):
// fetch, then integrate refs/remotes/<remote>/<branch> into the current branch.
func RunPull(ctx *Context, remoteName, branch string) (string, error) {
	if remoteName == "" {
		remoteName = "origin"
	}
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)

	if branch == "" {
		headRef, err := resolver.ResolveSymbolic("HEAD")
		if err != nil {
			return "", fmt.Errorf("pull: detached HEAD; specify a branch")
		}
		branch = strings.TrimPrefix(headRef, "refs/heads/")
	}

	fetchOut, err := RunFetch(ctx, remoteName)
	if err != nil {
		return "", err
	}

	trackRef := fmt.Sprintf("refs/remotes/%s/%s", remoteName, branch)
	target, err := resolver.Resolve(trackRef)
	if err != nil {
		return "", fmt.Errorf("pull: remote branch '%s/%s' not found", remoteName, branch)
	}

	// Reload the index from disk: fetch may have run, and the merge pipeline
	// operates on ctx.Index.
	if idx := reloadIndex(ctx.Repository.VaraDir); idx != nil {
		ctx.Index = idx
	}

	mergeOut, err := MergeIntoHEAD(ctx, target, remoteName+"/"+branch)
	if err != nil {
		return "", err
	}
	return fetchOut + mergeOut, nil
}

// --- vara push ---

// RunPush implements `vara push <remote> <branch>` (RFC-0014 §9.4).
func RunPush(ctx *Context, remoteName, branch string, force bool) (string, error) {
	if remoteName == "" {
		remoteName = "origin"
	}
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)

	if branch == "" {
		headRef, err := resolver.ResolveSymbolic("HEAD")
		if err != nil {
			return "", fmt.Errorf("push: detached HEAD; specify a branch")
		}
		branch = strings.TrimPrefix(headRef, "refs/heads/")
	}

	localRef := "refs/heads/" + branch
	newID, err := resolver.Resolve(localRef)
	if err != nil {
		return "", fmt.Errorf("push: local branch '%s' not found", branch)
	}

	cfg, err := loadConfig(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("push: load config: %w", err)
	}
	remote, ok := cfg.Remote(remoteName)
	if !ok {
		return "", fmt.Errorf("no such remote: %s", remoteName)
	}

	tr, err := transport.Open(remote.URL)
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	defer tr.Close()

	// Discover the remote's current value for this branch.
	advs, err := tr.ListRefs()
	if err != nil {
		return "", fmt.Errorf("push: list refs: %w", err)
	}
	remoteRef := "refs/heads/" + branch
	var oldID types.CommitID
	haves := make([]types.CommitID, 0, len(advs))
	for _, a := range advs {
		haves = append(haves, a.Target)
		if a.Name == remoteRef {
			oldID = a.Target
		}
	}

	if oldID == newID {
		return "Everything up-to-date\n", nil
	}

	// Build the pack from OUR store: everything the remote lacks to reach
	// newID. Enumeration and encoding run against the local repository; the
	// transport only carries the finished stream to the peer.
	localStore := object.NewStore(ctx.Repository.VaraDir)
	ids, err := transfer.Enumerate(localStore, []types.CommitID{newID}, haves)
	if err != nil {
		return "", fmt.Errorf("push: enumerate: %w", err)
	}
	var packBuf bytes.Buffer
	if err := transfer.WritePack(localStore, ids, &packBuf); err != nil {
		return "", fmt.Errorf("push: build pack: %w", err)
	}

	results, err := tr.ReceivePack(&packBuf, []transport.RefUpdate{
		{Name: remoteRef, Old: oldID, New: newID, Force: force},
	})
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	r := results[0]
	if !r.OK {
		return "", fmt.Errorf("push rejected: %s\n"+
			"  (fetch and merge, or use --force to overwrite)", r.Reason)
	}

	// Mirror the pushed state locally as a remote-tracking ref.
	_ = resolver.Update(fmt.Sprintf("refs/remotes/%s/%s", remoteName, branch), newID)

	oldStr := "(new)"
	if (oldID != types.CommitID{}) {
		oldStr = oldID.String()[:7]
	}
	return fmt.Sprintf("To %s\n  %s..%s  %s -> %s\n",
		remote.URL, oldStr, newID.String()[:7], branch, branch), nil
}

// --- shared helpers ---

// checkoutCommit materializes the working tree and index from a commit,
// reusing the checkout machinery from switch.go.
func checkoutCommit(ctx *Context, commitID types.CommitID) error {
	store := object.NewStore(ctx.Repository.VaraDir)
	obj, err := store.Read(types.ObjectID(commitID))
	if err != nil {
		return fmt.Errorf("read commit %s: %w", commitID.String()[:7], err)
	}
	commit, ok := obj.(*object.Commit)
	if !ok {
		return fmt.Errorf("%s is not a commit", commitID.String()[:7])
	}
	newIdx := index.New()
	if err := checkoutTree(store, commit.TreeHash, ctx.Repository.RootDir, newIdx); err != nil {
		return err
	}
	ctx.Index = newIdx
	data, err := newIdx.Serialize()
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(ctx.Repository.VaraDir, "index"), data, 0644)
}

// localCommitIDs returns every commit ID currently referenced locally (heads
// and remote-tracking refs), used as haves during fetch negotiation.
func localCommitIDs(resolver *refs.FSResolver) []types.CommitID {
	list, err := resolver.List()
	if err != nil {
		return nil
	}
	seen := map[types.CommitID]bool{}
	var out []types.CommitID
	for _, r := range list {
		if !seen[r.ObjectID] {
			seen[r.ObjectID] = true
			out = append(out, r.ObjectID)
		}
	}
	return out
}

// reloadIndex reads .vara/index from disk, returning nil on any error.
func reloadIndex(varaDir string) *index.Index {
	data, err := os.ReadFile(filepath.Join(varaDir, "index"))
	if err != nil || len(data) == 0 {
		return index.New()
	}
	idx, err := index.Deserialize(data)
	if err != nil {
		return nil
	}
	return idx
}
