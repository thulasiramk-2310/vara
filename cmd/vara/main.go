package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
)

// version is the release version. The fallback below is the last tagged
// release; release builds override it via -ldflags "-X main.version=<tag>"
// (see .goreleaser.yaml), so a downloaded binary reports its exact release.
var version = "0.4.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	// Top-level help / version flags
	switch cmd {
	case "--help", "-h":
		if len(rest) > 0 {
			printCommandHelp(rest[0])
		} else {
			printUsage()
		}
		return
	case "help":
		if len(rest) > 0 {
			printCommandHelp(rest[0])
		} else {
			printUsage()
		}
		return
	case "--version", "version":
		fmt.Printf("vara version %s\n", version)
		return
	}

	// Per-command --help/-h flag
	if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
		printCommandHelp(cmd)
		return
	}

	switch cmd {
	case "init":
		path := "."
		if len(rest) > 0 {
			path = rest[0]
		}
		repo, err := repository.Init(path)
		if err != nil {
			die("init: %v", err)
		}
		fmt.Printf("Initialized empty VARA repository in %s\n", repo.VaraDir)

	case "add":
		ctx := mustCtx(true)
		if err := commands.RunAdd(ctx, rest); err != nil {
			die("%v", err)
		}

	case "rm":
		ctx := mustCtx(true)
		out, err := commands.RunRm(ctx, rest)
		if err != nil {
			die("rm: %v", err)
		}
		if out != "" {
			fmt.Print(out)
		}

	case "status":
		ctx := mustCtx(true)
		out, err := commands.RunStatus(ctx)
		if err != nil {
			die("status: %v", err)
		}
		fmt.Print(out)

	case "commit":
		if len(rest) < 2 || rest[0] != "-m" {
			fmt.Fprintln(os.Stderr, "usage: vara commit -m \"<message>\"")
			os.Exit(1)
		}
		ctx := mustCtx(true)
		id, err := commands.RunCommit(ctx, rest[1])
		if err != nil {
			die("commit: %v", err)
		}
		fmt.Printf("[%s] %s\n", id.String()[:7], rest[1])

	case "history", "log":
		ctx := mustCtx(true)
		out, err := commands.RunHistory(ctx)
		if err != nil {
			die("history: %v", err)
		}
		fmt.Print(out)

	case "branch":
		ctx := mustCtx(false)
		name := ""
		if len(rest) > 0 {
			name = rest[0]
		}
		out, err := commands.RunBranch(ctx, name)
		if err != nil {
			die("branch: %v", err)
		}
		if out != "" {
			fmt.Print(out)
		}

	case "switch":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vara switch <branch>")
			os.Exit(1)
		}
		ctx := mustCtx(true)
		out, err := commands.RunSwitch(ctx, rest[0])
		if err != nil {
			die("switch: %v", err)
		}
		if out != "" {
			fmt.Print(out)
		}

	case "merge":
		// `vara merge --abort` undoes an in-progress conflicted merge.
		if len(rest) == 1 && (rest[0] == "--abort" || rest[0] == "--cancel") {
			ctx := mustCtx(true)
			out, err := commands.RunMergeAbort(ctx)
			if err != nil {
				die("merge --abort: %v", err)
			}
			if out != "" {
				fmt.Print(out)
			}
			break
		}
		// `vara merge --continue [-m <msg>]` concludes a resolved conflicted merge.
		if len(rest) >= 1 && rest[0] == "--continue" {
			msg, err := parseMergeContinueFlags(rest[1:])
			if err != nil {
				die("merge --continue: %v", err)
			}
			ctx := mustCtx(true)
			out, err := commands.RunMergeContinue(ctx, msg)
			if err != nil {
				die("merge --continue: %v", err)
			}
			if out != "" {
				fmt.Print(out)
			}
			break
		}
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vara merge <branch>  (or: vara merge --abort | --continue)")
			os.Exit(1)
		}
		ctx := mustCtx(true)
		out, err := commands.RunMerge(ctx, rest[0])
		if err != nil {
			die("merge: %v", err)
		}
		if out != "" {
			fmt.Print(out)
		}

	case "resolve":
		ctx := mustCtx(true)
		ra, err := parseResolveFlags(rest)
		if err != nil {
			die("resolve: %v", err)
		}
		out, err := commands.RunResolve(ctx, ra)
		if err != nil {
			die("resolve: %v", err)
		}
		fmt.Print(out)

	case "diff":
		ctx := mustCtx(true)
		da, err := parseDiffFlags(rest)
		if err != nil {
			die("diff: %v", err)
		}
		out, err := commands.RunDiff(ctx, da)
		if err != nil {
			die("diff: %v", err)
		}
		fmt.Print(out)

	case "undo":
		ctx := mustCtx(true)
		out, err := commands.RunUndo(ctx)
		if err != nil {
			die("undo: %v", err)
		}
		if out != "" {
			fmt.Print(out)
		}

	case "verify":
		ctx := mustCtx(false)
		out, err := commands.RunVerify(ctx)
		if err != nil {
			die("verify: %v", err)
		}
		fmt.Print(out)

	case "remote":
		ctx := mustCtx(false)
		out, err := commands.RunRemote(ctx, rest)
		if err != nil {
			die("remote: %v", err)
		}
		fmt.Print(out)

	case "clone":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vara clone <url> [<directory>]")
			os.Exit(1)
		}
		dir := ""
		if len(rest) > 1 {
			dir = rest[1]
		}
		out, err := commands.RunClone(rest[0], dir)
		if err != nil {
			die("clone: %v", err)
		}
		fmt.Print(out)

	case "fetch":
		ctx := mustCtx(false)
		remote := ""
		if len(rest) > 0 {
			remote = rest[0]
		}
		out, err := commands.RunFetch(ctx, remote)
		if err != nil {
			die("fetch: %v", err)
		}
		fmt.Print(out)

	case "pull":
		ctx := mustCtx(true)
		remote, branch := "", ""
		if len(rest) > 0 {
			remote = rest[0]
		}
		if len(rest) > 1 {
			branch = rest[1]
		}
		out, err := commands.RunPull(ctx, remote, branch)
		if err != nil {
			die("pull: %v", err)
		}
		fmt.Print(out)

	case "gc":
		ctx := mustCtx(false)
		apply := true
		for _, a := range rest {
			if a == "--dry-run" || a == "-n" {
				apply = false
			}
		}
		out, err := commands.RunGC(ctx, apply)
		if err != nil {
			die("gc: %v", err)
		}
		fmt.Print(out)

	case "push":
		ctx := mustCtx(false)
		force := false
		var pos []string
		for _, a := range rest {
			if a == "--force" || a == "-f" {
				force = true
				continue
			}
			pos = append(pos, a)
		}
		remote, branch := "", ""
		if len(pos) > 0 {
			remote = pos[0]
		}
		if len(pos) > 1 {
			branch = pos[1]
		}
		out, err := commands.RunPush(ctx, remote, branch, force)
		if err != nil {
			die("%v", err)
		}
		fmt.Print(out)

	case "serve":
		addr, root := ":8080", "."
		cfg := commands.ServeConfig{Basic: map[string]string{}, Bearer: map[string]string{}}
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--addr", "-a":
				if i+1 < len(rest) {
					addr = rest[i+1]
					i++
				}
			case "--root", "-r":
				if i+1 < len(rest) {
					root = rest[i+1]
					i++
				}
			case "--policy", "-p":
				if i+1 < len(rest) {
					cfg.PolicyDir = rest[i+1]
					i++
				}
			case "--meta", "-m":
				if i+1 < len(rest) {
					cfg.MetaDir = rest[i+1]
					i++
				}
			case "--accounts":
				if i+1 < len(rest) {
					cfg.AccountsDir = rest[i+1]
					i++
				}
			case "--hub":
				if i+1 < len(rest) {
					cfg.HubDir = rest[i+1]
					i++
				}
			case "--basic":
				if i+1 < len(rest) {
					user, secret, ok := splitCred(rest[i+1])
					if !ok {
						die("serve: --basic expects user:secret")
					}
					cfg.Basic[user] = secret
					i++
				}
			case "--bearer":
				if i+1 < len(rest) {
					token, subject, ok := splitCred(rest[i+1])
					if !ok {
						die("serve: --bearer expects token:subject")
					}
					cfg.Bearer[token] = subject
					i++
				}
			}
		}
		if err := commands.RunServe(addr, root, cfg); err != nil {
			die("%v", err)
		}

	case "repo":
		var cfg commands.RepoConfig
		var args []string
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--basic":
				if i+1 < len(rest) {
					cfg.Basic = rest[i+1]
					i++
				}
			case "--bearer":
				if i+1 < len(rest) {
					cfg.Bearer = rest[i+1]
					i++
				}
			default:
				args = append(args, rest[i])
			}
		}
		if err := commands.RunRepo(args, cfg); err != nil {
			die("%v", err)
		}

	case "doctor":
		acfg, args := parseAuthFlags(rest)
		// RunDoctor prints its own report; a non-nil error means a check failed,
		// so exit non-zero quietly (no "vara:" prefix over the printed report).
		if err := commands.RunDoctor(args, version, acfg); err != nil {
			os.Exit(1)
		}

	case "identity":
		sub := ""
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			sub = rest[0]
			rest = rest[1:]
		}
		ia := parseIdentityFlags(rest)
		if err := commands.RunIdentity(sub, ia); err != nil {
			die("%v", err)
		}

	case "login", "logout", "token", "whoami":
		acfg, args := parseAuthFlags(rest)
		var err error
		switch cmd {
		case "login":
			err = commands.RunLogin(args, acfg)
		case "logout":
			err = commands.RunLogout(args, acfg)
		case "token":
			err = commands.RunToken(args, acfg)
		case "whoami":
			err = commands.RunWhoami(args, acfg)
		}
		if err != nil {
			die("%v", err)
		}

	case "account":
		// On-host bootstrap mode (filesystem, no auth) is selected by --accounts;
		// otherwise this is the HTTP account-admin client.
		if containsFlag(rest, "--accounts") {
			if err := commands.RunAccountBootstrap(rest); err != nil {
				die("%v", err)
			}
		} else {
			acfg, args := parseAuthFlags(rest)
			if err := commands.RunAccount(args, acfg); err != nil {
				die("%v", err)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "vara: '%s' is not a vara command\n\nRun 'vara --help' to see available commands.\n", cmd)
		os.Exit(1)
	}
}

// mustCtx builds a command context, discovering the repository and loading the
// index. It exits with an error message if either step fails.
func mustCtx(needIndex bool) *commands.Context {
	repo, err := repository.Discover(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var idx *index.Index
	if needIndex {
		idx = loadIndex(repo.VaraDir)
	} else {
		idx = index.New()
	}
	return &commands.Context{Repository: repo, Index: idx}
}

// loadIndex reads and deserializes .vara/index. Returns an empty index if the
// file does not exist yet. Exits on deserialization failure (corrupt index).
func loadIndex(varaDir string) *index.Index {
	data, err := os.ReadFile(filepath.Join(varaDir, "index"))
	if err != nil {
		if os.IsNotExist(err) {
			return index.New()
		}
		fmt.Fprintf(os.Stderr, "vara: cannot read index: %v\n", err)
		os.Exit(1)
	}
	if len(data) == 0 {
		return index.New()
	}
	idx, err := index.Deserialize(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vara: corrupt index: %v\n", err)
		os.Exit(1)
	}
	return idx
}

// splitCred splits a "key:value" credential flag argument at the first colon.
func splitCred(s string) (key, value string, ok bool) {
	return strings.Cut(s, ":")
}

// containsFlag reports whether flag appears in args.
func containsFlag(args []string, flag string) bool {
	return slices.Contains(args, flag)
}

// parseAuthFlags pulls --basic/--bearer/--password out of args (RFC-0020 client
// commands) and returns the credential config plus the remaining positionals.
func parseAuthFlags(rest []string) (commands.AuthConfig, []string) {
	var cfg commands.AuthConfig
	var args []string
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--basic":
			if i+1 < len(rest) {
				cfg.Basic = rest[i+1]
				i++
			}
		case "--bearer":
			if i+1 < len(rest) {
				cfg.Bearer = rest[i+1]
				i++
			}
		case "--password", "-p":
			if i+1 < len(rest) {
				cfg.Password = rest[i+1]
				i++
			}
		default:
			args = append(args, rest[i])
		}
	}
	return cfg, args
}

// parseResolveFlags parses `vara resolve` options: a strategy flag
// (--ours/--theirs/--union/--auto), --list, and any positional pathspecs.
func parseResolveFlags(rest []string) (commands.ResolveArgs, error) {
	var ra commands.ResolveArgs
	for _, a := range rest {
		switch a {
		case "--ours":
			ra.Strategy = conflict.Ours
		case "--theirs":
			ra.Strategy = conflict.Theirs
		case "--union":
			ra.Strategy = conflict.Union
		case "--auto":
			ra.Strategy = conflict.Auto
		case "--list", "-l":
			ra.List = true
		case "--merge", "--diff3", "--zdiff3":
			s, _ := conflict.ParseMarkerStyle(strings.TrimPrefix(a, "--"))
			ra.Style = s
			ra.StyleSet = true
		default:
			if strings.HasPrefix(a, "-") {
				return ra, fmt.Errorf("unknown flag %q", a)
			}
			ra.Paths = append(ra.Paths, a)
		}
	}
	return ra, nil
}

// parseDiffFlags parses `vara diff [--staged|--cached] [<pathspec>...]`.
func parseDiffFlags(rest []string) (commands.DiffArgs, error) {
	var da commands.DiffArgs
	for _, a := range rest {
		switch a {
		case "--staged", "--cached":
			da.Staged = true
		default:
			if strings.HasPrefix(a, "-") {
				return da, fmt.Errorf("unknown flag %q", a)
			}
			da.Paths = append(da.Paths, a)
		}
	}
	return da, nil
}

// parseMergeContinueFlags parses `vara merge --continue [-m <msg>]`, returning the
// optional commit message (empty means use the default merge message).
func parseMergeContinueFlags(rest []string) (string, error) {
	msg := ""
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "-m", "--message":
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s requires a message argument", rest[i])
			}
			msg = rest[i+1]
			i++
		default:
			return "", fmt.Errorf("unknown flag %q", rest[i])
		}
	}
	return msg, nil
}

// parseIdentityFlags pulls --name/--email/--default out of `vara identity set`.
func parseIdentityFlags(rest []string) commands.IdentityArgs {
	var ia commands.IdentityArgs
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--default":
			ia.Default = true
		case "--name":
			if i+1 < len(rest) {
				ia.Name = rest[i+1]
				i++
			}
		case "--email":
			if i+1 < len(rest) {
				ia.Email = rest[i+1]
				i++
			}
		}
	}
	return ia
}

// die prints a formatted error to stderr and exits.
func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vara: "+format+"\n", args...)
	os.Exit(1)
}

func printUsage() {
	fmt.Print(`usage: vara <command> [<args>]

Commands:
  init      Create an empty VARA repository
  add       Stage file changes into the index
  rm        Remove tracked files and stage the deletion
  status    Show working tree status
  diff      Show changes (working tree vs index, or --staged vs HEAD)
  commit    Record staged changes as a new commit
  log       Show commit history (alias: history)
  history   Show commit history
  branch    List or create branches
  switch    Switch to a different branch
  merge     Join development histories together
  resolve   Automatically fix merge conflicts (ours/theirs/union/auto)
  undo      Revert to the last committed state
  verify    Check repository integrity
  doctor    Diagnose repo, config, and remote health

Remote commands (RFC-0014):
  clone     Clone a repository into a new directory
  remote    Manage the set of tracked repositories
  fetch     Download objects and refs from a remote
  pull      Fetch and integrate with the current branch
  push      Upload local branch commits to a remote
  serve     Serve repositories over HTTP (RFC-0016)
  gc        Reclaim unreferenced objects

Identity (RFC-0017):
  identity  Configure the name/email recorded as your commit author (offline)

Hub commands (RFC-0019, RFC-0020):
  repo      Manage repositories on a server (create/delete/rename/list/show)
  login     Log in to a server; the session is stored for later commands
  logout    Log out of a server and clear the stored session
  whoami    Show who the server thinks you are (and your capabilities)
  token     Manage API tokens (create/list/revoke)
  account   Manage accounts (create/disable/delete/passwd)

  version   Print VARA version

Run 'vara help <command>' for per-command usage.
Run 'vara --version' to print the version.
`)
}

var commandHelp = map[string]string{
	"init": `usage: vara init [<directory>]

Create an empty VARA repository in <directory> (default: current directory).

Creates the .vara/ directory layout as specified in RFC-0003.
`,
	"add": `usage: vara add <pathspec>...
       vara add .

Stage changes into the index. Use '.' to stage all modified and untracked files.
Specific files or directories can be named:

  vara add src/
  vara add main.go

Files matching .varaignore are excluded.
`,
	"rm": `usage: vara rm <pathspec>...

Remove tracked files from the working tree and stage the deletion, so the next
commit drops them from the tree. Pathspecs match like 'vara add' (exact file or
directory prefix).

During a merge, removing a conflicted file records the deletion as that path's
resolution — the way to resolve a modify/delete conflict by choosing to delete.
`,
	"status": `usage: vara status

Show the working tree status relative to the index.

Output shows:
  modified:  tracked files changed since last add
  deleted:   tracked files removed from disk
  ??         untracked files not in the index

During a merge it also lists unmerged/resolved paths from the conflict sidecar.
`,
	"diff": `usage: vara diff [--staged] [<pathspec>...]

Show line-level changes as a unified diff.

  (default)     working tree vs the index (unstaged changes)
  --staged      the index vs HEAD (what a commit would record); alias --cached
  <pathspec>    restrict to the given files/directories

During a merge, unmerged paths are shown first as an ours-vs-theirs diff, since a
plain diff of a file full of conflict markers is not useful.
`,
	"commit": `usage: vara commit -m "<message>"

Record staged index contents as a new commit. Requires at least one prior add.

The -m flag is required. The message should describe the change, not the
mechanism. Example:

  vara commit -m "fix scanner fingerprint fast path"
`,
	"history": `usage: vara history
       vara log

Show the commit history from HEAD, newest first.

The warm path uses the RFC-0013 binary graph index (graph.idx) and reads
only that one file. The cold path (first call after a commit) rebuilds the
index from the object store.
`,
	"log": `usage: vara log
       vara history

Alias for 'vara history'. See 'vara help history'.
`,
	"branch": `usage: vara branch [<name>]

With no arguments, list all branches. The current branch is marked with '*'.
With a name, create a new branch pointing to the current HEAD commit.

  vara branch            # list
  vara branch feature    # create
`,
	"switch": `usage: vara switch <branch>

Switch to an existing branch. The working tree is updated to match the
branch's latest commit, and the index is updated accordingly.

A snapshot of the current working tree is taken before switching so that
'vara undo' can restore the pre-switch state.
`,
	"merge": `usage: vara merge <branch>
       vara merge --continue [-m <msg>]
       vara merge --abort

Merge <branch> into the current branch.

  Fast-forward: if the current branch is an ancestor, the ref is advanced.
  Three-way:    Myers diff + diff3 merge. Files with conflicting edits
                receive conflict markers. Run 'vara resolve' then
                'vara merge --continue' (or 'vara commit') after a conflict.

  --continue    conclude an in-progress merge as a two-parent merge commit,
                once every conflict is resolved. Refuses, naming them, while
                any conflict remains unresolved. With no -m, uses the default
                "Merge branch '<branch>'" message.
  --abort       undo an in-progress conflicted merge, restoring the working
                tree and index to the pre-merge (HEAD) state. Refuses, naming
                the files, if it would discard edits you made to files the
                merge never touched.
`,
	"resolve": `usage: vara resolve [--ours | --theirs | --union | --auto] [<pathspec>...]
       vara resolve --list

Automatically fix the conflict markers left in the working tree by a conflicted
'vara merge' or 'vara pull', then re-stage the fixed files so 'vara commit' can
complete the merge (RFC-0008 §5).

Strategies (applied to every conflict hunk):
  --ours     keep our side of each conflict
  --theirs   keep their side of each conflict
  --union    keep both sides (ours first) — good for append-only files
  --auto     (default) base-aware three-way merge: combine non-overlapping edits
             (including disjoint adjacent ones), leave genuine conflicts marked

Marker style for conflicts --auto leaves behind (default: repo config
'merge.conflictStyle', else zdiff3):
  --merge    classic 2-way markers
  --diff3    add the ||||||| base section (common ancestor)
  --zdiff3   diff3 with shared leading/trailing lines hoisted out as context

Other:
  --list     list conflicted files and their conflict counts; change nothing
  <pathspec> restrict resolution to the given files/directories

After resolving, run 'vara merge --continue' (or 'vara commit') to record the
merge. A commit is refused while any conflict remains unresolved.

  vara resolve --list
  vara resolve --theirs
  vara resolve --union config/hosts
`,
	"undo": `usage: vara undo

Revert to the last known good state using three-layer recovery (RFC-0009):

  1. Journal rollback   — incomplete transaction (crash recovery)
  2. Reflog restore     — last committed HEAD position
  3. Snapshot restore   — pre-operation working-tree archive

Tries each layer in order; stops at the first one that succeeds.
`,
	"verify": `usage: vara verify

Run an eight-phase integrity check on the repository:

  1. Objects   — recompute SHA-256 for every object, compare to filename
  2. Trees     — verify all blob IDs in tree objects exist
  3. Commits   — verify tree and parent IDs in commit objects exist
  4. DAG       — DFS to confirm the commit graph is acyclic
  5. Refs      — verify all branch refs parse as valid commit IDs
  6. Index     — verify all index blob IDs exist in the object store
  7. Journal   — flag incomplete transactions that may need recovery
  8. Snapshots — list snapshot archives (count and size)
`,
	"version": `usage: vara version
       vara --version

Print the VARA version string.
`,
	"clone": `usage: vara clone <url> [<directory>]

Clone the repository at <url> into a new directory (RFC-0014 §9.1).

<url> is a local filesystem path, a file:// URL, or an http(s):// URL served
by 'vara serve' (RFC-0016). The clone:

  1. Creates <directory> (default: the last path segment of <url>).
  2. Records the source as remote 'origin'.
  3. Transfers the full object closure of every branch.
  4. Creates refs/remotes/origin/* tracking refs.
  5. Checks out the remote's default branch.

  vara clone ../other-repo
  vara clone http://localhost:8080/myproject
`,
	"remote": `usage: vara remote
       vara remote add <name> <url>
       vara remote remove <name>

Manage tracked repositories (RFC-0014 §4).

  vara remote                       list configured remotes
  vara remote add origin ../up      register a remote
  vara remote remove origin         delete a remote

Remotes are stored in .vara/config under [remote "<name>"].
`,
	"fetch": `usage: vara fetch [<remote>]

Download new objects from <remote> (default: origin) and update the
remote-tracking references refs/remotes/<remote>/* (RFC-0014 §9.2).

Local branches are NOT modified. Use 'vara merge' or 'vara pull' to
integrate fetched changes.
`,
	"pull": `usage: vara pull [<remote>] [<branch>]

Fetch from <remote> (default: origin) and integrate the corresponding
remote-tracking branch into the current branch (RFC-0014 §9.3).

  Fast-forward: the current branch is advanced and the working tree updated.
  Divergent:    a three-way merge is performed (same engine as 'vara merge').
                Conflicts leave markers to resolve and commit.
`,
	"push": `usage: vara push [<remote>] [<branch>] [--force]

Upload commits on <branch> (default: current branch) to <remote>
(default: origin), then update the remote's branch (RFC-0014 §9.4).

The remote rejects a non-fast-forward update unless --force is given:

  vara push origin main
  vara push origin main --force

A rejected push changes nothing on either side.
`,
	"serve": `usage: vara serve [--addr <host:port>] [--root <dir>] [--policy <dir>]
                  [--meta <dir>] [--basic <user:secret>]... [--bearer <token:subject>]...

Serve the VARA repositories under <root> (default: current directory) over
HTTP, implementing the RFC-0016 remote transport protocol (HTTP binding v1),
with optional identity (RFC-0017), authorization (RFC-0018), and the RFC-0019
repository control plane.

Each subdirectory of <root> that is a VARA repository is served under its own
name. Clients clone/fetch/pull/push over http:// URLs:

  vara serve --root ./repositories --addr :8080
  vara clone http://localhost:8080/myproject

Identity (RFC-0017), repeatable:
  --basic  alice:s3cret     accept HTTP Basic for user 'alice'
  --bearer tok123:bob       accept Bearer token 'tok123' as subject 'bob'
  With no credential flags the server is anonymous.

Authorization (RFC-0018):
  --policy ./policy         enforce per-repo policy from <dir>/<repo>.json
  Policy maps subjects to capabilities: read, create-ref, push, force-push,
  delete-ref. Absent policy = default-deny. 'anonymous' is an ordinary subject.

Repository control plane (RFC-0019):
  --meta ./meta             enable /_vara/repositories (create/delete/rename/
                            list). Requires --policy: creation seeds the owner's
                            policy. Server-scope grants (create-repo, list-repos)
                            live in <policy>/_server.json. Only Active repos are
                            served on the data plane.

Account control plane (RFC-0020):
  --accounts ./accounts     enable /_vara/sessions, /_vara/tokens, /_vara/accounts
                            and durable password/session/token authentication.
                            Account admin needs the manage-accounts capability on
                            the server (_server.json). Passwords are argon2id.

Hub UI (RFC-0021):
  --hub ./web               serve a same-origin static Hub UI from <dir> at any
                            path not claimed by the API or data plane. Enables the
                            read API (repo summary/branches/commits) and browser
                            cookie sessions. Serve behind TLS in production.

With neither identity nor policy configured this is an anonymous, allow-all
server; do NOT expose that as a write endpoint on an untrusted network. Press
Ctrl-C to shut down gracefully.
`,
	"repo": `usage: vara repo <create|delete|rename|list|show> <server-url> [args]
                 [--basic <user:secret>] [--bearer <token>]

Manage repositories on a VARA server through its RFC-0019 control plane. The
first argument after the subcommand is always the server base URL.

Subcommands:
  list    <server-url>                     list repositories (needs list-repos)
  create  <server-url> <name>              create a repository (needs create-repo)
          [--visibility private|public] [--description <text>]
  show    <server-url> <name>              show a repository's descriptor
  delete  <server-url> <name>              delete a repository (needs delete-repo)
  rename  <server-url> <name> <new-name>   rename (needs rename-repo + create-repo)

Credentials (choose one):
  --basic  alice:s3cret     present HTTP Basic
  --bearer tok123           present a Bearer token

Examples:
  vara repo create http://localhost:8080 myproject --basic alice:s3cret
  vara repo list   http://localhost:8080 --basic alice:s3cret
`,
	"identity": `usage: vara identity show
       vara identity set [--default] --name "<name>" --email "<email>"

Configure the developer identity recorded as the author/committer of your
commits (RFC-0017). This is local configuration, NOT authentication — it never
contacts a server, and your account ID never enters a commit object.

  vara identity show                              print the resolved identity
  vara identity set  --name "A" --email a@x.io    set THIS repository's identity
  vara identity set --default --name "A" --email a@x.io   set your user default

Resolution at commit time: repository identity (.vara/identity) overrides the
user default ($VARA_HOME/identity). If neither is set, 'vara commit' fails with
guidance rather than inventing a placeholder author.

'vara identity show' is fully offline. The "Account/Status" lines reflect a
local account association (from a prior 'vara login') and never imply a request.
`,
	"login": `usage: vara login <server-url> <username> --password <pw>

Authenticate to a server (RFC-0020) and store the resulting session in your
credential store, keyed by the server's origin. The session secret is NOT
printed. Subsequent clone/fetch/pull/push to any repository on that server
authenticate automatically — no per-command --bearer needed.

  vara login http://localhost:8080 alice --password s3cret

Credentials are stored under your VARA home ($VARA_HOME or the OS config dir),
never inside a repository, in a file with owner-only (0600) permissions.
`,
	"logout": `usage: vara logout <server-url>

Revoke the stored session on the server (best-effort) and remove the credential
from your local store (RFC-0020). After logout, requests to that server are
anonymous again.

  vara logout http://localhost:8080
`,
	"whoami": `usage: vara whoami <server-url> [--repo <name>] [--basic u:s | --bearer t]

Show the identity the server resolves for your credential (RFC-0020 §8.5). With
no --basic/--bearer, the stored login for the server's origin is used, so this
"just works" after 'vara login'. With --repo, also list which capabilities you
hold on that repository — handy for debugging why a push or an admin action is
denied. Use --repo _server for server-scope capabilities.

  vara whoami http://localhost:8080
  vara whoami http://localhost:8080 --repo demo
`,
	"token": `usage: vara token <create|list|revoke> <server-url> [args] [--basic u:s | --bearer t]

Manage your API tokens (RFC-0020). A token carries your account's authority and
is long-lived until revoked; its secret is shown once at creation.

  vara token create http://localhost:8080 ci-bot --basic alice:s3cret
  vara token list   http://localhost:8080 --basic alice:s3cret
  vara token revoke http://localhost:8080 <token-id> --basic alice:s3cret
`,
	"account": `usage: vara account <create|disable|delete|passwd> <server-url> <username>
                    [--password <pw>] [--basic u:s | --bearer t]
   or: vara account create --accounts <dir> --username <name> --password <pw>
                    [--policy <dir>] [--force]        (on-host bootstrap)

Manage accounts (RFC-0020). Over HTTP, create/disable/delete need the
manage-accounts capability on the server; an account may change its own password.

  vara account create  http://localhost:8080 bob --password hunter2 --basic admin:pw
  vara account passwd   http://localhost:8080 bob --password newpw   --bearer <bob-token>
  vara account disable  http://localhost:8080 bob --basic admin:pw

On-host bootstrap (RFC-0020 §10b) creates the FIRST admin directly on the
server's filesystem — no HTTP, no authentication. It refuses if accounts already
exist (unless --force) and, given --policy, grants the new admin manage-accounts,
create-repo, and list-repos in <policy>/_server.json. Run this once at install:

  vara account create --accounts /var/lib/vara/accounts \
                      --policy   /var/lib/vara/policy \
                      --username admin --password <pw>
`,
	"doctor": `usage: vara doctor [<server-url>] [--basic <user:secret> | --bearer <token>]

Run a fast, read-only health check and print a report. With no arguments it
checks the current repository (format, HEAD, object store, refs, index, commit
graph), your configuration, and the reachability of each configured remote.

Pass a server URL to focus the remote checks on it (with optional credentials):

  vara doctor
  vara doctor http://localhost:8080/myrepo --basic alice:s3cret

Exits non-zero if any check fails, so it is safe to use in scripts. For a deep
object/DAG integrity audit, use 'vara verify'.
`,
	"gc": `usage: vara gc [--dry-run]

Reclaim unreferenced objects left behind by interrupted transfers
(clone/fetch/push) or discarded history (RFC-0014 §12).

An object is kept if it is reachable from any reference, HEAD, or a commit
recorded in HEAD's reflog — so anything 'vara undo' could restore is safe.

  vara gc              reclaim unreferenced objects
  vara gc --dry-run    report what would be reclaimed, delete nothing
`,
}

func printCommandHelp(cmd string) {
	h, ok := commandHelp[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "vara: no help for '%s'\n", cmd)
		os.Exit(1)
	}
	fmt.Print(h)
}
