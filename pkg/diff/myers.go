package diff

// editOp classifies a single line-level operation in an edit script.
type editOp int

const (
	opKeep   editOp = iota // Line present in both src and dst (unchanged).
	opInsert               // Line present only in dst (inserted).
	opDelete               // Line present only in src (deleted).
)

// edit is one element of a Myers edit script.
type edit struct {
	op  editOp
	src string // Populated for opKeep and opDelete.
	dst string // Populated for opKeep and opInsert.
}

// myersDiff computes the shortest edit script transforming src into dst using
// the Myers O(ND) algorithm (Eugene W. Myers, 1986).
func myersDiff(src, dst []string) []edit {
	n, m := len(src), len(dst)

	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 {
		out := make([]edit, m)
		for i, l := range dst {
			out[i] = edit{op: opInsert, dst: l}
		}
		return out
	}
	if m == 0 {
		out := make([]edit, n)
		for i, l := range src {
			out[i] = edit{op: opDelete, src: l}
		}
		return out
	}

	max := n + m
	offset := max
	v := make([]int, 2*max+1)

	// traces[d] holds a snapshot of v taken before the d-th round.
	traces := make([][]int, 0, max+1)

	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		traces = append(traces, snap)

		for k := -d; k <= d; k += 2 {
			// Choose: move down (insert) or right (delete)?
			down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])

			var x int
			if down {
				x = v[k+1+offset] // Take the x from diagonal k+1.
			} else {
				x = v[k-1+offset] + 1 // Move right from diagonal k-1.
			}
			y := x - k

			// Extend along the diagonal (equal lines = free moves).
			for x < n && y < m && src[x] == dst[y] {
				x++
				y++
			}
			v[k+offset] = x

			if x >= n && y >= m {
				return buildEdits(traces, src, dst, d, offset)
			}
		}
	}

	// Fallback (unreachable for well-formed inputs).
	out := make([]edit, 0, n+m)
	for _, l := range src {
		out = append(out, edit{op: opDelete, src: l})
	}
	for _, l := range dst {
		out = append(out, edit{op: opInsert, dst: l})
	}
	return out
}

// buildEdits traces backward through the Myers D-paths to produce the edit
// script in forward order.
func buildEdits(traces [][]int, src, dst []string, d, offset int) []edit {
	x, y := len(src), len(dst)
	edits := make([]edit, 0, x+y)

	for d > 0 {
		v := traces[d]
		k := x - y

		// Determine whether we went down (insert) or right (delete) to reach k.
		down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])

		var prevK int
		if down {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[prevK+offset]
		prevY := prevX - prevK

		// Diagonal snake: Keep lines from (prevX+??, prevY+??) to (x, y).
		// After a down step: x does not change, y increases by 1 from prevY.
		//   Snake starts at (prevX, prevY+1).
		// After a right step: x increases by 1 from prevX, y does not change.
		//   Snake starts at (prevX+1, prevY).
		if down {
			// Emit keeps for the snake portion (from snake start to (x,y)).
			for x > prevX && y > prevY+1 {
				x--
				y--
				edits = append(edits, edit{op: opKeep, src: src[x], dst: dst[y]})
			}
			// The insert that preceded the snake.
			edits = append(edits, edit{op: opInsert, dst: dst[prevY]})
		} else {
			// Emit keeps for the snake portion.
			for x > prevX+1 && y > prevY {
				x--
				y--
				edits = append(edits, edit{op: opKeep, src: src[x], dst: dst[y]})
			}
			// The delete that preceded the snake.
			edits = append(edits, edit{op: opDelete, src: src[prevX]})
		}

		x = prevX
		y = prevY
		d--
	}

	// Remaining diagonal from the d=0 path (all keeps before any edit).
	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, edit{op: opKeep, src: src[x], dst: dst[y]})
	}

	// Reverse: we built the list from the end backward.
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
	return edits
}
