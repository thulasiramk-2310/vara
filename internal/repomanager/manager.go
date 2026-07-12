package repomanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/repository"
)

// Errors returned by the Manager. The server maps them to control-plane status
// codes (RFC-0019 §8.2): ErrExists→409, ErrNotFound→404, ErrInvalidName→400.
var (
	ErrExists      = errors.New("repository already exists")
	ErrNotFound    = errors.New("repository not found")
	ErrInvalidName = errors.New("invalid repository name")
)

// ownerCaps is the capability set seeded for a repository's owner at creation
// (RFC-0019 §7.3): full data-plane access plus repository-scoped management.
// Server-scoped capabilities (create-repo/list-repos) are NOT here — they live
// only in _server.json, so an owner can never self-grant host-wide powers.
var ownerCaps = []authz.Capability{
	authz.CapRead, authz.CapCreateRef, authz.CapPush, authz.CapForcePush, authz.CapDeleteRef,
	authz.CapDeleteRepo, authz.CapRenameRepo, authz.CapAdmin,
}

// Manager performs the repository lifecycle by orchestrating three artifacts:
// content (repository.Init under reposRoot), policy (authz under policyRoot), and
// metadata (this package under metaDir). All mutation is serialized by mu so
// each lifecycle operation is atomic with respect to the others (v1 favors
// simplicity over control-plane throughput; these operations are rare).
type Manager struct {
	reposRoot  string
	policyRoot string
	metaDir    string

	mu     sync.Mutex
	byName map[string]*Metadata // includes tombstones (Creating/Deleting)
	byID   map[string]*Metadata
	loaded bool
}

// New builds a Manager. A policy root is REQUIRED: ownership is expressed by
// seeding policy (RFC-0019 §7.3), so a manager with nowhere to write policy
// could not give a creator administration of their own repository.
func New(reposRoot, policyRoot, metaDir string) (*Manager, error) {
	if policyRoot == "" {
		return nil, fmt.Errorf("repomanager: a policy root is required (ownership is seeded as policy)")
	}
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return nil, fmt.Errorf("repomanager: create metadata dir: %w", err)
	}
	return &Manager{
		reposRoot:  reposRoot,
		policyRoot: policyRoot,
		metaDir:    metaDir,
		byName:     make(map[string]*Metadata),
		byID:       make(map[string]*Metadata),
	}, nil
}

// load populates the in-memory index from disk once. Callers hold mu.
func (m *Manager) load() error {
	if m.loaded {
		return nil
	}
	metas, err := scanMeta(m.metaDir)
	if err != nil {
		return err
	}
	for _, md := range metas {
		m.byID[md.ID] = md
		m.byName[md.Name] = md
	}
	m.loaded = true
	return nil
}

func (m *Manager) contentPath(name string) string { return filepath.Join(m.reposRoot, name) }
func (m *Manager) policyPath(name string) string  { return authz.PolicyPath(m.policyRoot, name) }

// Create brings a new repository into existence as three artifacts, all-or-
// nothing (RFC-0019 §5.5, M6). The owner is the creating subject, seeded with
// ownerCaps. A crash mid-create leaves only a Creating tombstone (never served,
// never listed), which a later create of the same name reclaims.
func (m *Manager) Create(name, owner string, vis Visibility, desc string) (*Metadata, error) {
	if !ValidName(name) {
		return nil, ErrInvalidName
	}
	if vis == "" {
		vis = VisibilityPrivate
	}
	if !vis.valid() {
		return nil, fmt.Errorf("%w: invalid visibility %q", ErrInvalidName, vis)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}

	if existing, ok := m.byName[name]; ok {
		if existing.State.live() {
			return nil, ErrExists
		}
		// A leftover tombstone (crashed create/delete) — reclaim the name by
		// removing whatever artifacts remain, then proceed fresh.
		m.purge(existing)
	}

	now := time.Now().UTC()
	md := &Metadata{
		ID: newID(), Name: name, Owner: owner, Visibility: vis,
		State: StateCreating, Description: desc, CreatedAt: now, UpdatedAt: now,
	}

	// Step 1: metadata (Creating) — claims the name against concurrent creates.
	if err := writeMeta(m.metaDir, md); err != nil {
		return nil, err
	}
	m.byID[md.ID] = md
	m.byName[name] = md

	// Step 2: content.
	if _, err := repository.Init(m.contentPath(name)); err != nil {
		m.rollback(md, false, false)
		return nil, fmt.Errorf("create %q: init content: %w", name, err)
	}

	// Step 3: policy seed (owner administration).
	if err := authz.WritePolicy(m.policyPath(name), map[string][]authz.Capability{owner: ownerCaps}); err != nil {
		m.rollback(md, true, false)
		return nil, fmt.Errorf("create %q: seed policy: %w", name, err)
	}

	// Step 4: activate — the repository becomes servable (M10) only now.
	md.State = StateActive
	md.UpdatedAt = time.Now().UTC()
	if err := writeMeta(m.metaDir, md); err != nil {
		m.rollback(md, true, true)
		return nil, fmt.Errorf("create %q: activate: %w", name, err)
	}
	return md.clone(), nil
}

// Delete hard-deletes a repository, tombstoned first so a crash resumes rather
// than resurrects (RFC-0019 §5.6, M7). From the Deleting flip onward the
// repository is logically gone: the data plane refuses it and it leaves listings.
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return err
	}
	md, ok := m.byName[name]
	if !ok || !md.State.live() {
		return ErrNotFound
	}

	// Step 1: tombstone.
	md.State = StateDeleting
	md.UpdatedAt = time.Now().UTC()
	if err := writeMeta(m.metaDir, md); err != nil {
		return err
	}
	// Steps 2–4: remove content, policy, metadata.
	m.purge(md)
	return nil
}

// Rename changes a repository's name while preserving its ID (RFC-0019 §5.7,
// M11). It moves the name-addressed artifacts (content, policy) and updates the
// name field in the id-addressed metadata. The caller (server) is responsible
// for the dual-capability authorization (rename-repo on old + create-repo on
// server, §7.1) before invoking this.
func (m *Manager) Rename(oldName, newName string) (*Metadata, error) {
	if !ValidName(newName) {
		return nil, ErrInvalidName
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}
	md, ok := m.byName[oldName]
	if !ok || !md.State.live() {
		return nil, ErrNotFound
	}
	if oldName == newName {
		return md.clone(), nil
	}
	if existing, ok := m.byName[newName]; ok && existing.State.live() {
		return nil, ErrExists
	} else if ok {
		// Reclaim a tombstone occupying the target name.
		m.purge(existing)
	}

	if err := os.Rename(m.contentPath(oldName), m.contentPath(newName)); err != nil {
		return nil, fmt.Errorf("rename %q->%q: move content: %w", oldName, newName, err)
	}
	if err := os.Rename(m.policyPath(oldName), m.policyPath(newName)); err != nil {
		// Best-effort revert of the content move so the repository stays coherent.
		_ = os.Rename(m.contentPath(newName), m.contentPath(oldName))
		return nil, fmt.Errorf("rename %q->%q: move policy: %w", oldName, newName, err)
	}

	delete(m.byName, oldName)
	md.Name = newName
	md.UpdatedAt = time.Now().UTC()
	m.byName[newName] = md
	if err := writeMeta(m.metaDir, md); err != nil {
		return nil, err
	}
	return md.clone(), nil
}

// Get returns a live repository's metadata, or ErrNotFound for an absent or
// tombstoned one.
func (m *Manager) Get(name string) (*Metadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}
	md, ok := m.byName[name]
	if !ok || !md.State.live() {
		return nil, ErrNotFound
	}
	return md.clone(), nil
}

// List returns live repositories in a stable, deterministic order: ascending
// byte-order by name (RFC-0019 §5.8). Tombstones are excluded.
func (m *Manager) List() ([]*Metadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}
	out := make([]*Metadata, 0, len(m.byName))
	for _, md := range m.byName {
		if md.State.live() {
			out = append(out, md.clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Servable reports whether the data plane may serve name (RFC-0019 M10): true
// only for an Active repository. Archived and tombstoned repositories, and
// unknown names, are not servable.
func (m *Manager) Servable(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return false
	}
	md, ok := m.byName[name]
	return ok && md.State == StateActive
}

// rollback undoes a partial create (RFC-0019 §5.5). content/policy say which
// artifacts were successfully written and therefore must be removed; metadata is
// always removed. Callers hold mu.
func (m *Manager) rollback(md *Metadata, content, policy bool) {
	if policy {
		_ = os.Remove(m.policyPath(md.Name))
	}
	if content {
		_ = os.RemoveAll(m.contentPath(md.Name))
	}
	_ = removeMeta(m.metaDir, md.ID)
	delete(m.byID, md.ID)
	delete(m.byName, md.Name)
}

// purge removes all three artifacts for a repository unconditionally (used to
// finish a delete and to reclaim a tombstoned name). Callers hold mu.
func (m *Manager) purge(md *Metadata) {
	_ = os.RemoveAll(m.contentPath(md.Name))
	_ = os.Remove(m.policyPath(md.Name))
	_ = removeMeta(m.metaDir, md.ID)
	delete(m.byID, md.ID)
	delete(m.byName, md.Name)
}
