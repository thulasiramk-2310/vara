// Package hub is the RFC-0021 projection layer: it reads repository content
// (branches, commits, summary) through the existing engine primitives — refs,
// the commit graph, and the object store — and returns plain Go structs. It is
// the concrete expression of RFC-0021 H8: every value it returns is a projection
// of engine state, never an alternate representation. It knows nothing about HTTP
// or the wire format (the server maps these structs to JSON), and it adds no
// traversal or storage logic of its own.
//
// Layer: above the engine read packages (object, refs, graph), below
// internal/server. It imports no binding layer and changes no engine.
package hub

import (
	"errors"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Errors returned by the projection layer. The server maps them to status codes
// (RFC-0021 §10): ErrNotFound→404, ErrBadCursor→400.
var (
	ErrNotFound  = errors.New("not found")
	ErrBadCursor = errors.New("invalid cursor")
)

const branchPrefix = "refs/heads/"

// Repo is a read-only view of one repository's content, opened over its .vara
// directory.
type Repo struct {
	store *object.Store
	refs  *refs.FSResolver
}

// Open builds a read view over a repository's .vara directory. It does not touch
// the filesystem until a projection method is called.
func Open(varaDir string) *Repo {
	return &Repo{store: object.NewStore(varaDir), refs: refs.NewFSResolver(varaDir)}
}

// Branch is a projected branch: its short name, the commit it points at, and
// whether it is HEAD.
type Branch struct {
	Name   string
	Target types.CommitID
	IsHead bool
}

// Commit is a projected commit summary.
type Commit struct {
	ID        types.CommitID
	Message   string
	Author    string
	Timestamp int64
	Parents   []types.CommitID
}

// CommitDetail adds the tree id to a commit summary.
type CommitDetail struct {
	Commit
	Tree types.TreeID
}

// Summary is the projected repository summary (RFC-0021 §5.5).
type Summary struct {
	DefaultBranch string
	Head          types.CommitID
	CommitCount   int
	BranchCount   int
	Last          *Commit
}

// headTarget returns the symbolic HEAD target ref (e.g. "refs/heads/main"), or ""
// if HEAD is unresolved.
func (r *Repo) headTarget() string {
	t, err := r.refs.ResolveSymbolic("HEAD")
	if err != nil {
		return ""
	}
	return t
}

// Branches lists the repository's branches in stable name order, flagging HEAD.
func (r *Repo) Branches() ([]Branch, error) {
	list, err := r.refs.List()
	if err != nil {
		return nil, err
	}
	head := r.headTarget()
	var out []Branch
	for _, ref := range list {
		if ref.IsSymbolic() || !strings.HasPrefix(ref.Name, branchPrefix) {
			continue
		}
		target, err := r.refs.Resolve(ref.Name)
		if err != nil {
			continue // a ref that does not resolve is skipped, not fatal
		}
		out = append(out, Branch{
			Name:   strings.TrimPrefix(ref.Name, branchPrefix),
			Target: target,
			IsHead: ref.Name == head,
		})
	}
	return out, nil
}

// resolveStart resolves the starting commit for a history walk. ref may be "" (
// HEAD), a short branch name, or a full ref name.
func (r *Repo) resolveStart(ref string) (types.CommitID, error) {
	if ref == "" {
		return r.refs.Resolve("HEAD")
	}
	if id, err := r.refs.Resolve(ref); err == nil {
		return id, nil
	}
	// Try as a short branch name.
	return r.refs.Resolve(branchPrefix + ref)
}

// Commits returns up to limit commits reachable from ref (default HEAD) in graph
// order (newest first), starting at the cursor `before` when non-zero. The
// returned next cursor is the id at which the following page begins, or the zero
// id at the end. A non-zero `before` that is not reachable is ErrBadCursor.
func (r *Repo) Commits(ref string, limit int, before types.CommitID) ([]Commit, types.CommitID, error) {
	start, err := r.resolveStart(ref)
	if err != nil {
		return nil, types.CommitID{}, ErrNotFound
	}
	zero := types.CommitID{}
	w := graph.NewWalker(r.store, start)
	collecting := before == zero
	var out []Commit
	var next types.CommitID
	for {
		c, id, err := w.Next()
		if err != nil {
			return nil, types.CommitID{}, err
		}
		if id == zero {
			break // walk exhausted
		}
		if !collecting {
			if id == before {
				collecting = true
			} else {
				continue
			}
		}
		if len(out) == limit {
			next = id
			break
		}
		out = append(out, toCommit(c, id))
	}
	if before != zero && !collecting {
		return nil, types.CommitID{}, ErrBadCursor
	}
	return out, next, nil
}

// Commit returns one commit's detail, or ErrNotFound.
func (r *Repo) Commit(id types.CommitID) (*CommitDetail, error) {
	obj, err := r.store.Read(types.ObjectID(id))
	if err != nil {
		return nil, ErrNotFound
	}
	c, ok := obj.(*object.Commit)
	if !ok {
		return nil, ErrNotFound
	}
	return &CommitDetail{Commit: toCommit(c, id), Tree: c.TreeHash}, nil
}

// Summary composes a one-call overview (RFC-0021 §5.5). commit_count is a walk
// from HEAD; a caller that hits this often benefits from the ETag (304) rather
// than re-walking.
func (r *Repo) Summary() (*Summary, error) {
	branches, err := r.Branches()
	if err != nil {
		return nil, err
	}
	s := &Summary{
		DefaultBranch: strings.TrimPrefix(r.headTarget(), branchPrefix),
		BranchCount:   len(branches),
	}
	head, err := r.refs.Resolve("HEAD")
	if err != nil {
		return s, nil // an unborn/empty repository has a valid, mostly-zero summary
	}
	s.Head = head
	if last, err := r.Commit(head); err == nil {
		lc := last.Commit
		s.Last = &lc
	}
	// Count reachable commits via the same walk the history endpoint uses.
	w := graph.NewWalker(r.store, head)
	for {
		_, id, err := w.Next()
		if err != nil {
			return nil, err
		}
		if (id == types.CommitID{}) {
			break
		}
		s.CommitCount++
	}
	return s, nil
}

func toCommit(c *object.Commit, id types.CommitID) Commit {
	return Commit{
		ID: id, Message: c.Message, Author: c.Author,
		Timestamp: c.Timestamp, Parents: c.Parents,
	}
}
