// Package commands — vara identity (RFC-0017 commit identity, client side).
//
// `vara identity` configures the name and email recorded as the author of a
// commit. It is purely local configuration: every subcommand here reads and
// writes files only and performs NO network request — `vara identity show` in
// particular must be answerable on a plane with no server in sight. Identity is
// not authentication; the linked-account line shown by `show` is read from the
// local credential store, and displaying it never contacts the server.
package commands

import (
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/credstore"
	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/color"
)

// IdentityArgs carries the parsed flags for `vara identity set`.
type IdentityArgs struct {
	Name    string
	Email   string
	Default bool // write the user-default identity instead of the repo identity
}

// RunIdentity dispatches `vara identity <show|set> [...]`. It is offline.
func RunIdentity(sub string, ia IdentityArgs) error {
	switch sub {
	case "", "show":
		return runIdentityShow()
	case "set":
		return runIdentitySet(ia)
	default:
		return fmt.Errorf("identity: unknown subcommand %q (want show|set)", sub)
	}
}

func runIdentitySet(ia IdentityArgs) error {
	id := devidentity.Identity{Name: strings.TrimSpace(ia.Name), Email: strings.TrimSpace(ia.Email)}
	if ia.Default {
		if err := devidentity.SetDefault(id); err != nil {
			return err
		}
		fmt.Printf("Set user default identity: %s\n", id)
		return nil
	}
	// Repository scope requires being inside a repository.
	repo, err := repository.Discover(".")
	if err != nil {
		return fmt.Errorf("identity set: not inside a VARA repository\n" +
			"  (use 'vara identity set --default' to set your user-wide identity)")
	}
	if err := devidentity.SetRepo(repo.VaraDir, id); err != nil {
		return err
	}
	fmt.Printf("Set repository identity: %s\n", id)
	return nil
}

// runIdentityShow prints the resolved commit identity and, if one exists, the
// locally linked account — all from local files, no network.
func runIdentityShow() error {
	fmt.Println(color.Bold("VARA Identity"))
	fmt.Println()

	// Resolve against the current repository when we are inside one; otherwise
	// only the user-default identity is visible.
	var varaDir string
	if repo, err := repository.Discover("."); err == nil {
		varaDir = repo.VaraDir
	}

	id, scope := resolveShownIdentity(varaDir)
	if scope == devidentity.ScopeNone {
		fmt.Printf("  %-10s %s\n", "Name", color.Dim("(not set)"))
		fmt.Printf("  %-10s %s\n", "Email", color.Dim("(not set)"))
		fmt.Printf("  %-10s %s\n", "Scope", color.Dim(string(devidentity.ScopeNone)))
	} else {
		fmt.Printf("  %-10s %s\n", "Name", id.Name)
		fmt.Printf("  %-10s %s\n", "Email", id.Email)
		fmt.Printf("  %-10s %s\n", "Scope", string(scope))
	}

	account, origin := linkedAccount(varaDir)
	if account == "" {
		fmt.Printf("  %-10s %s\n", "Account", "Not linked")
		fmt.Printf("  %-10s %s\n", "Status", "Local")
	} else {
		detail := account
		if origin != "" {
			detail = fmt.Sprintf("%s (%s)", account, origin)
		}
		fmt.Printf("  %-10s %s\n", "Account", detail)
		fmt.Printf("  %-10s %s\n", "Status", "Linked")
	}

	if scope == devidentity.ScopeNone {
		fmt.Println()
		fmt.Println(color.Dim("  No identity configured. Configure one with:"))
		fmt.Println(color.Dim("    vara identity set --default --name \"Your Name\" --email \"you@example.com\""))
	}
	return nil
}

// resolveShownIdentity resolves the identity for display, tolerating "none".
func resolveShownIdentity(varaDir string) (devidentity.Identity, devidentity.Scope) {
	if varaDir != "" {
		id, scope, err := devidentity.Resolve(varaDir)
		if err == nil {
			return id, scope
		}
		return devidentity.Identity{}, devidentity.ScopeNone
	}
	// Outside a repository only the default is meaningful.
	if id, ok, err := devidentity.Default(); err == nil && ok {
		return id, devidentity.ScopeDefault
	}
	return devidentity.Identity{}, devidentity.ScopeNone
}

// linkedAccount reports the account this identity is locally associated with,
// read from the credential store — offline. It prefers the account for the
// current repository's origin remote, then falls back to the sole stored
// account if there is exactly one. It returns ("", "") when nothing is linked.
func linkedAccount(varaDir string) (account, origin string) {
	store, err := credstore.Load()
	if err != nil {
		return "", ""
	}
	entries := store.List()
	if len(entries) == 0 {
		return "", ""
	}
	// Prefer the credential matching the repo's origin remote.
	if varaDir != "" {
		if cfg, err := loadConfig(varaDir); err == nil {
			if r, ok := cfg.Remote("origin"); ok {
				if o, err := credstore.Origin(r.URL); err == nil {
					if c, ok := store.Servers[o]; ok {
						return c.Username, o
					}
				}
			}
		}
	}
	if len(entries) == 1 {
		return entries[0].Credential.Username, entries[0].Origin
	}
	return fmt.Sprintf("%d accounts", len(entries)), ""
}
