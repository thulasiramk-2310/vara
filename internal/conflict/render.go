package conflict

// Styled base-aware three-way conflict rendering.
//
// Render is the single conflict-rendering engine used at both merge time and
// resolve time. It is the base-aware, order-independent three-way merge (same
// region walk as the auto-merge in threeway.go), but for each genuine conflict it
// emits markers in a chosen style:
//
//   - StyleMerge  — classic 2-way (<<<<<<< / ======= / >>>>>>>).
//   - StyleDiff3  — adds the |||||||  base section (the common ancestor lines).
//   - StyleZdiff3 — diff3 with lines common to both sides at the top/bottom of a
//     hunk hoisted out as context, so the markers wrap only the truly divergent
//     middle (git's zdiff3).
//
// Because the decision comes from this package's own LCS (changeBlocks), not the
// engine's Myers alignment, the result is independent of which side is "ours" —
// fixing the engine's order-dependence on adjacent edits.

// MarkerStyle selects how conflict regions are rendered.
type MarkerStyle int

const (
	StyleMerge  MarkerStyle = iota // 2-way markers
	StyleDiff3                     // adds the ||||||| base section
	StyleZdiff3                    // diff3 with shared prefix/suffix hoisted out
)

// ParseMarkerStyle converts a config/flag value to a MarkerStyle.
func ParseMarkerStyle(s string) (MarkerStyle, bool) {
	switch s {
	case "merge", "":
		return StyleMerge, true
	case "diff3":
		return StyleDiff3, true
	case "zdiff3":
		return StyleZdiff3, true
	default:
		return StyleMerge, false
	}
}

// Render performs the base-aware three-way merge and renders remaining conflicts
// in the given style. It settles every region only one side changed (including
// disjoint edits that abut, which the engine reports as one conflict), settles
// identical changes, and leaves genuine same-region disagreements — plus
// hunk-level modify/delete — as markers. Stats.Remaining == 0 ⟺ marker-free.
func Render(base, ours, theirs []byte, ourLabel, theirLabel string, style MarkerStyle) ([]byte, Stats) {
	baseLines := splitLines(base)
	ourLines := splitLines(ours)
	theirLines := splitLines(theirs)

	if len(baseLines) == 0 {
		if equalLines(ourLines, theirLines) {
			return ours, Stats{}
		}
		var out []byte
		emitConflict(&out, ourLines, nil, theirLines, ourLabel, theirLabel, style)
		return out, Stats{Total: 1, Remaining: 1}
	}

	ourBlocks := changeBlocks(baseLines, ourLines)
	theirBlocks := changeBlocks(baseLines, theirLines)

	var out []byte
	var stats Stats
	emit := func(lines []string) {
		for _, l := range lines {
			out = append(out, l...)
		}
	}

	basePos := 0
	oi, ti := 0, 0
	for {
		ourStart := len(baseLines)
		theirStart := len(baseLines)
		if oi < len(ourBlocks) {
			ourStart = ourBlocks[oi].baseStart
		}
		if ti < len(theirBlocks) {
			theirStart = theirBlocks[ti].baseStart
		}
		if oi >= len(ourBlocks) && ti >= len(theirBlocks) {
			emit(baseLines[basePos:])
			break
		}

		next := ourStart
		if theirStart < next {
			next = theirStart
		}
		emit(baseLines[basePos:next])
		basePos = next

		rs, re := next, next
		var ourRegion, theirRegion []lineChange
		for {
			grew := false
			for oi < len(ourBlocks) && (ourBlocks[oi].baseStart < re || ourBlocks[oi].baseStart == rs) {
				if ourBlocks[oi].baseEnd > re {
					re = ourBlocks[oi].baseEnd
				}
				ourRegion = append(ourRegion, ourBlocks[oi])
				oi++
				grew = true
			}
			for ti < len(theirBlocks) && (theirBlocks[ti].baseStart < re || theirBlocks[ti].baseStart == rs) {
				if theirBlocks[ti].baseEnd > re {
					re = theirBlocks[ti].baseEnd
				}
				theirRegion = append(theirRegion, theirBlocks[ti])
				ti++
				grew = true
			}
			if !grew {
				break
			}
		}

		ourOut := flattenRegion(baseLines, ourRegion, rs, re)
		theirOut := flattenRegion(baseLines, theirRegion, rs, re)
		ourChanged := len(ourRegion) > 0
		theirChanged := len(theirRegion) > 0

		switch {
		case ourChanged && !theirChanged:
			emit(ourOut)
		case theirChanged && !ourChanged:
			emit(theirOut)
		case equalLines(ourOut, theirOut):
			stats.Total++
			stats.Resolved++
			emit(ourOut)
		default:
			stats.Total++
			stats.Remaining++
			emitConflict(&out, ourOut, baseLines[rs:re], theirOut, ourLabel, theirLabel, style)
		}
		basePos = re
	}

	return out, stats
}

// emitConflict appends one conflict region's markers to out in the given style.
// baseRegion is the common-ancestor lines for the region (nil when there is no
// base); it is used only by the diff3/zdiff3 styles.
func emitConflict(out *[]byte, ourMid, baseRegion, theirMid []string, ourLabel, theirLabel string, style MarkerStyle) {
	appendLines := func(ls []string) {
		for _, l := range ls {
			*out = append(*out, l...)
		}
	}
	ensureNL := func() {
		if n := len(*out); n > 0 && (*out)[n-1] != '\n' {
			*out = append(*out, '\n')
		}
	}

	if style == StyleZdiff3 {
		// Hoist lines common to both sides at the top/bottom of the hunk out as
		// context, so the markers wrap only the divergent middle.
		pre := commonPrefixLen(ourMid, theirMid)
		suf := commonSuffixLen(ourMid[pre:], theirMid[pre:])
		hoistPre := ourMid[:pre]
		hoistSuf := ourMid[len(ourMid)-suf:]
		oMid := ourMid[pre : len(ourMid)-suf]
		tMid := theirMid[pre : len(theirMid)-suf]
		bMid := trimBaseByHoist(baseRegion, hoistPre, hoistSuf)

		appendLines(hoistPre)
		ensureNL()
		*out = append(*out, ("<<<<<<< " + ourLabel + "\n")...)
		appendLines(oMid)
		ensureNL()
		*out = append(*out, "||||||| base\n"...)
		appendLines(bMid)
		ensureNL()
		*out = append(*out, "=======\n"...)
		appendLines(tMid)
		ensureNL()
		*out = append(*out, (">>>>>>> " + theirLabel + "\n")...)
		appendLines(hoistSuf)
		ensureNL()
		return
	}

	*out = append(*out, ("<<<<<<< " + ourLabel + "\n")...)
	appendLines(ourMid)
	ensureNL()
	if style == StyleDiff3 {
		*out = append(*out, "||||||| base\n"...)
		appendLines(baseRegion)
		ensureNL()
	}
	*out = append(*out, "=======\n"...)
	appendLines(theirMid)
	ensureNL()
	*out = append(*out, (">>>>>>> " + theirLabel + "\n")...)
}

// trimBaseByHoist strips from base the leading lines equal to the hoisted prefix
// and the trailing lines equal to the hoisted suffix, so the diff3 base section
// shows only the ancestor of the divergent middle.
func trimBaseByHoist(base, pre, suf []string) []string {
	i := 0
	for i < len(base) && i < len(pre) && base[i] == pre[i] {
		i++
	}
	j := len(base)
	k := len(suf)
	for j > i && k > 0 && base[j-1] == suf[k-1] {
		j--
		k--
	}
	return base[i:j]
}

func commonPrefixLen(a, b []string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func commonSuffixLen(a, b []string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}
