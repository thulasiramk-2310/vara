package diff

// changeBlock represents a contiguous set of base lines that were replaced by
// a different set of lines in one version (ours or theirs).
type changeBlock struct {
	baseStart int      // First base line index (0-based, inclusive).
	baseEnd   int      // Last base line index (exclusive).
	newLines  []string // Replacement lines (may be empty for a pure deletion).
}

// extractChangeBlocks converts a Myers edit script into a sorted slice of
// non-overlapping change blocks aligned to base-line positions.
func extractChangeBlocks(edits []edit) []changeBlock {
	var blocks []changeBlock
	baseIdx := 0

	i := 0
	for i < len(edits) {
		e := edits[i]
		if e.op == opKeep {
			baseIdx++
			i++
			continue
		}
		// Start of a change region.
		regionStart := baseIdx
		var newLines []string

		for i < len(edits) && edits[i].op != opKeep {
			if edits[i].op == opDelete {
				baseIdx++
			} else { // opInsert
				newLines = append(newLines, edits[i].dst)
			}
			i++
		}

		blocks = append(blocks, changeBlock{
			baseStart: regionStart,
			baseEnd:   baseIdx,
			newLines:  newLines,
		})
	}
	return blocks
}

// mergeChangeBlocks applies the diff3 three-way merge algorithm, producing
// the merged line list and a conflict flag.
func mergeChangeBlocks(
	baseLines []string,
	ourBlocks, theirBlocks []changeBlock,
	ourLabel, theirLabel string,
) ([]string, bool) {
	hasConflict := false
	var result []string
	basePos := 0
	oi, ti := 0, 0

	for {
		// Determine the next change event from either side.
		ourNext := len(baseLines)
		theirNext := len(baseLines)
		if oi < len(ourBlocks) {
			ourNext = ourBlocks[oi].baseStart
		}
		if ti < len(theirBlocks) {
			theirNext = theirBlocks[ti].baseStart
		}

		if oi >= len(ourBlocks) && ti >= len(theirBlocks) {
			// No more changes; emit remaining context.
			result = append(result, baseLines[basePos:]...)
			break
		}

		next := ourNext
		if theirNext < next {
			next = theirNext
		}

		// Emit context lines up to the next change.
		result = append(result, baseLines[basePos:next]...)
		basePos = next

		// Expand the conflict zone to cover all overlapping blocks from both sides.
		regionEnd := basePos
		if oi < len(ourBlocks) && ourBlocks[oi].baseStart <= regionEnd {
			regionEnd = maxInt(regionEnd, ourBlocks[oi].baseEnd)
		}
		if ti < len(theirBlocks) && theirBlocks[ti].baseStart <= regionEnd {
			regionEnd = maxInt(regionEnd, theirBlocks[ti].baseEnd)
		}
		// Keep expanding until no new block falls within the current region.
		for {
			expanded := false
			if oi < len(ourBlocks) && ourBlocks[oi].baseStart < regionEnd &&
				ourBlocks[oi].baseEnd > regionEnd {
				regionEnd = ourBlocks[oi].baseEnd
				expanded = true
			}
			if ti < len(theirBlocks) && theirBlocks[ti].baseStart < regionEnd &&
				theirBlocks[ti].baseEnd > regionEnd {
				regionEnd = theirBlocks[ti].baseEnd
				expanded = true
			}
			if !expanded {
				break
			}
		}

		// Collect our changes for this region.
		ourChanged := false
		ourNew := applyBlocksInRegion(baseLines, ourBlocks, basePos, regionEnd, &oi, &ourChanged)

		// Collect their changes for this region.
		theirChanged := false
		theirNew := applyBlocksInRegion(baseLines, theirBlocks, basePos, regionEnd, &ti, &theirChanged)

		// Resolve the conflict zone.
		switch {
		case !ourChanged && !theirChanged:
			// Both sides untouched — emit base (already emitted as context).
		case equalLines(ourNew, theirNew):
			// Both sides made the same change.
			result = append(result, ourNew...)
		case !ourChanged:
			// Only theirs changed.
			result = append(result, theirNew...)
		case !theirChanged:
			// Only ours changed.
			result = append(result, ourNew...)
		default:
			// True conflict.
			hasConflict = true
			result = append(result, "<<<<<<< "+ourLabel+"\n")
			result = append(result, ourNew...)
			result = append(result, "=======\n")
			result = append(result, theirNew...)
			result = append(result, ">>>>>>> "+theirLabel+"\n")
		}

		basePos = regionEnd
	}

	return result, hasConflict
}

// applyBlocksInRegion collects all change blocks from blocks that fall within
// [regionStart, regionEnd), builds the resulting line sequence (interleaving
// unchanged base lines between blocks), and advances the index pointer.
// Sets *changed to true if any block was consumed.
func applyBlocksInRegion(
	baseLines []string,
	blocks []changeBlock,
	regionStart, regionEnd int,
	idx *int,
	changed *bool,
) []string {
	var result []string
	pos := regionStart

	for *idx < len(blocks) && blocks[*idx].baseStart < regionEnd {
		b := blocks[*idx]
		// Context within this side's view before the block.
		result = append(result, baseLines[pos:b.baseStart]...)
		result = append(result, b.newLines...)
		pos = b.baseEnd
		*changed = true
		(*idx)++
	}

	// Remaining base lines in the region (after the last consumed block).
	if !*changed {
		// No blocks consumed — return the base lines for this region so the
		// caller can use them as the "unchanged" side.
		result = append(result, baseLines[regionStart:regionEnd]...)
	} else {
		result = append(result, baseLines[pos:regionEnd]...)
	}

	return result
}

// equalLines returns true if a and b contain the same strings in the same order.
func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
