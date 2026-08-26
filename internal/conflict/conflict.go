// Package conflict parses and rewrites the diff3-style conflict markers VARA's
// merge engine writes into a working-tree file, so an automatic resolver can
// settle conflicts by a chosen strategy without re-running the merge.
//
// RFC:
//
//	VARA-RFC-0008 Merge Algorithm (§5 conflict markers)
//
// The markers produced by the engine (pkg/diff) are the classic two-way form:
//
//	<<<<<<< < our-label>
//	<our lines>
//	=======
//	<their lines>
//	>>>>>>> <their-label>
//
// This package operates purely on bytes: it never touches the object store, the
// index, or the filesystem — the command layer (internal/commands) owns that.
// It sits above the frozen engine and imports nothing from it.
package conflict

import (
	"fmt"
	"slices"
	"strings"
)

// Marker prefixes. The separator is matched exactly; the begin/end markers are
// matched by prefix because they carry a trailing label.
const (
	markerBegin = "<<<<<<<"
	markerSep   = "======="
	markerEnd   = ">>>>>>>"
)

// Strategy selects how each conflict hunk is resolved.
type Strategy string

const (
	// Ours keeps only our side of every conflict.
	Ours Strategy = "ours"
	// Theirs keeps only their side of every conflict.
	Theirs Strategy = "theirs"
	// Union keeps both sides, ours first — good for append-only files.
	Union Strategy = "union"
	// Auto resolves only the hunks it can settle without guessing (identical
	// sides, or one side empty) and leaves genuinely ambiguous hunks in place.
	Auto Strategy = "auto"
)

// ParseStrategy converts a flag value to a Strategy.
func ParseStrategy(s string) (Strategy, error) {
	switch Strategy(s) {
	case Ours, Theirs, Union, Auto:
		return Strategy(s), nil
	default:
		return "", fmt.Errorf("unknown conflict strategy %q (want ours|theirs|union|auto)", s)
	}
}

// Stats reports what a resolution did.
type Stats struct {
	Total     int // conflict hunks found
	Resolved  int // hunks rewritten to a single resolved form
	Remaining int // hunks left unresolved (only possible under Auto)
}

// HasMarkers reports whether data contains at least one conflict begin marker.
func HasMarkers(data []byte) bool {
	return slices.ContainsFunc(splitLines(data), isBegin)
}

// binarySniffLen bounds how much of a blob IsBinary inspects, matching Git's
// convention of sniffing the first ~8000 bytes for a NUL.
const binarySniffLen = 8000

// IsBinary reports whether data looks binary, using Git's heuristic: a NUL byte
// within the first binarySniffLen bytes. Line-merging binary content produces
// corruption, so callers must fall back to a whole-side choice for such blobs.
func IsBinary(data []byte) bool {
	n := min(len(data), binarySniffLen)
	for i := range n {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// Count returns the number of conflict hunks in data.
func Count(data []byte) int {
	n := 0
	for _, line := range splitLines(data) {
		if isBegin(line) {
			n++
		}
	}
	return n
}

// Resolve rewrites every conflict hunk in data according to strat and returns
// the new bytes plus Stats. It errors on malformed markers (a begin without a
// matching separator and end) rather than risk silently corrupting the file.
func Resolve(data []byte, strat Strategy) ([]byte, Stats, error) {
	lines := splitLines(data)
	var out []string
	var stats Stats

	i := 0
	for i < len(lines) {
		if !isBegin(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}

		// Parse one hunk: begin, our lines, sep, their lines, end.
		stats.Total++
		var ours, theirs []string
		j := i + 1
		for j < len(lines) && !isSep(lines[j]) {
			if isBegin(lines[j]) || isEnd(lines[j]) {
				return nil, Stats{}, fmt.Errorf("malformed conflict markers near line %d", j+1)
			}
			ours = append(ours, lines[j])
			j++
		}
		if j >= len(lines) {
			return nil, Stats{}, fmt.Errorf("unterminated conflict: missing '=======' separator")
		}
		j++ // consume the separator
		for j < len(lines) && !isEnd(lines[j]) {
			if isBegin(lines[j]) || isSep(lines[j]) {
				return nil, Stats{}, fmt.Errorf("malformed conflict markers near line %d", j+1)
			}
			theirs = append(theirs, lines[j])
			j++
		}
		if j >= len(lines) {
			return nil, Stats{}, fmt.Errorf("unterminated conflict: missing '>>>>>>>' marker")
		}
		j++ // consume the end marker

		resolved, ok := resolveHunk(ours, theirs, strat, lines[i:j])
		out = append(out, resolved...)
		if ok {
			stats.Resolved++
		} else {
			stats.Remaining++
		}
		i = j
	}

	return []byte(strings.Join(out, "")), stats, nil
}

// resolveHunk returns the replacement lines for one hunk and whether it was
// resolved. When Auto cannot decide, it returns the original hunk (markers and
// all) and ok=false so the caller can report it as still-conflicted.
func resolveHunk(ours, theirs []string, strat Strategy, original []string) ([]string, bool) {
	switch strat {
	case Ours:
		return ours, true
	case Theirs:
		return theirs, true
	case Union:
		return append(append([]string{}, ours...), theirs...), true
	case Auto:
		switch {
		case equalLines(ours, theirs):
			return ours, true
		case len(ours) == 0:
			return theirs, true
		case len(theirs) == 0:
			return ours, true
		default:
			return original, false // genuinely ambiguous — leave it
		}
	default:
		return original, false
	}
}

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

func isBegin(line string) bool { return strings.HasPrefix(trimEnd(line), markerBegin) }
func isEnd(line string) bool   { return strings.HasPrefix(trimEnd(line), markerEnd) }
func isSep(line string) bool   { return trimEnd(line) == markerSep }

func trimEnd(line string) string { return strings.TrimRight(line, "\r\n") }

// splitLines splits data into lines, each retaining its trailing newline. A
// final line without a newline is kept as-is; empty input yields no lines. This
// preserves byte-exactness so a round-trip through Resolve changes only the
// conflict hunks.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var lines []string
	start := 0
	for i := range len(data) {
		if data[i] == '\n' {
			lines = append(lines, string(data[start:i+1]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
