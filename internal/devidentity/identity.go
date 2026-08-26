// Package devidentity manages a developer's *commit identity* — the name and
// email recorded as the author/committer of a commit. This is deliberately and
// completely separate from authentication (who is making a remote request) and
// authorization (what that subject may do): an identity is local configuration,
// never a credential, and an account ID never enters an immutable commit object
// (RFC-0017 §4, the three-concept separation).
//
// RFC:
//
//	VARA-RFC-0017 Identity
//
// Layer: a client-config leaf above internal/varahome and below the command
// layer. It reads and writes two small files and performs no network I/O — the
// whole point is that `vara commit` and `vara identity show` work offline.
package devidentity

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/varahome"
)

// Identity is a developer's commit identity.
type Identity struct {
	Name  string
	Email string
}

// String renders the identity in the canonical "Name <email>" form stored in
// commit objects.
func (id Identity) String() string {
	return fmt.Sprintf("%s <%s>", id.Name, id.Email)
}

// Scope names where a resolved identity came from, for `vara identity show`.
type Scope string

const (
	ScopeRepository Scope = "Repository"
	ScopeDefault    Scope = "User default"
	ScopeNone       Scope = "None"
)

// identityFile is the basename of both the repository identity (inside the
// repo's .vara/) and the user default identity (inside the VARA home).
const identityFile = "identity"

// ErrNoIdentity is returned by Resolve when neither a repository nor a default
// identity is configured. Callers turn it into an actionable message rather
// than inventing a placeholder author — RFC-0017 forbids a fake fallback.
type ErrNoIdentity struct{}

func (ErrNoIdentity) Error() string {
	return "no VARA identity configured"
}

// RepoPath returns the path to a repository's identity file given its .vara
// directory.
func RepoPath(varaDir string) string {
	return filepath.Join(varaDir, identityFile)
}

// defaultPath returns the path to the user default identity file.
func defaultPath() (string, error) {
	home, err := varahome.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, identityFile), nil
}

// Resolve returns the identity that a commit made in the repository at varaDir
// should carry, following the precedence in RFC-0017:
//
//  1. Repository identity  (<varaDir>/identity)  — overrides the default
//  2. User default         (<VARA_HOME>/identity)
//  3. ErrNoIdentity        — never a fabricated "User <user@example.com>"
//
// The returned Scope reports which tier supplied the identity.
func Resolve(varaDir string) (Identity, Scope, error) {
	if id, ok, err := readIdentity(RepoPath(varaDir)); err != nil {
		return Identity{}, ScopeNone, err
	} else if ok {
		return id, ScopeRepository, nil
	}
	dp, err := defaultPath()
	if err != nil {
		return Identity{}, ScopeNone, err
	}
	if id, ok, err := readIdentity(dp); err != nil {
		return Identity{}, ScopeNone, err
	} else if ok {
		return id, ScopeDefault, nil
	}
	return Identity{}, ScopeNone, ErrNoIdentity{}
}

// BestEffort resolves the commit identity like Resolve but, when none is
// configured, returns the empty Identity and false instead of an error. It is
// used for local-only reflog metadata, where a missing identity must never fail
// an offline operation such as `vara switch` or `vara branch`.
func BestEffort(varaDir string) (Identity, bool) {
	id, _, err := Resolve(varaDir)
	if err != nil {
		return Identity{}, false
	}
	return id, true
}

// SetRepo writes the repository identity for the repo at varaDir.
func SetRepo(varaDir string, id Identity) error {
	if err := validate(id); err != nil {
		return err
	}
	return writeIdentity(RepoPath(varaDir), id)
}

// SetDefault writes the user default identity under the VARA home, creating the
// home directory if necessary.
func SetDefault(id Identity) error {
	if err := validate(id); err != nil {
		return err
	}
	if _, err := varahome.Ensure(); err != nil {
		return err
	}
	dp, err := defaultPath()
	if err != nil {
		return err
	}
	return writeIdentity(dp, id)
}

// Default returns the user default identity, if one is configured.
func Default() (Identity, bool, error) {
	dp, err := defaultPath()
	if err != nil {
		return Identity{}, false, err
	}
	return readIdentity(dp)
}

func validate(id Identity) error {
	if strings.TrimSpace(id.Name) == "" {
		return fmt.Errorf("identity: --name is required")
	}
	if strings.TrimSpace(id.Email) == "" {
		return fmt.Errorf("identity: --email is required")
	}
	// A newline would corrupt the key=value file and could smuggle extra keys.
	if strings.ContainsAny(id.Name+id.Email, "\r\n") {
		return fmt.Errorf("identity: name and email must not contain newlines")
	}
	if !strings.Contains(id.Email, "@") {
		return fmt.Errorf("identity: %q does not look like an email address", id.Email)
	}
	return nil
}

// readIdentity parses a "key = value" identity file. A missing file yields
// (_, false, nil); a present file missing name or email is treated as absent so
// a half-written file never silently produces a partial author.
func readIdentity(path string) (Identity, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Identity{}, false, nil
		}
		return Identity{}, false, err
	}
	defer f.Close()

	var id Identity
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			id.Name = strings.TrimSpace(val)
		case "email":
			id.Email = strings.TrimSpace(val)
		}
	}
	if err := sc.Err(); err != nil {
		return Identity{}, false, err
	}
	if id.Name == "" || id.Email == "" {
		return Identity{}, false, nil
	}
	return id, true, nil
}

// writeIdentity atomically writes an identity file (temp + rename), so a crash
// mid-write never leaves a truncated file behind.
func writeIdentity(path string, id Identity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body := fmt.Sprintf("name = %s\nemail = %s\n", id.Name, id.Email)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
