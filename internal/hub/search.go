package hub

// This file is the RFC-0024 search projection: commit search (message/author
// across history), path search (file names in the tree at a ref), and content
// search (grep matching text lines at a ref). Like the rest of internal/hub it
// only reads engine objects and returns plain Go structs — it builds NO persistent
// index (S2): every query is a live, bounded scan of the graph, the trees, and the
// blobs. Matching is literal substring (linear), never a backtracking pattern, and
// every scan is bounded on every axis (S5) so no request can run unbounded work.
// Binary blobs are skipped before any byte is matched (S4, reusing diffText).

import (
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Search bounds (RFC-0024 §7). Package vars, not consts, only so tests can shrink
// them without large fixtures.
var (
	maxSearchWalk      = 5000     // commits examined per commit-search request → next/truncated
	maxSearchEntries   = 50000    // tree entries visited per path/content walk → truncated
	maxSearchFileBytes = 1 << 20  // 1 MiB — per-file cap for content search (larger files skipped)
	maxSearchScanBytes = 64 << 20 // 64 MiB — total bytes scanned per content-search request → truncated
	maxSearchLines     = 50       // matching lines returned per file (content search)
)

// SetMaxSearchWalk / SetMaxSearchEntries / SetMaxSearchScanBytes /
// SetMaxSearchFileBytes are test hooks that bound a scan (RFC-0024 §7) without
// large fixtures; each returns the previous value so a test can restore it.
func SetMaxSearchWalk(n int) int      { old := maxSearchWalk; maxSearchWalk = n; return old }
func SetMaxSearchEntries(n int) int   { old := maxSearchEntries; maxSearchEntries = n; return old }
func SetMaxSearchScanBytes(n int) int { old := maxSearchScanBytes; maxSearchScanBytes = n; return old }
func SetMaxSearchFileBytes(n int) int { old := maxSearchFileBytes; maxSearchFileBytes = n; return old }

// matcher is a compiled literal-substring test (RFC-0024 §6): case-insensitive by
// default (query and target lower-cased before comparison), or an exact byte match
// when caseSensitive. There is no pattern engine, so the match itself is linear.
type matcher struct {
	needle string
	fold   bool
}

func newMatcher(query string, caseSensitive bool) matcher {
	if caseSensitive {
		return matcher{needle: query}
	}
	return matcher{needle: strings.ToLower(query), fold: true}
}

func (m matcher) match(s string) bool {
	if m.fold {
		return strings.Contains(strings.ToLower(s), m.needle)
	}
	return strings.Contains(s, m.needle)
}

// --- commit search (§5.1) ------------------------------------------------------

// CommitSearchResult is the projected result of a commit search.
type CommitSearchResult struct {
	RefCommit types.CommitID
	Matches   []Commit
	Next      types.CommitID
	Truncated bool
}

// SearchCommits walks history from ref (default HEAD), newest first — the same walk
// as Commits — and returns commits whose message and/or author contains query
// (inMsg/inAuthor select the fields). Paging follows the underlying-walk cursor
// model (before/next are raw-walk ids, the filter re-applied after resuming). The
// walk is bounded by a traversal budget: if it is exhausted before limit matches,
// Next names the first UN-examined commit and Truncated is set, so a resume neither
// skips nor repeats a match.
func (r *Repo) SearchCommits(ref, query string, caseSensitive, inMsg, inAuthor bool, limit int, before types.CommitID) (*CommitSearchResult, error) {
	start, err := r.resolveStart(ref)
	if err != nil {
		return nil, ErrNotFound
	}
	m := newMatcher(query, caseSensitive)
	zero := types.CommitID{}
	w := graph.NewWalker(r.store, start)
	collecting := before == zero
	res := &CommitSearchResult{RefCommit: start}
	walked := 0
	for {
		c, id, err := w.Next()
		if err != nil {
			return nil, err
		}
		if id == zero {
			break // walk exhausted — no more history, Next stays zero
		}
		if !collecting {
			if id == before {
				collecting = true
			} else {
				continue
			}
		}
		if walked >= maxSearchWalk {
			res.Next = id // first un-examined commit — resume point
			res.Truncated = true
			break
		}
		walked++
		if !matchCommit(c, m, inMsg, inAuthor) {
			continue
		}
		if len(res.Matches) == limit {
			res.Next = id // page full — id is the first unconsumed match
			break
		}
		res.Matches = append(res.Matches, toCommit(c, id))
	}
	if before != zero && !collecting {
		return nil, ErrBadCursor
	}
	return res, nil
}

func matchCommit(c *object.Commit, m matcher, inMsg, inAuthor bool) bool {
	if inMsg && m.match(c.Message) {
		return true
	}
	if inAuthor && m.match(c.Author) {
		return true
	}
	return false
}

// --- shared bounded tree walk --------------------------------------------------

// walkState carries the entry budget across a recursive tree walk.
type walkState struct {
	budget    int
	visited   int
	truncated bool
}

// walkTree visits every entry under treeID in sorted (deterministic) pre-order,
// calling visit(path, entry). It returns false to stop the whole walk when the
// entry budget is exhausted (setting truncated) or the visitor returns false. An
// unreadable subtree is skipped, not fatal. Path resolution is over tree OBJECTS,
// never the filesystem (B4).
func (r *Repo) walkTree(treeID types.TreeID, prefix string, st *walkState, visit func(path string, e object.TreeEntry) bool) bool {
	t, err := r.loadTree(treeID)
	if err != nil {
		return true // skip an unreadable subtree, keep walking the rest
	}
	entries := make([]object.TreeEntry, len(t.Entries))
	copy(entries, t.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	for _, e := range entries {
		if st.visited >= st.budget {
			st.truncated = true
			return false
		}
		st.visited++
		path := e.Name
		if prefix != "" {
			path = prefix + "/" + e.Name
		}
		if !visit(path, e) {
			return false
		}
		if e.Mode == modeSubtree {
			if !r.walkTree(types.TreeID(e.Hash), path, st, visit) {
				return false
			}
		}
	}
	return true
}

// refTree resolves ref to a commit and its root tree (RFC-0024 §6).
func (r *Repo) refTree(ref string) (types.CommitID, types.TreeID, error) {
	cid, err := r.resolveStart(ref)
	if err != nil {
		return types.CommitID{}, types.TreeID{}, ErrNotFound
	}
	root, ok := r.commitRoot(cid)
	if !ok {
		return types.CommitID{}, types.TreeID{}, ErrNotFound
	}
	return cid, root, nil
}

// --- path search (§5.2) --------------------------------------------------------

// PathMatch is one matching tree entry: its full path, object id, raw mode, and
// whether it is a directory. It costs no blob read (§5.2).
type PathMatch struct {
	Path  string
	ID    types.ObjectID
	Mode  uint32
	IsDir bool
}

// PathSearchResult is the projected result of a path search.
type PathSearchResult struct {
	RefCommit types.CommitID
	Tree      types.TreeID
	Matches   []PathMatch
	Truncated bool
}

// SearchPaths returns tree entries whose full path contains query, at ref. It reads
// only tree objects — never a blob (§5.2). Bounded by the entry budget and limit;
// results are sorted by path.
func (r *Repo) SearchPaths(ref, query string, caseSensitive bool, limit int) (*PathSearchResult, error) {
	cid, root, err := r.refTree(ref)
	if err != nil {
		return nil, err
	}
	m := newMatcher(query, caseSensitive)
	res := &PathSearchResult{RefCommit: cid, Tree: root}
	st := &walkState{budget: maxSearchEntries}
	capped := false
	r.walkTree(root, "", st, func(path string, e object.TreeEntry) bool {
		if !m.match(path) {
			return true
		}
		if len(res.Matches) == limit {
			capped = true // a match beyond the cap exists — genuinely truncated
			return false
		}
		res.Matches = append(res.Matches, PathMatch{
			Path: path, ID: e.Hash, Mode: e.Mode, IsDir: e.Mode == modeSubtree,
		})
		return true
	})
	res.Truncated = st.truncated || capped
	sort.Slice(res.Matches, func(i, j int) bool { return res.Matches[i].Path < res.Matches[j].Path })
	return res, nil
}

// --- content search (§5.3) -----------------------------------------------------

// ContentLine is one matching line of a file: its 1-based number and inert content.
type ContentLine struct {
	Line    int
	Content string
}

// ContentMatch is one file with at least one matching line.
type ContentMatch struct {
	Path  string
	Blob  types.BlobID
	Lines []ContentLine
}

// ContentSearchResult is the projected result of a content search.
type ContentSearchResult struct {
	RefCommit types.CommitID
	Tree      types.TreeID
	Matches   []ContentMatch
	Truncated bool
}

// SearchContent returns files at ref containing a line that matches query, with the
// matching lines. Only text blobs are scanned (binary skipped before any byte is
// read into a string, §7); a file over the per-file cap is skipped; the scan stops
// on the bytes budget, the per-file line cap, or limit, setting Truncated with
// partial results. Results are sorted by path.
func (r *Repo) SearchContent(ref, query string, caseSensitive bool, limit int) (*ContentSearchResult, error) {
	cid, root, err := r.refTree(ref)
	if err != nil {
		return nil, err
	}
	m := newMatcher(query, caseSensitive)
	res := &ContentSearchResult{RefCommit: cid, Tree: root}
	st := &walkState{budget: maxSearchEntries}
	scanned := 0
	capped, budgetHit := false, false
	r.walkTree(root, "", st, func(path string, e object.TreeEntry) bool {
		if e.Mode == modeSubtree {
			return true // directories carry no content
		}
		data, ok := r.readBlobData(e.Hash)
		if !ok || !diffText(data) || len(data) > maxSearchFileBytes {
			return true // missing, binary, or oversize → skipped, not a match
		}
		if scanned+len(data) > maxSearchScanBytes {
			budgetHit = true
			return false // bytes budget exhausted
		}
		scanned += len(data)
		lines := scanLines(data, m)
		if len(lines) == 0 {
			return true
		}
		if len(res.Matches) == limit {
			capped = true
			return false
		}
		res.Matches = append(res.Matches, ContentMatch{Path: path, Blob: types.BlobID(e.Hash), Lines: lines})
		return true
	})
	res.Truncated = st.truncated || capped || budgetHit
	sort.Slice(res.Matches, func(i, j int) bool { return res.Matches[i].Path < res.Matches[j].Path })
	return res, nil
}

// readBlobData reads a blob's bytes, or ok=false if the id is missing or not a blob.
func (r *Repo) readBlobData(id types.ObjectID) ([]byte, bool) {
	obj, err := r.store.Read(id)
	if err != nil {
		return nil, false
	}
	b, ok := obj.(*object.Blob)
	if !ok {
		return nil, false
	}
	return b.Data, true
}

// scanLines returns the matching lines of data (1-based), up to the per-file cap.
// A blank line never matches a non-empty query, so the trailing-newline phantom
// line is harmless.
func scanLines(data []byte, m matcher) []ContentLine {
	var out []ContentLine
	for i, line := range strings.Split(string(data), "\n") {
		if m.match(line) {
			out = append(out, ContentLine{Line: i + 1, Content: line})
			if len(out) >= maxSearchLines {
				break
			}
		}
	}
	return out
}
