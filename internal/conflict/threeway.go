package conflict

// Base-aware three-way line merge (the engine of `vara resolve --auto`).
//
// The frozen merge engine (pkg/diff) already performs a coarse three-way merge:
// a region only one side changed is taken from that side, an identical change on
// both is taken once, and anything else is emitted as conflict markers. But it
// forms a conflict region by expanding across *abutting* change blocks, so two
// edits that touch adjacent-but-disjoint base lines (ours edits line 2, theirs
// edits line 3) collapse into one conflict even though they combine cleanly.
//
// MergeThreeWay re-does the merge above the engine with a finer region rule: a
// region is grown only across change blocks whose base ranges actually *overlap*
// (or are co-located insertions), never across ones that merely abut. Disjoint
// edits therefore settle on their own sides. Regions both sides changed
// identically are settled; regions changed differently — including a hunk-level
// modify/delete, where one side empties a region the other edits — are left as
// markers rather than guessed, matching `--auto`'s refusal to pick silently.
//
// It reuses this package's splitLines/equalLines and imports nothing from the
// engine; it operates purely on the base/ours/theirs bytes the caller recovered
// from the conflict sidecar's stored blobs.

// lineChange is a contiguous edit turning a base range into replacement lines:
// base[baseStart:baseEnd) becomes repl. A pure deletion has empty repl; a pure
// insertion has baseStart == baseEnd (a zero-width point between base lines).
type lineChange struct {
	baseStart int
	baseEnd   int
	repl      []string
}

// MergeThreeWay merges base/ours/theirs by line and returns the merged bytes plus
// Stats, rendering remaining conflicts as classic 2-way markers. Total counts the
// regions both sides changed, Resolved those it settled, and Remaining those left
// as conflict markers — so Remaining == 0 exactly when the result is marker-free.
// It never returns an error (the signature mirrors Resolve so the caller can treat
// both settlement paths uniformly). It is a thin wrapper over Render(StyleMerge);
// callers that want the base section use Render(..., StyleDiff3|StyleZdiff3).
func MergeThreeWay(base, ours, theirs []byte, ourLabel, theirLabel string) ([]byte, Stats, error) {
	out, stats := Render(base, ours, theirs, ourLabel, theirLabel, StyleMerge)
	return out, stats, nil
}

// flattenRegion renders one side's view of [rs, re): its change blocks'
// replacements interleaved with the base lines they left untouched.
func flattenRegion(base []string, region []lineChange, rs, re int) []string {
	pos := rs
	var out []string
	for _, b := range region {
		if b.baseStart > pos {
			out = append(out, base[pos:b.baseStart]...)
		}
		out = append(out, b.repl...)
		pos = b.baseEnd
	}
	if re > pos {
		out = append(out, base[pos:re]...)
	}
	return out
}

// changeBlocks computes the edits turning base into other as base-aligned,
// non-overlapping change blocks, via a longest-common-subsequence backtrack.
// The LCS table is O(n*m); conflicted files are modest, so this is kept simple
// and obviously correct rather than switching to Myers.
func changeBlocks(base, other []string) []lineChange {
	n, m := len(base), len(other)
	if n == 0 {
		if m == 0 {
			return nil
		}
		return []lineChange{{baseStart: 0, baseEnd: 0, repl: append([]string(nil), other...)}}
	}

	// lcs[i][j] = length of the LCS of base[i:] and other[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if base[i] == other[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var blocks []lineChange
	i, j := 0, 0
	for i < n && j < m {
		if base[i] == other[j] {
			i++
			j++
			continue
		}
		start := i
		var repl []string
		for i < n && j < m && base[i] != other[j] {
			if lcs[i+1][j] >= lcs[i][j+1] {
				i++ // delete base[i]
			} else {
				repl = append(repl, other[j]) // insert other[j]
				j++
			}
		}
		blocks = append(blocks, lineChange{baseStart: start, baseEnd: i, repl: repl})
	}
	// Trailing edit: base lines left to delete and/or other lines left to insert.
	if i < n || j < m {
		blocks = append(blocks, lineChange{baseStart: i, baseEnd: n, repl: append([]string(nil), other[j:]...)})
	}
	return blocks
}
