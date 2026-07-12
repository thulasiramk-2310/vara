package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
)

// RunAccountBootstrap is the ON-HOST account bootstrap (RFC-0020 §10b): it
// creates the first admin account by writing directly to the server's credential
// store on the local filesystem, with no HTTP and no authentication. It exists to
// break the chicken-and-egg of remote account admin (which needs an account that
// already holds manage-accounts). It refuses to run if any account already exists
// unless --force is given, and seeds the _server policy so the new admin can
// immediately administer accounts and repositories over the wire.
//
// It is selected (over the HTTP `vara account` client) by the presence of the
// --accounts flag, which names the same credential-store directory as
// `vara serve --accounts`.
func RunAccountBootstrap(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("on-host bootstrap supports only:\n" +
			"  vara account create --accounts <dir> --username <name> --password <pw> [--policy <dir>] [--force]")
	}
	var accountsDir, policyDir, username, password string
	var force bool
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--accounts":
			accountsDir, i = nextArg(rest, i)
		case "--policy":
			policyDir, i = nextArg(rest, i)
		case "--username", "-u":
			username, i = nextArg(rest, i)
		case "--password":
			password, i = nextArg(rest, i)
		case "--force":
			force = true
		}
	}
	if accountsDir == "" || username == "" || password == "" {
		return fmt.Errorf("bootstrap requires --accounts <dir>, --username <name>, and --password <pw>")
	}
	absAccounts, err := filepath.Abs(accountsDir)
	if err != nil {
		return err
	}

	// Refuse on a store that already has accounts, unless forced — so a bootstrap
	// can't silently add a second admin to a live host.
	if !force && accountsExist(absAccounts) {
		return fmt.Errorf("accounts already exist under %s; refusing to bootstrap (use --force to add another admin)", accountsDir)
	}

	mgr, err := identity.NewAccountManager(absAccounts)
	if err != nil {
		return err
	}
	if err := mgr.CreateAccount(username, password); err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	fmt.Printf("created account %q (on-host)\n", username)

	if policyDir == "" {
		fmt.Printf("note: no --policy given — grant %q manage-accounts in <policy>/_server.json to enable remote account admin\n", username)
		return nil
	}
	if err := seedServerPolicy(policyDir, username); err != nil {
		return fmt.Errorf("seed _server policy: %w", err)
	}
	fmt.Printf("granted %q manage-accounts, create-repo, list-repos in %s\n", username, filepath.Join(policyDir, "_server.json"))
	return nil
}

func nextArg(args []string, i int) (string, int) {
	if i+1 < len(args) {
		return args[i+1], i + 1
	}
	return "", i
}

// accountsExist reports whether the credential store already holds any account.
func accountsExist(accountsDir string) bool {
	entries, err := os.ReadDir(filepath.Join(accountsDir, "accounts"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return true
		}
	}
	return false
}

// serverPolicyShape is the on-disk _server.json shape (mirrors RFC-0018 §7.1);
// used only to MERGE the new admin's grant into any existing server policy.
type serverPolicyShape struct {
	Version  int                 `json:"version"`
	Subjects map[string][]string `json:"subjects"`
}

// seedServerPolicy grants username the server-scope admin capabilities in
// <policyDir>/_server.json, preserving any existing subjects/grants. Writing goes
// through authz.WritePolicy so the identity layer never formats policy itself and
// the caps are validated against the closed set.
func seedServerPolicy(policyDir, username string) error {
	if err := os.MkdirAll(policyDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(policyDir, "_server.json")

	existing := serverPolicyShape{Subjects: map[string][]string{}}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("existing %s is malformed: %w", path, err)
		}
		if existing.Subjects == nil {
			existing.Subjects = map[string][]string{}
		}
	}

	// Merge the admin grant into the current set for this subject.
	set := map[string]bool{}
	for _, c := range existing.Subjects[username] {
		set[c] = true
	}
	for _, c := range []string{"manage-accounts", "create-repo", "list-repos"} {
		set[c] = true
	}

	subjects := map[string][]authz.Capability{}
	for subj, caps := range existing.Subjects {
		if subj == username {
			continue
		}
		subjects[subj] = toCapabilities(caps)
	}
	admin := make([]string, 0, len(set))
	for c := range set {
		admin = append(admin, c)
	}
	sort.Strings(admin)
	subjects[username] = toCapabilities(admin)

	return authz.WritePolicy(path, subjects)
}

func toCapabilities(ss []string) []authz.Capability {
	out := make([]authz.Capability, len(ss))
	for i, s := range ss {
		out[i] = authz.Capability(s)
	}
	return out
}
