package conflict

import (
	"fmt"
	"strings"
)

// diffContext is the number of unchanged lines shown around each change.
const diffContext = 3

// FormatUnified renders a unified diff (context 3) that turns a into b, reusing
// the package's LCS change blocks. aLabel/bLabel are the file headers (e.g.
// "a/foo", "b/foo"). It returns nil when a and b are identical. Change lines are
// prefixed '-' (removed), '+' (added), or ' ' (context); each emitted line ends
// with a newline even when the source's final line does not.
func FormatUnified(a, b []byte, aLabel, bLabel string) []byte {
	aLines := splitLines(a)
	bLines := splitLines(b)
	blocks := changeBlocks(aLines, bLines)
	if len(blocks) == 0 {
		return nil
	}

	// Group blocks whose gap is small enough to share context into one hunk.
	var hunks [][]lineChange
	var cur []lineChange
	for _, blk := range blocks {
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			if blk.baseStart-prev.baseEnd > 2*diffContext {
				hunks = append(hunks, cur)
				cur = nil
			}
		}
		cur = append(cur, blk)
	}
	if len(cur) > 0 {
		hunks = append(hunks, cur)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", aLabel)
	fmt.Fprintf(&sb, "+++ %s\n", bLabel)

	emitLine := func(prefix, line string) {
		sb.WriteString(prefix)
		sb.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			sb.WriteString("\n")
		}
	}

	for _, hb := range hunks {
		aStart := max(hb[0].baseStart-diffContext, 0)
		aEnd := min(hb[len(hb)-1].baseEnd+diffContext, len(aLines))

		// b-line where the hunk begins = a-line shifted by the net line delta of
		// every block that starts before this hunk's first block.
		bStart := aStart
		for _, blk := range blocks {
			if blk.baseStart < hb[0].baseStart {
				bStart += len(blk.repl) - (blk.baseEnd - blk.baseStart)
			}
		}

		var body []string
		aCount, bCount := 0, 0
		pos := aStart
		hbi := 0
		for pos < aEnd || hbi < len(hb) {
			if hbi < len(hb) && hb[hbi].baseStart == pos {
				blk := hb[hbi]
				for k := blk.baseStart; k < blk.baseEnd; k++ {
					body = append(body, "-"+aLines[k])
					aCount++
				}
				for _, l := range blk.repl {
					body = append(body, "+"+l)
					bCount++
				}
				pos = blk.baseEnd
				hbi++
				continue
			}
			if pos >= aEnd {
				break
			}
			body = append(body, " "+aLines[pos])
			aCount++
			bCount++
			pos++
		}

		// An empty range uses a 0-based start (git convention), otherwise 1-based.
		aHdr := aStart + 1
		if aCount == 0 {
			aHdr = aStart
		}
		bHdr := bStart + 1
		if bCount == 0 {
			bHdr = bStart
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aHdr, aCount, bHdr, bCount)
		for _, l := range body {
			emitLine(l[:1], l[1:])
		}
	}

	return []byte(sb.String())
}
