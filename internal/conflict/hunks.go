package conflict

import (
	"fmt"
	"strings"
)

// Hunk is one conflict region of a file: the two competing versions, and the
// common ancestor when the markers carry one (the diff3 and zdiff3 styles).
type Hunk struct {
	OurLabel   string
	TheirLabel string
	Ours       []string
	Base       []string
	Theirs     []string
	HasBase    bool
	Raw        []string // the hunk exactly as it appeared, markers included
}

// Segment is either plain text or one conflict hunk. A file is a run of segments.
type Segment struct {
	Text []string // set for a plain-text segment
	Hunk *Hunk    // set for a conflict hunk
}

// Pick says how one hunk is to be settled.
type Pick int

const (
	PickSkip            Pick = iota // leave the hunk as it is, markers and all
	PickOurs                        // keep our side
	PickTheirs                      // keep their side
	PickBoth                        // keep both, ours first
	PickBothTheirsFirst             // keep both, theirs first
	PickCustom                      // use Decision.Custom
)

// Decision is the choice made for one hunk.
type Decision struct {
	Pick   Pick
	Custom []string // lines to use when Pick is PickCustom
}

const markerBase = "|||||||"

func isBaseMarker(line string) bool { return strings.HasPrefix(trimEnd(line), markerBase) }

// label returns what follows a marker such as "<<<<<<< main".
func label(line, marker string) string {
	return strings.TrimSpace(strings.TrimPrefix(trimEnd(line), marker))
}

// Parse splits data into plain text and conflict hunks. Unlike Resolve it
// understands the optional "|||||||" base section, so it reads the markers a merge
// writes in any style. It errors on malformed markers instead of guessing.
func Parse(data []byte) ([]Segment, error) {
	lines := splitLines(data)
	var segs []Segment
	var text []string
	flush := func() {
		if len(text) > 0 {
			segs = append(segs, Segment{Text: text})
			text = nil
		}
	}

	for i := 0; i < len(lines); {
		if !isBegin(lines[i]) {
			text = append(text, lines[i])
			i++
			continue
		}
		flush()
		h := &Hunk{OurLabel: label(lines[i], markerBegin)}
		start := i
		j := i + 1
		section := &h.Ours
		for ; j < len(lines); j++ {
			l := lines[j]
			switch {
			case isBegin(l):
				return nil, fmt.Errorf("malformed conflict markers near line %d", j+1)
			case isBaseMarker(l) && !h.HasBase && section == &h.Ours:
				h.HasBase = true
				section = &h.Base
			case isBaseMarker(l) && h.HasBase && section == &h.Base:
				return nil, fmt.Errorf("malformed conflict markers near line %d: a second base section", j+1)
			case isSep(l) && section != &h.Theirs:
				section = &h.Theirs
			case isEnd(l) && section == &h.Theirs:
				h.TheirLabel = label(l, markerEnd)
				goto done
			case isEnd(l):
				return nil, fmt.Errorf("malformed conflict markers near line %d: end marker before the separator", j+1)
			default:
				*section = append(*section, l)
			}
		}
		return nil, fmt.Errorf("unterminated conflict starting at line %d", start+1)
	done:
		h.Raw = append([]string(nil), lines[start:j+1]...)
		segs = append(segs, Segment{Hunk: h})
		i = j + 1
	}
	flush()
	return segs, nil
}

// HunkCount returns how many conflict hunks the segments hold.
func HunkCount(segs []Segment) int {
	n := 0
	for _, s := range segs {
		if s.Hunk != nil {
			n++
		}
	}
	return n
}

// Assemble puts the file back together. decisions[k] settles the k-th hunk; a
// missing or skipped decision leaves that hunk exactly as it was, so a file with
// nothing decided comes back byte for byte. The second result counts the hunks
// still unresolved.
func Assemble(segs []Segment, decisions []Decision) ([]byte, int) {
	var out []string
	remaining, k := 0, 0
	for _, s := range segs {
		if s.Hunk == nil {
			out = append(out, s.Text...)
			continue
		}
		var d Decision
		if k < len(decisions) {
			d = decisions[k]
		}
		k++
		switch d.Pick {
		case PickOurs:
			out = append(out, s.Hunk.Ours...)
		case PickTheirs:
			out = append(out, s.Hunk.Theirs...)
		case PickBoth:
			out = append(out, joinLines(s.Hunk.Ours, s.Hunk.Theirs)...)
		case PickBothTheirsFirst:
			out = append(out, joinLines(s.Hunk.Theirs, s.Hunk.Ours)...)
		case PickCustom:
			out = append(out, d.Custom...)
		default:
			out = append(out, s.Hunk.Raw...)
			remaining++
		}
	}
	return []byte(strings.Join(out, "")), remaining
}

// joinLines concatenates two runs of lines. If the first run ends without a
// newline (the last line of a file), one is added so the runs do not fuse.
func joinLines(a, b []string) []string {
	out := append([]string(nil), a...)
	if n := len(out); n > 0 && len(b) > 0 && !strings.HasSuffix(out[n-1], "\n") {
		out[n-1] += "\n"
	}
	return append(out, b...)
}
