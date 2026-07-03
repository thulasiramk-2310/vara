package transport

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/transfer"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Local is a filesystem transport (RFC-0014 §6). It opens the peer's `.vara`
// directory directly and operates on its object store and ref store.
type Local struct {
	varaDir string
	store   *object.Store
	refs    *refs.FSResolver
}

// Open resolves url to a local repository and returns a Local transport. The
// url is a filesystem path or a file:// URL pointing at the repository's
// working root (the directory containing .vara). It is an error if that
// directory does not already contain a valid VARA repository (RFC-0014 §11).
func Open(url string) (*Local, error) {
	root, err := ParsePath(url)
	if err != nil {
		return nil, err
	}
	varaDir := filepath.Join(root, repository.VaraDir)
	stat, err := os.Stat(varaDir)
	if err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("no VARA repository at %q", root)
	}
	return &Local{
		varaDir: varaDir,
		// NOTE: VARA's object store writes under the .vara root directly
		// (`.vara/<xx>/<rest>`), matching every command's
		// object.NewStore(VaraDir) call. The `objects/` subdir that Init
		// creates is currently unused. The transport MUST use the same
		// convention or it will not interoperate with real repositories.
		store: object.NewStore(varaDir),
		refs:  refs.NewFSResolver(varaDir),
	}, nil
}

// ParsePath converts a remote URL (path or file:// URL) into an absolute local
// filesystem path. Non-local schemes (http, vara, ssh) are rejected here and
// are the domain of a future network transport (RFC-0016).
func ParsePath(url string) (string, error) {
	p := url
	if rest, ok := strings.CutPrefix(p, "file://"); ok {
		p = rest
		// file:///abs on Unix; on Windows file:///C:/... -> strip leading slash.
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
	} else if scheme, _, ok := strings.Cut(p, "://"); ok {
		return "", fmt.Errorf("unsupported remote scheme %q (only local paths and file:// are supported in v0.2)", scheme)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// ListRefs returns every branch under refs/heads on the peer. Remote-tracking
// refs and tags are not advertised for fetch in v0.2.
func (l *Local) ListRefs() ([]RefAdvertisement, error) {
	all, err := l.refs.List()
	if err != nil {
		return nil, err
	}
	var out []RefAdvertisement
	for _, r := range all {
		if !strings.HasPrefix(r.Name, "refs/heads/") {
			continue
		}
		out = append(out, RefAdvertisement{Name: r.Name, Target: r.ObjectID})
	}
	return out, nil
}

// HeadTarget returns the ref name the peer's HEAD points at.
func (l *Local) HeadTarget() (string, error) {
	return l.refs.ResolveSymbolic("HEAD")
}

// FetchPack enumerates wants-minus-haves and returns a VPCK stream.
func (l *Local) FetchPack(wants, haves []types.CommitID) (io.ReadCloser, error) {
	ids, err := transfer.Enumerate(l.store, wants, haves)
	if err != nil {
		return nil, fmt.Errorf("enumerate: %w", err)
	}
	var buf bytes.Buffer
	if err := transfer.WritePack(l.store, ids, &buf); err != nil {
		return nil, fmt.Errorf("write pack: %w", err)
	}
	return io.NopCloser(&buf), nil
}

// ReceivePack ingests the pack into the peer's store, then applies each ref
// update under compare-and-swap and fast-forward rules (RFC-0014 §9.4, §10).
func (l *Local) ReceivePack(pack io.Reader, updates []RefUpdate) ([]RefUpdateResult, error) {
	if _, err := transfer.ReadPack(l.store, pack); err != nil {
		return nil, fmt.Errorf("ingest pack: %w", err)
	}
	results := make([]RefUpdateResult, 0, len(updates))
	for _, u := range updates {
		results = append(results, l.applyUpdate(u))
	}
	return results, nil
}

// applyUpdate performs one reference update with CAS + fast-forward checks.
func (l *Local) applyUpdate(u RefUpdate) RefUpdateResult {
	reject := func(reason string) RefUpdateResult {
		return RefUpdateResult{Name: u.Name, OK: false, Reason: reason}
	}

	// Current value on the peer (zero if the ref does not exist).
	current, err := l.refs.Resolve(u.Name)
	if err != nil {
		current = zeroCommit
	}

	// Compare-and-swap against the value the caller expected.
	if current != u.Old {
		return reject(fmt.Sprintf("stale: %s moved to %s since ref discovery", u.Name, shortID(current)))
	}

	// Ensure the new commit actually landed in the store.
	if _, err := l.store.Read(types.ObjectID(u.New)); err != nil {
		return reject(fmt.Sprintf("new commit %s missing from pack", shortID(u.New)))
	}

	// Fast-forward check unless forced or the ref is new.
	if !u.Force && current != zeroCommit {
		base, err := graph.MergeBase(l.store, current, u.New)
		if err != nil || base != current {
			return reject(fmt.Sprintf("non-fast-forward: %s is not an ancestor of %s", shortID(current), shortID(u.New)))
		}
	}

	// Apply.
	if current == zeroCommit {
		if err := l.refs.Create(u.Name, u.New); err != nil {
			return reject(fmt.Sprintf("create ref: %v", err))
		}
	} else if err := l.refs.Update(u.Name, u.New); err != nil {
		return reject(fmt.Sprintf("update ref: %v", err))
	}
	return RefUpdateResult{Name: u.Name, OK: true}
}

// Close is a no-op for the local transport.
func (l *Local) Close() error { return nil }

func shortID(c types.CommitID) string {
	s := c.String()
	if c == zeroCommit {
		return "(none)"
	}
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
