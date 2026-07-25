package hub

// This file is the RFC-0022 browse projection: tree listings, file content, and
// path-filtered commit history. Like the rest of internal/hub it only reads
// engine objects (trees, blobs, commits) and returns plain Go structs — it adds
// no traversal or storage logic and changes no engine (B2/B7). Path resolution
// walks tree OBJECTS segment by segment, never the host filesystem, so ".." and
// absolute paths are ordinary non-existent entry names, not traversal (B4).

import (
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// modeSubtree is the single mode that marks a directory (a sub-tree); every other
// mode is a file. This mirrors the engine's own convention (pkg/transfer).
const modeSubtree = 0o040000

// TreeEntry is a projected tree entry (RFC-0022 §5.1): a child's name, raw mode,
// object id, and whether it is a directory. Size is intentionally absent — it
// would cost a blob read per entry (§5.1, B6).
type TreeEntry struct {
	Name  string
	Mode  uint32
	ID    types.ObjectID
	IsDir bool
}

// TreeListing is a projected directory listing at a resolved commit.
type TreeListing struct {
	Commit  types.CommitID
	Tree    types.TreeID
	Entries []TreeEntry
}

// BlobContent is a projected file: the commit it was read at, its blob id (content
// address), and its exact bytes. The server decides text/binary and truncation as
// a presentation concern; the bytes and id here are the engine's, unaltered (B2).
type BlobContent struct {
	Commit types.CommitID
	ID     types.BlobID
	Data   []byte
}

// splitPath splits a browse path into non-empty segments. Leading, trailing, and
// repeated slashes yield no empty segments, and the empty path yields none (the
// root). A segment like ".." is returned verbatim: it is looked up as an entry
// name and simply does not exist, so there is no filesystem to escape (B4).
func splitPath(p string) []string {
	var segs []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// loadTree reads a tree object by id, mapping any miss/type error to ErrNotFound.
func (r *Repo) loadTree(id types.TreeID) (*object.Tree, error) {
	obj, err := r.store.Read(types.ObjectID(id))
	if err != nil {
		return nil, ErrNotFound
	}
	t, ok := obj.(*object.Tree)
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

// commitRoot returns a commit's root tree id, or false if it is missing/not a
// commit.
func (r *Repo) commitRoot(id types.CommitID) (types.TreeID, bool) {
	obj, err := r.store.Read(types.ObjectID(id))
	if err != nil {
		return types.TreeID{}, false
	}
	c, ok := obj.(*object.Commit)
	if !ok {
		return types.TreeID{}, false
	}
	return c.TreeHash, true
}

// findChild returns the entry named name in t, or false.
func findChild(t *object.Tree, name string) (object.TreeEntry, bool) {
	for _, e := range t.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return object.TreeEntry{}, false
}

// resolveEntry walks segs (non-empty) within root, returning the final entry. A
// non-final segment that is missing or not a sub-tree, or a missing final
// segment, is ErrNotFound.
func (r *Repo) resolveEntry(root types.TreeID, segs []string) (object.TreeEntry, error) {
	cur := root
	for i, seg := range segs {
		t, err := r.loadTree(cur)
		if err != nil {
			return object.TreeEntry{}, err
		}
		e, ok := findChild(t, seg)
		if !ok {
			return object.TreeEntry{}, ErrNotFound
		}
		if i == len(segs)-1 {
			return e, nil
		}
		if e.Mode != modeSubtree {
			return object.TreeEntry{}, ErrNotFound // a file cannot be a path prefix
		}
		cur = types.TreeID(e.Hash)
	}
	return object.TreeEntry{}, ErrNotFound // unreachable: segs is non-empty
}

// Tree lists the entries of the tree at path in ref (empty path = root). A path
// that resolves to a file, or does not exist, is ErrNotFound. It costs one tree
// read plus one per path segment — never a blob read (§5.1).
func (r *Repo) Tree(ref, path string) (*TreeListing, error) {
	cid, err := r.resolveStart(ref)
	if err != nil {
		return nil, ErrNotFound
	}
	root, ok := r.commitRoot(cid)
	if !ok {
		return nil, ErrNotFound
	}
	treeID := root
	if segs := splitPath(path); len(segs) > 0 {
		e, err := r.resolveEntry(root, segs)
		if err != nil {
			return nil, err
		}
		if e.Mode != modeSubtree {
			return nil, ErrNotFound // a file path to `tree` is 404
		}
		treeID = types.TreeID(e.Hash)
	}
	t, err := r.loadTree(treeID)
	if err != nil {
		return nil, err
	}
	out := &TreeListing{Commit: cid, Tree: treeID, Entries: make([]TreeEntry, 0, len(t.Entries))}
	for _, e := range t.Entries {
		out.Entries = append(out.Entries, TreeEntry{
			Name: e.Name, Mode: e.Mode, ID: e.Hash, IsDir: e.Mode == modeSubtree,
		})
	}
	return out, nil
}

// Blob returns the file at path in ref. A directory path, an empty path (the
// root is a directory), or a missing path is ErrNotFound.
func (r *Repo) Blob(ref, path string) (*BlobContent, error) {
	cid, err := r.resolveStart(ref)
	if err != nil {
		return nil, ErrNotFound
	}
	root, ok := r.commitRoot(cid)
	if !ok {
		return nil, ErrNotFound
	}
	segs := splitPath(path)
	if len(segs) == 0 {
		return nil, ErrNotFound // the root is a directory, not a blob
	}
	e, err := r.resolveEntry(root, segs)
	if err != nil {
		return nil, err
	}
	if e.Mode == modeSubtree {
		return nil, ErrNotFound // a directory path to `blob` is 404
	}
	obj, err := r.store.Read(e.Hash)
	if err != nil {
		return nil, ErrNotFound
	}
	b, ok := obj.(*object.Blob)
	if !ok {
		return nil, ErrNotFound
	}
	return &BlobContent{Commit: cid, ID: types.BlobID(e.Hash), Data: b.Data}, nil
}

// objIDInTree returns the object id at segs within root, or false if absent.
// segs may be empty (the root tree itself).
func (r *Repo) objIDInTree(root types.TreeID, segs []string) (types.ObjectID, bool) {
	if len(segs) == 0 {
		return types.ObjectID(root), true
	}
	e, err := r.resolveEntry(root, segs)
	if err != nil {
		return types.ObjectID{}, false
	}
	return e.Hash, true
}

// changedPath reports whether commit c changed the path segs, comparing the
// object id at that path against the FIRST parent (RFC-0022 §5.4). A root commit
// (no parents) that contains the path counts as its introduction; for a merge,
// only the first parent is compared (the git log -- path simplification).
func (r *Repo) changedPath(c *object.Commit, segs []string) bool {
	cur, curOK := r.objIDInTree(c.TreeHash, segs)
	if len(c.Parents) == 0 {
		return curOK
	}
	var par types.ObjectID
	var parOK bool
	if proot, ok := r.commitRoot(c.Parents[0]); ok {
		par, parOK = r.objIDInTree(proot, segs)
	}
	if curOK != parOK {
		return true // appeared or disappeared
	}
	if !curOK {
		return false // absent in both
	}
	return cur != par
}

// CommitsForPath is Commits filtered to commits that changed path (RFC-0022
// §5.4): a file's history. Cursoring is over the UNDERLYING walk — before/next are
// raw-walk commit ids and the path filter is re-applied after resuming — so pages
// never skip or repeat. A page may traverse many commits to yield limit matches.
func (r *Repo) CommitsForPath(ref, path string, limit int, before types.CommitID) ([]Commit, types.CommitID, error) {
	start, err := r.resolveStart(ref)
	if err != nil {
		return nil, types.CommitID{}, ErrNotFound
	}
	segs := splitPath(path)
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
		if !r.changedPath(c, segs) {
			continue
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
