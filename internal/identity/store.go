package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The credential store is owned solely by the identity layer (RFC-0020 §9, S13)
// and lives outside any repository (S2). v1 is file-backed behind two interfaces
// so a database backend can replace it later without touching the sources or the
// control plane. Secrets are stored only as hashes (§6).

var (
	ErrNoAccount     = errors.New("no such account")
	ErrAccountExists = errors.New("account already exists")
	ErrNoCredential  = errors.New("no such credential")
)

// AccountStore persists accounts, one per username.
type AccountStore interface {
	Create(*Account) error // ErrAccountExists if the username is taken
	Get(username string) (*Account, error)
	Update(*Account) error
	Delete(username string) error
}

// CredentialStore persists sessions and API tokens. The hot path is
// GetBySecretHash (one per authenticated request); listing and id-based
// revocation are cold admin paths.
type CredentialStore interface {
	Put(*BearerCredential) error
	GetBySecretHash(hash string) (*BearerCredential, error)
	Touch(hash string, t time.Time) error
	ListBySubject(subject string) ([]*BearerCredential, error)
	RevokeByID(subject, id string) error
	DeleteBySecretHash(hash string) error
	RevokeAllForSubject(subject string) error
}

// --- file-backed account store ------------------------------------------------

type fileAccountStore struct {
	dir string
	mu  sync.Mutex
}

func newFileAccountStore(dir string) (*fileAccountStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &fileAccountStore{dir: dir}, nil
}

func (s *fileAccountStore) path(username string) (string, error) {
	if !ValidUsername(username) {
		return "", fmt.Errorf("%w: invalid username", ErrNoAccount)
	}
	return filepath.Join(s.dir, username+".json"), nil
}

func (s *fileAccountStore) Create(a *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(a.Username)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return ErrAccountExists
	}
	return writeJSONAtomic(p, a)
}

func (s *fileAccountStore) Get(username string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(username)
	if err != nil {
		return nil, err
	}
	var a Account
	if err := readJSON(p, &a); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoAccount
		}
		return nil, err
	}
	return &a, nil
}

func (s *fileAccountStore) Update(a *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(a.Username)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return ErrNoAccount
	}
	return writeJSONAtomic(p, a)
}

func (s *fileAccountStore) Delete(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(username)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// --- file-backed credential store ---------------------------------------------
//
// Files are named by the secret hash, so authenticating a bearer request is one
// O(1) read (hash the presented secret, open the file). Listing and id-based
// revocation scan the directory — acceptable for the admin paths they serve.

type fileCredentialStore struct {
	dir string
	mu  sync.Mutex
}

func newFileCredentialStore(dir string) (*fileCredentialStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &fileCredentialStore{dir: dir}, nil
}

func (s *fileCredentialStore) pathForHash(hash string) string {
	return filepath.Join(s.dir, hash+".json")
}

func (s *fileCredentialStore) Put(c *BearerCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONAtomic(s.pathForHash(c.SecretHash), c)
}

func (s *fileCredentialStore) GetBySecretHash(hash string) (*BearerCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c BearerCredential
	if err := readJSON(s.pathForHash(hash), &c); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoCredential
		}
		return nil, err
	}
	return &c, nil
}

func (s *fileCredentialStore) Touch(hash string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c BearerCredential
	if err := readJSON(s.pathForHash(hash), &c); err != nil {
		return err
	}
	c.LastUsedAt = t
	return writeJSONAtomic(s.pathForHash(hash), &c)
}

func (s *fileCredentialStore) forEach(fn func(*BearerCredential, string) error) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		p := filepath.Join(s.dir, name)
		var c BearerCredential
		if err := readJSON(p, &c); err != nil {
			continue // skip unreadable/partial entries
		}
		if err := fn(&c, p); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileCredentialStore) ListBySubject(subject string) ([]*BearerCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*BearerCredential
	err := s.forEach(func(c *BearerCredential, _ string) error {
		if c.Subject == subject {
			cc := *c
			out = append(out, &cc)
		}
		return nil
	})
	return out, err
}

func (s *fileCredentialStore) RevokeByID(subject, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	err := s.forEach(func(c *BearerCredential, p string) error {
		if c.Subject == subject && c.ID == id {
			found = true
			return os.Remove(p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		return ErrNoCredential
	}
	return nil
}

func (s *fileCredentialStore) DeleteBySecretHash(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.pathForHash(hash)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *fileCredentialStore) RevokeAllForSubject(subject string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forEach(func(c *BearerCredential, p string) error {
		if c.Subject == subject {
			return os.Remove(p)
		}
		return nil
	})
}

// --- json helpers (atomic write) ----------------------------------------------

func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
