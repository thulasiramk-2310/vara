// Package credstore is the client-side store for remote authentication
// credentials — the session or token secret obtained by `vara login`, keyed by
// the *server origin* so one login authenticates every repository on that
// server. It is authentication state, kept strictly apart from commit identity
// (devidentity) and from any repository: a secret is NEVER written inside a
// `.vara/` directory (RFC-0020 §11).
//
// RFC:
//
//	VARA-RFC-0017 Identity        (credential presentation)
//	VARA-RFC-0020 Accounts        (§11 credential storage)
//
// Storage backend: the credentials live in a single 0600 JSON file under the
// VARA home, written atomically. The file is the safest mechanism wired into
// the current environment; the API is deliberately backend-neutral so an OS
// keychain (Windows Credential Manager, macOS Keychain, Secret Service) can be
// slotted in later without touching callers. Secrets are only ever stored and
// returned — this package never prints or logs them.
//
// Layer: a client-config leaf above internal/varahome, below the command layer.
package credstore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thulasiramk-2310/vara/internal/varahome"
)

// credentialsFile is the basename of the store under the VARA home.
const credentialsFile = "credentials.json"

// Credential is the stored authentication material for one server origin.
type Credential struct {
	// Username is the account the credential authenticates as. It is not a
	// secret; it is kept for display (whoami, doctor) and to record the local
	// account linkage.
	Username string `json:"username"`

	// Secret is the bearer secret (a session token or an API token) presented
	// as `Authorization: Bearer <secret>`. It is sensitive: never print it.
	Secret string `json:"secret"`

	// ExpiresAt is the RFC3339 expiry of a session secret, or empty for a
	// long-lived API token that does not expire.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Expired reports whether a session credential is known, locally, to have
// expired. An empty ExpiresAt (an API token) never expires. An unparseable
// timestamp is treated as not-expired so a storage quirk never silently
// suppresses a credential — the server remains the authority.
func (c Credential) Expired() bool {
	if c.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().After(t)
}

// Store is the in-memory view of the credential file.
type Store struct {
	Version int                   `json:"version"`
	Servers map[string]Credential `json:"servers"`
}

// Path returns the credential file's absolute path.
func Path() (string, error) {
	home, err := varahome.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, credentialsFile), nil
}

// Load reads the credential store. A missing file yields an empty store, not an
// error — a fresh install simply has no credentials yet.
func Load() (*Store, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{Version: 1, Servers: map[string]Credential{}}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("credstore: corrupt %s: %w", path, err)
	}
	if s.Servers == nil {
		s.Servers = map[string]Credential{}
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return &s, nil
}

// Save writes the store atomically with 0600 permissions. Writing the temp file
// with 0600 up front (rather than relying on a later chmod) means the secret is
// never briefly world-readable on disk.
func (s *Store) Save() error {
	home, err := varahome.Ensure()
	if err != nil {
		return err
	}
	path := filepath.Join(home, credentialsFile)
	if s.Version == 0 {
		s.Version = 1
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	// Best-effort re-assert 0600 in case an inherited umask or a pre-existing
	// file loosened it; ignore on platforms where it is a no-op.
	_ = os.Chmod(path, 0600)
	return nil
}

// Get returns the credential stored for the origin of rawURL, if any.
func (s *Store) Get(rawURL string) (Credential, bool) {
	origin, err := Origin(rawURL)
	if err != nil {
		return Credential{}, false
	}
	c, ok := s.Servers[origin]
	return c, ok
}

// Set stores cred under the origin of rawURL, replacing any existing entry.
func (s *Store) Set(rawURL string, cred Credential) error {
	origin, err := Origin(rawURL)
	if err != nil {
		return err
	}
	if s.Servers == nil {
		s.Servers = map[string]Credential{}
	}
	s.Servers[origin] = cred
	return nil
}

// Delete removes the credential for the origin of rawURL. It reports whether an
// entry was present.
func (s *Store) Delete(rawURL string) (bool, error) {
	origin, err := Origin(rawURL)
	if err != nil {
		return false, err
	}
	if _, ok := s.Servers[origin]; !ok {
		return false, nil
	}
	delete(s.Servers, origin)
	return true, nil
}

// Entry pairs an origin with its credential for listing.
type Entry struct {
	Origin     string
	Credential Credential
}

// List returns every stored credential, sorted by origin, for offline display
// (`vara identity show`, `vara doctor`). Callers must not print Secret.
func (s *Store) List() []Entry {
	out := make([]Entry, 0, len(s.Servers))
	for origin, c := range s.Servers {
		out = append(out, Entry{Origin: origin, Credential: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Origin < out[j].Origin })
	return out
}

// Origin reduces a full repository or server URL to its authentication origin —
// scheme://host[:port], lowercased host, no path, query, or userinfo. Keying by
// origin (not the full URL) is what lets a single `vara login` cover every
// repository on the same server (RFC-0020 §11):
//
//	https://vara.example.com/repo-a  ┐
//	https://vara.example.com/repo-b  ├─▶  https://vara.example.com
//	https://vara.example.com/repo-c  ┘
func Origin(rawURL string) (string, error) {
	u := rawURL
	if rest, ok := strings.CutPrefix(u, "vara://"); ok {
		u = "http://" + rest
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("credstore: bad url %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("credstore: %q is not an http(s) server URL", rawURL)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("credstore: url %q has no host", rawURL)
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}
