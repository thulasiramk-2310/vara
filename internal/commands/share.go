// Package commands — vara share: give another account access to a repository on a
// server you run. It edits the server's policy files (RFC-0018), the same ones a
// person used to edit by hand, and does the two edits that sharing needs together:
// the repository's own policy, and `list-repos` in _server.json so the person also
// sees the repository in the Hub's web page.
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/authz"
)

// ShareLevel is how much a person may do in a repository.
type ShareLevel string

const (
	ShareRead  ShareLevel = "read"  // clone, fetch, pull
	ShareWrite ShareLevel = "write" // read, plus push and create branches
	ShareNone  ShareLevel = "none"  // no access
)

// shareCaps is what each level grants. Neither level can force-push, delete a
// branch, rename or delete the repository, or change who may do any of that.
var shareCaps = map[ShareLevel][]authz.Capability{
	ShareRead:  {authz.CapRead},
	ShareWrite: {authz.CapRead, authz.CapCreateRef, authz.CapPush},
}

// ownerCaps mark a subject as an owner of a repository. Sharing never changes an
// owner's access: that would let a typo lock the owner out of their own repository.
var ownerCaps = []string{"admin", "delete-repo", "rename-repo", "force-push", "delete-ref"}

var shareName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// ShareOptions describes one change.
type ShareOptions struct {
	PolicyDir   string
	AccountsDir string // optional: when set, the account must exist
	Repo        string
	User        string
	Level       ShareLevel
}

type policyShape struct {
	Version  int                 `json:"version"`
	Subjects map[string][]string `json:"subjects"`
}

func loadPolicyShape(path string) (policyShape, error) {
	p := policyShape{Version: 1, Subjects: map[string][]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("%s is malformed: %w", path, err)
	}
	if p.Subjects == nil {
		p.Subjects = map[string][]string{}
	}
	return p, nil
}

func savePolicyShape(path string, p policyShape) error {
	subjects := make(map[string][]authz.Capability, len(p.Subjects))
	for who, caps := range p.Subjects {
		subjects[who] = toCapabilities(caps)
	}
	return authz.WritePolicy(path, subjects)
}

func hasAny(have []string, want []string) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

// Share applies one change and returns a sentence saying what happened.
func Share(o ShareOptions) (string, error) {
	if o.PolicyDir == "" {
		return "", fmt.Errorf("share needs --policy <dir>: the policy folder of the server you run")
	}
	if !shareName.MatchString(o.Repo) || o.Repo == "_server" {
		return "", fmt.Errorf("%q is not a repository name", o.Repo)
	}
	if !shareName.MatchString(o.User) {
		return "", fmt.Errorf("%q is not an account name", o.User)
	}
	if o.Level != ShareRead && o.Level != ShareWrite && o.Level != ShareNone {
		return "", fmt.Errorf("access must be --read, --write or --none")
	}
	if o.AccountsDir != "" {
		if _, err := os.Stat(filepath.Join(o.AccountsDir, "accounts", o.User+".json")); err != nil {
			return "", fmt.Errorf("there is no account named %q on this server. Create it first, while the server is running and you are signed in: vara account create <server-url> %s --password '<a first password>'", o.User, o.User)
		}
	}

	repoPath := authz.PolicyPath(o.PolicyDir, o.Repo)
	repo, err := loadPolicyShape(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("there is no repository named %q in %s (create it on the server first)", o.Repo, o.PolicyDir)
		}
		return "", err
	}
	if hasAny(repo.Subjects[o.User], ownerCaps) {
		return "", fmt.Errorf("%s is an owner of %s, and sharing does not change an owner's access", o.User, o.Repo)
	}

	if o.Level == ShareNone {
		if _, had := repo.Subjects[o.User]; !had {
			return fmt.Sprintf("%s already has no access to %s.", o.User, o.Repo), nil
		}
		delete(repo.Subjects, o.User)
	} else {
		caps := make([]string, 0, 3)
		for _, c := range shareCaps[o.Level] {
			caps = append(caps, string(c))
		}
		repo.Subjects[o.User] = caps
	}
	if err := savePolicyShape(repoPath, repo); err != nil {
		return "", err
	}

	// The Hub's repository list needs a server-level permission as well.
	serverPath := filepath.Join(o.PolicyDir, "_server.json")
	server, err := loadPolicyShape(serverPath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	switch {
	case o.Level != ShareNone:
		if !hasAny(server.Subjects[o.User], []string{"list-repos"}) {
			server.Subjects[o.User] = append(server.Subjects[o.User], "list-repos")
			sort.Strings(server.Subjects[o.User])
			if err := savePolicyShape(serverPath, server); err != nil {
				return "", err
			}
		}
	default:
		// Take list-repos away only when it is all this person has and no other repository
		// is shared with them; an admin keeps it.
		if caps := server.Subjects[o.User]; len(caps) == 1 && caps[0] == "list-repos" && !hasOtherAccess(o.PolicyDir, o.User, o.Repo) {
			delete(server.Subjects, o.User)
			if err := savePolicyShape(serverPath, server); err != nil {
				return "", err
			}
		}
	}

	switch o.Level {
	case ShareRead:
		return fmt.Sprintf("Shared %s with %s: they can clone, fetch and pull, and they will see it in the Hub.", o.Repo, o.User), nil
	case ShareWrite:
		return fmt.Sprintf("Shared %s with %s: they can clone, fetch, pull and push, and they will see it in the Hub.", o.Repo, o.User), nil
	}
	return fmt.Sprintf("Removed %s's access to %s.", o.User, o.Repo), nil
}

// hasOtherAccess reports whether user holds any grant in a repository other than except.
func hasOtherAccess(policyDir, user, except string) bool {
	entries, err := os.ReadDir(policyDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "_server.json" || name == except+".json" {
			continue
		}
		if p, err := loadPolicyShape(filepath.Join(policyDir, name)); err == nil {
			if len(p.Subjects[user]) > 0 {
				return true
			}
		}
	}
	return false
}

// levelOf names a subject's access in plain words.
func levelOf(caps []string) string {
	switch {
	case hasAny(caps, []string{"admin"}):
		return "owner"
	case hasAny(caps, []string{"push"}):
		return "write"
	case len(caps) == 1 && caps[0] == "read":
		return "read"
	}
	return "custom (" + strings.Join(caps, ", ") + ")"
}

// ShareList says who has access to one repository, or to every repository when repo is "".
func ShareList(policyDir, repo string) (string, error) {
	if policyDir == "" {
		return "", fmt.Errorf("share needs --policy <dir>: the policy folder of the server you run")
	}
	var repos []string
	if repo != "" {
		if !shareName.MatchString(repo) || repo == "_server" {
			return "", fmt.Errorf("%q is not a repository name", repo)
		}
		repos = []string{repo}
	} else {
		entries, err := os.ReadDir(policyDir)
		if err != nil {
			return "", err
		}
		for _, e := range entries {
			n := e.Name()
			if !e.IsDir() && strings.HasSuffix(n, ".json") && n != "_server.json" {
				repos = append(repos, strings.TrimSuffix(n, ".json"))
			}
		}
		sort.Strings(repos)
	}
	var b strings.Builder
	for _, r := range repos {
		p, err := loadPolicyShape(authz.PolicyPath(policyDir, r))
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("there is no repository named %q in %s", r, policyDir)
			}
			return "", err
		}
		fmt.Fprintf(&b, "%s\n", r)
		who := make([]string, 0, len(p.Subjects))
		for u := range p.Subjects {
			who = append(who, u)
		}
		sort.Strings(who)
		if len(who) == 0 {
			b.WriteString("  nobody has access\n")
		}
		for _, u := range who {
			fmt.Fprintf(&b, "  %-20s %s\n", u, levelOf(p.Subjects[u]))
		}
	}
	if b.Len() == 0 {
		return "No repositories in " + policyDir + " yet.\n", nil
	}
	return b.String(), nil
}

// RunShare is the command line: vara share --policy DIR [--accounts DIR] <repo> <user> [--read|--write|--none]
// and vara share --policy DIR [<repo>] to list who has access.
func RunShare(args []string) error {
	var o ShareOptions
	var positional []string
	level := ShareLevel("")
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--policy", "--accounts":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a folder", a)
			}
			i++
			if a == "--policy" {
				o.PolicyDir = args[i]
			} else {
				o.AccountsDir = args[i]
			}
		case "--read", "--write", "--none":
			if level != "" {
				return fmt.Errorf("choose one of --read, --write, --none")
			}
			level = ShareLevel(strings.TrimPrefix(a, "--"))
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown option %s (see: vara share --help)", a)
			}
			positional = append(positional, a)
		}
	}
	if len(positional) > 2 {
		return fmt.Errorf("share takes a repository and a person, nothing more")
	}
	if len(positional) < 2 {
		if level != "" {
			return fmt.Errorf("to change access, give both the repository and the person: vara share --policy <dir> <repo> <user> --write")
		}
		repo := ""
		if len(positional) == 1 {
			repo = positional[0]
		}
		out, err := ShareList(o.PolicyDir, repo)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	}
	o.Repo, o.User = positional[0], positional[1]
	if level == "" {
		level = ShareRead
	}
	o.Level = level
	msg, err := Share(o)
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}
