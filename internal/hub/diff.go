package hub

// This file is the RFC-0023 diff projection. The FILE level is a pure projection
// of the engine's diff.DiffTrees (which files changed between two trees). The LINE
// level is a PRESENTATIONAL unified diff computed here, deliberately separate from
// the engine's merge-oriented Myers (RFC-0023 D2): a viewer diff optimizes for
// readable, stable output, a merge diff for correct three-way reconstruction, and
// the two must never be conflated. Everything is bounded (D5) and inert (D4).

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/thulasiramk-2310/vara/pkg/diff"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Diff bounds (RFC-0023 §7). Package vars, not consts, only so tests can shrink
// them without large fixtures.
var (
	maxDiffFiles     = 1000      // summary file-count cap → truncated
	maxDiffInputHard = 5 << 20   // 5 MiB — hard per-side ceiling → ErrTooLarge (413)
	maxDiffInputSoft = 512 << 10 // 512 KiB — soft per-side cap → truncated
	maxDiffEdits     = 10000     // Myers max edit distance → too different → truncated
	maxHunkLines     = 20000     // total emitted diff lines → truncated
)

const diffContext = 3 // lines of context around each changed region

// SetMaxDiffInputHard / SetMaxDiffInputSoft are test hooks that bound the diff
// input sizes (RFC-0023 §7) without multi-MiB fixtures; each returns the previous
// value so a test can restore it.
func SetMaxDiffInputHard(n int) int { old := maxDiffInputHard; maxDiffInputHard = n; return old }
func SetMaxDiffInputSoft(n int) int { old := maxDiffInputSoft; maxDiffInputSoft = n; return old }

// DiffFile is one changed file in a summary (RFC-0023 §5.1): path, status, and —
// reserved for future rename detection — an old path.
type DiffFile struct {
	Path    string
	OldPath string
	Status  string // added / deleted / modified
}

// DiffSummary is the projected changed-file set between two resolved commits.
type DiffSummary struct {
	BaseCommit types.CommitID
	HeadCommit types.CommitID
	Files      []DiffFile
	Truncated  bool
}

// DiffLine is one line of a hunk: type context/add/del, content inert.
type DiffLine struct {
	Type    string
	Content string
}

// Hunk is a contiguous diff region with unified @@ coordinates.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Header             string
	Lines              []DiffLine
}

// FileDiff is the projected line-level diff of one changed file.
type FileDiff struct {
	BaseCommit types.CommitID
	HeadCommit types.CommitID
	Status     string
	Binary     bool
	Additions  int
	Deletions  int
	Hunks      []Hunk
	Truncated  bool
}

func (r *Repo) readCommit(id types.CommitID) (*object.Commit, bool) {
	obj, err := r.store.Read(types.ObjectID(id))
	if err != nil {
		return nil, false
	}
	c, ok := obj.(*object.Commit)
	return c, ok
}

// resolveDiffEnds resolves base/head to their commits and trees. head is required;
// an empty base defaults to head's first parent (the zero tree — an empty tree —
// when head is a root commit), so `?head=X` shows what X did (RFC-0023 §5).
func (r *Repo) resolveDiffEnds(base, head string) (baseC, headC types.CommitID, baseT, headT types.TreeID, err error) {
	hc, e := r.resolveStart(head)
	if e != nil {
		return baseC, headC, baseT, headT, ErrNotFound
	}
	hcommit, ok := r.readCommit(hc)
	if !ok {
		return baseC, headC, baseT, headT, ErrNotFound
	}
	headC, headT = hc, hcommit.TreeHash

	if base == "" {
		if len(hcommit.Parents) > 0 {
			baseC = hcommit.Parents[0]
			pt, ok := r.commitRoot(baseC)
			if !ok {
				return baseC, headC, baseT, headT, ErrNotFound
			}
			baseT = pt
		}
		// else: root commit → baseC/baseT stay zero (the empty tree).
		return baseC, headC, baseT, headT, nil
	}
	bc, e := r.resolveStart(base)
	if e != nil {
		return baseC, headC, baseT, headT, ErrNotFound
	}
	pt, ok := r.commitRoot(bc)
	if !ok {
		return baseC, headC, baseT, headT, ErrNotFound
	}
	baseC, baseT = bc, pt
	return baseC, headC, baseT, headT, nil
}

func diffStatus(t diff.ChangeType) string {
	switch t {
	case diff.Added:
		return "added"
	case diff.Deleted:
		return "deleted"
	default:
		return "modified"
	}
}

// DiffSummary lists the files changed between base and head — a pure projection of
// diff.DiffTrees, no blob reads (RFC-0023 §5.1). Over the file-count cap it sets
// Truncated and caps the list.
func (r *Repo) DiffSummary(base, head string) (*DiffSummary, error) {
	baseC, headC, baseT, headT, err := r.resolveDiffEnds(base, head)
	if err != nil {
		return nil, err
	}
	fds, err := diff.DiffTrees(r.store, types.ObjectID(baseT), types.ObjectID(headT))
	if err != nil {
		return nil, err
	}
	out := &DiffSummary{BaseCommit: baseC, HeadCommit: headC}
	for _, fd := range fds {
		if len(out.Files) >= maxDiffFiles {
			out.Truncated = true
			break
		}
		out.Files = append(out.Files, DiffFile{Path: fd.Path(), Status: diffStatus(fd.Type)})
	}
	return out, nil
}

// blobBytes reads a blob's bytes, or nil for the zero id (an absent side).
func (r *Repo) blobBytes(id types.ObjectID) ([]byte, error) {
	if id == (types.ObjectID{}) {
		return nil, nil
	}
	obj, err := r.store.Read(id)
	if err != nil {
		return nil, ErrNotFound
	}
	b, ok := obj.(*object.Blob)
	if !ok {
		return nil, ErrNotFound
	}
	return b.Data, nil
}

// diffText reports whether bytes may be line-diffed inline: NUL-free and valid
// UTF-8 (RFC-0023 §7, same rule as RFC-0022).
func diffText(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	return utf8.Valid(b)
}

// FileDiff computes the line-level diff of one changed file (RFC-0023 §5.2). A path
// not in the base..head change set is ErrNotFound; a side over the hard ceiling is
// ErrTooLarge; a binary side or an over-soft-cap/too-different diff yields a
// Binary/Truncated result with no hunks.
func (r *Repo) FileDiff(base, head, path string) (*FileDiff, error) {
	baseC, headC, baseT, headT, err := r.resolveDiffEnds(base, head)
	if err != nil {
		return nil, err
	}
	fds, err := diff.DiffTrees(r.store, types.ObjectID(baseT), types.ObjectID(headT))
	if err != nil {
		return nil, err
	}
	var match *diff.FileDiff
	for i := range fds {
		if fds[i].Path() == path {
			match = &fds[i]
			break
		}
	}
	if match == nil {
		return nil, ErrNotFound // not part of the change set (unchanged or absent)
	}
	out := &FileDiff{BaseCommit: baseC, HeadCommit: headC, Status: diffStatus(match.Type)}

	oldB, err := r.blobBytes(match.OldBlob)
	if err != nil {
		return nil, err
	}
	newB, err := r.blobBytes(match.NewBlob)
	if err != nil {
		return nil, err
	}
	if len(oldB) > maxDiffInputHard || len(newB) > maxDiffInputHard {
		return nil, ErrTooLarge
	}
	if !diffText(oldB) || !diffText(newB) {
		out.Binary = true
		return out, nil
	}
	if len(oldB) > maxDiffInputSoft || len(newB) > maxDiffInputSoft {
		out.Truncated = true
		return out, nil
	}
	hunks, adds, dels, ok := computeHunks(oldB, newB)
	if !ok {
		out.Truncated = true // too different, or over the hunk cap
		return out, nil
	}
	out.Hunks, out.Additions, out.Deletions = hunks, adds, dels
	return out, nil
}

// splitDiffLines splits content into lines for diffing, treating a trailing
// newline as a terminator (not a phantom empty line).
func splitDiffLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// computeHunks builds unified-diff hunks from two byte slices, returning the
// additions/deletions counts and ok=false when the diff is too different (over the
// Myers edit cap) or exceeds the hunk-line cap (§7).
func computeHunks(oldB, newB []byte) ([]Hunk, int, int, bool) {
	a := splitDiffLines(oldB)
	b := splitDiffLines(newB)
	ops, ok := myersLines(a, b, maxDiffEdits)
	if !ok {
		return nil, 0, 0, false
	}

	// Flatten ops into positioned entries.
	type ent struct {
		t            byte // '=', '+', '-'
		text         string
		oldNo, newNo int
	}
	var ents []ent
	oldNo, newNo := 1, 1
	for _, op := range ops {
		switch op.kind {
		case '=':
			ents = append(ents, ent{'=', op.text, oldNo, newNo})
			oldNo++
			newNo++
		case '-':
			ents = append(ents, ent{'-', op.text, oldNo, 0})
			oldNo++
		case '+':
			ents = append(ents, ent{'+', op.text, 0, newNo})
			newNo++
		}
	}

	// Segment into hunks: each changed entry pulls in diffContext lines of context
	// on both sides; changes within 2*context of each other merge into one hunk.
	n := len(ents)
	changed := func(i int) bool { return ents[i].t != '=' }
	var hunks []Hunk
	adds, dels, emitted := 0, 0, 0
	i := 0
	for i < n {
		if !changed(i) {
			i++
			continue
		}
		lo := i - diffContext
		if lo < 0 {
			lo = 0
		}
		last := i // index of the most recent change in this hunk
		j := i + 1
		for j < n {
			if changed(j) {
				last = j
				j++
			} else if j-last <= diffContext {
				j++ // still within the trailing-context gap; keep scanning
			} else {
				break
			}
		}
		hi := last + 1 + diffContext // trailing context after the last change
		if hi > n {
			hi = n
		}

		var lines []DiffLine
		oldStart, newStart, oldLines, newLines := 0, 0, 0, 0
		for k := lo; k < hi; k++ {
			e := ents[k]
			switch e.t {
			case '=':
				lines = append(lines, DiffLine{Type: "context", Content: e.text})
				if oldStart == 0 {
					oldStart = e.oldNo
				}
				if newStart == 0 {
					newStart = e.newNo
				}
				oldLines++
				newLines++
			case '-':
				lines = append(lines, DiffLine{Type: "del", Content: e.text})
				if oldStart == 0 {
					oldStart = e.oldNo
				}
				oldLines++
				dels++
			case '+':
				lines = append(lines, DiffLine{Type: "add", Content: e.text})
				if newStart == 0 {
					newStart = e.newNo
				}
				newLines++
				adds++
			}
		}
		emitted += len(lines)
		if emitted > maxHunkLines {
			return nil, 0, 0, false
		}
		hunks = append(hunks, Hunk{
			OldStart: oldStart, OldLines: oldLines,
			NewStart: newStart, NewLines: newLines,
			Header: fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldLines, newStart, newLines),
			Lines:  lines,
		})
		i = hi
	}
	return hunks, adds, dels, true
}

// --- presentational Myers line diff (D2) ---------------------------------------

type lineOp struct {
	kind byte // '=', '+', '-'
	text string
}

// myersLines computes a line-level edit script transforming src into dst with the
// Myers O(ND) algorithm, bounded by maxD edits. It returns ok=false when the edit
// distance exceeds maxD (too different to render inline). This is the viewer diff
// (D2), distinct from the engine's merge Myers.
func myersLines(src, dst []string, maxD int) ([]lineOp, bool) {
	n, m := len(src), len(dst)
	if n == 0 && m == 0 {
		return nil, true
	}
	if n == 0 {
		out := make([]lineOp, m)
		for i, l := range dst {
			out[i] = lineOp{'+', l}
		}
		return out, true
	}
	if m == 0 {
		out := make([]lineOp, n)
		for i, l := range src {
			out[i] = lineOp{'-', l}
		}
		return out, true
	}
	offset := n + m
	v := make([]int, 2*(n+m)+1)
	traces := make([][]int, 0, maxD+1)
	for d := 0; d <= n+m; d++ {
		if maxD > 0 && d > maxD {
			return nil, false
		}
		snap := make([]int, len(v))
		copy(snap, v)
		traces = append(traces, snap)
		for k := -d; k <= d; k += 2 {
			down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])
			var x int
			if down {
				x = v[k+1+offset]
			} else {
				x = v[k-1+offset] + 1
			}
			y := x - k
			for x < n && y < m && src[x] == dst[y] {
				x++
				y++
			}
			v[k+offset] = x
			if x >= n && y >= m {
				return buildLineOps(traces, src, dst, d, offset), true
			}
		}
	}
	return nil, false
}

// buildLineOps traces backward through the Myers D-paths to produce the edit
// script in forward order (mirrors pkg/diff buildEdits, viewer-side).
func buildLineOps(traces [][]int, src, dst []string, d, offset int) []lineOp {
	x, y := len(src), len(dst)
	ops := make([]lineOp, 0, x+y)
	for d > 0 {
		v := traces[d]
		k := x - y
		down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])
		var prevK int
		if down {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[prevK+offset]
		prevY := prevX - prevK
		if down {
			for x > prevX && y > prevY+1 {
				x--
				y--
				ops = append(ops, lineOp{'=', src[x]})
			}
			ops = append(ops, lineOp{'+', dst[prevY]})
		} else {
			for x > prevX+1 && y > prevY {
				x--
				y--
				ops = append(ops, lineOp{'=', src[x]})
			}
			ops = append(ops, lineOp{'-', src[prevX]})
		}
		x, y = prevX, prevY
		d--
	}
	for x > 0 && y > 0 {
		x--
		y--
		ops = append(ops, lineOp{'=', src[x]})
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}
