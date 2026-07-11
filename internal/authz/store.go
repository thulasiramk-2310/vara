package authz

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is the server-managed policy store: one JSON file per repository under a
// root directory OUTSIDE any repository (RFC-0018 §7, A3). It caches parsed
// policy and reloads atomically when a file changes (RFC-0018 §7.3).
type Store struct {
	root   string
	mu     sync.Mutex
	byRepo map[string]cacheEntry
}

type cacheEntry struct {
	policy  *Policy
	modTime time.Time
}

// NewStore builds a policy store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{root: dir, byRepo: make(map[string]cacheEntry)}
}

// PolicyFor returns the policy for repo. A missing file yields DenyAll (a
// configured store with no policy for a repo denies everything — default deny,
// A4). A parse/validation failure returns the previously-loaded policy if one
// exists (atomic reload: a broken edit never replaces a working policy, §7.3),
// or DenyAll plus the error on first load (fail closed).
func (s *Store) PolicyFor(repo string) (*Policy, error) {
	path := filepath.Join(s.root, repo+".json")
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return DenyAll, nil
	}
	if err != nil {
		return DenyAll, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.byRepo[repo]; ok && e.modTime.Equal(fi.ModTime()) {
		return e.policy, nil
	}

	p, err := LoadPolicy(path)
	if err != nil {
		// Keep the last good policy in force; report the error so it is logged.
		if e, ok := s.byRepo[repo]; ok {
			return e.policy, err
		}
		return DenyAll, err
	}
	s.byRepo[repo] = cacheEntry{policy: p, modTime: fi.ModTime()}
	return p, nil
}
