// Package refs implements VARA-RFC-0004.
//
// RFC:
// VARA-RFC-0004 References and Resolution
//
// Responsibilities:
// - Define the Resolver interface for HEAD, branches, and tags.
// - Implement FSResolver: filesystem-backed reference resolution.
// - Enforce reference name validation (RFC-0004 §3).
// - Use atomic lock-file + rename for all reference writes (RFC-0004 §4 steps 2–4).
//
// This package MUST NOT:
// - Import commands, transaction, locking, or any higher-layer package.
// - Perform lock acquisition (the caller holds refs.lock before mutating refs).
package refs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	varaerrors "github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Reference represents a resolved VARA reference.
type Reference struct {
	Name     string
	ObjectID types.CommitID
	// Target is set only for symbolic references (e.g. HEAD → refs/heads/main).
	Target string
}

// IsSymbolic returns true if the reference points to another reference.
func (r Reference) IsSymbolic() bool {
	return r.Target != ""
}

// Resolver defines the single authority for repository references.
type Resolver interface {
	Resolve(name string) (types.CommitID, error)
	Update(name string, id types.CommitID) error
	Create(name string, id types.CommitID) error
	Delete(name string) error
	Symbolic(name string, target string) error
	ResolveSymbolic(name string) (string, error)
	Detached(name string) (bool, error)
	List() ([]Reference, error)
}

// FSResolver implements Resolver using the local filesystem (.vara).
type FSResolver struct {
	VaraDir string
}

// NewFSResolver creates a new filesystem-backed reference resolver.
func NewFSResolver(varaDir string) *FSResolver {
	return &FSResolver{VaraDir: varaDir}
}

// refPath returns the absolute path for a reference name.
func (r *FSResolver) refPath(name string) string {
	return filepath.Join(r.VaraDir, filepath.FromSlash(name))
}

// ValidateName validates a branch or tag short name per RFC-0004 §3.
// name is the short name (e.g. "main", "feature/auth"), not the full ref path.
func ValidateName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("%w: name cannot be empty", varaerrors.ErrRefValidation)
	}
	if len(name) > 255 {
		return fmt.Errorf("%w: name exceeds 255 bytes", varaerrors.ErrRefValidation)
	}
	if name[0] == '/' || name[0] == '.' {
		return fmt.Errorf("%w: name cannot begin with '/' or '.'", varaerrors.ErrRefValidation)
	}
	if name[len(name)-1] == '/' || name[len(name)-1] == '.' {
		return fmt.Errorf("%w: name cannot end with '/' or '.'", varaerrors.ErrRefValidation)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: name cannot contain '..'", varaerrors.ErrRefValidation)
	}
	// Restricted characters per RFC-0004 §3.
	for _, ch := range []rune{'~', '^', ':', '?', '*', '[', '\\', ' '} {
		if strings.ContainsRune(name, ch) {
			return fmt.Errorf("%w: name contains restricted character %q", varaerrors.ErrRefValidation, ch)
		}
	}
	// No ASCII control characters.
	for _, b := range []byte(name) {
		if b < 0x20 || b == 0x7f {
			return fmt.Errorf("%w: name contains control character", varaerrors.ErrRefValidation)
		}
	}
	return nil
}

// Resolve looks up the commit ID for a reference, following symbolic links.
func (r *FSResolver) Resolve(name string) (types.CommitID, error) {
	visited := make(map[string]bool)
	current := name

	for {
		if visited[current] {
			return types.CommitID{}, fmt.Errorf("circular reference detected at %s", current)
		}
		visited[current] = true

		data, err := os.ReadFile(r.refPath(current))
		if err != nil {
			return types.CommitID{}, fmt.Errorf("%w: %s", varaerrors.ErrRefNotFound, current)
		}

		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "ref: ") {
			current = strings.TrimPrefix(content, "ref: ")
			continue
		}

		id, err := types.ParseHex(content)
		if err != nil {
			return types.CommitID{}, fmt.Errorf("invalid reference content in %s", current)
		}
		return types.CommitID(id), nil
	}
}

// Update changes an existing reference to point to a new commit.
// If the reference is symbolic, it follows it and updates the target.
// Uses atomic lock-file + rename (RFC-0004 §4 steps 2–4).
func (r *FSResolver) Update(name string, id types.CommitID) error {
	// Follow symbolic refs to find the actual ref file to update.
	current := name
	for {
		data, err := os.ReadFile(r.refPath(current))
		if err == nil {
			content := strings.TrimSpace(string(data))
			if strings.HasPrefix(content, "ref: ") {
				current = strings.TrimPrefix(content, "ref: ")
				continue
			}
		}
		break
	}

	path := r.refPath(current)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicWrite(path, id.String()+"\n")
}

// Create creates a new reference. Returns an error if it already exists.
// Uses atomic lock-file + rename (RFC-0004 §4 steps 2–4).
func (r *FSResolver) Create(name string, id types.CommitID) error {
	path := r.refPath(name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("reference already exists: %s", name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicWrite(path, id.String()+"\n")
}

// Delete removes a reference.
func (r *FSResolver) Delete(name string) error {
	return os.Remove(r.refPath(name))
}

// Symbolic creates or updates a symbolic reference (e.g., HEAD → refs/heads/main).
func (r *FSResolver) Symbolic(name string, target string) error {
	path := r.refPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicWrite(path, "ref: "+target+"\n")
}

// ResolveSymbolic reads the direct target of a symbolic reference without following it.
func (r *FSResolver) ResolveSymbolic(name string) (string, error) {
	data, err := os.ReadFile(r.refPath(name))
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref: ") {
		return strings.TrimPrefix(content, "ref: "), nil
	}
	return "", fmt.Errorf("not a symbolic reference: %s", name)
}

// Detached returns true if the reference directly contains a commit ID (not symbolic).
func (r *FSResolver) Detached(name string) (bool, error) {
	data, err := os.ReadFile(r.refPath(name))
	if err != nil {
		return false, err
	}
	content := strings.TrimSpace(string(data))
	return !strings.HasPrefix(content, "ref: "), nil
}

// List returns all references under refs/.
func (r *FSResolver) List() ([]Reference, error) {
	var refs []Reference
	refsDir := filepath.Join(r.VaraDir, "refs")

	err := filepath.Walk(refsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Skip atomic temp files.
		if strings.HasSuffix(path, ".lock") {
			return nil
		}

		rel, err := filepath.Rel(r.VaraDir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)

		id, err := r.Resolve(name)
		if err != nil {
			return nil // Skip invalid refs.
		}

		refs = append(refs, Reference{
			Name:     name,
			ObjectID: id,
		})
		return nil
	})

	return refs, err
}

// atomicWrite writes content to path using the lock-file + rename protocol
// (RFC-0004 §4 steps 2–4): write to <path>.lock, sync, rename to <path>.
func atomicWrite(path, content string) error {
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(lockPath, path); err != nil {
		os.Remove(lockPath)
		return err
	}
	return nil
}
