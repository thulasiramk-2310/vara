// Package smartmerge implements the structural (semantic) merge driver from
// VARA-RFC-0025 §5. It sits ABOVE the frozen engine: it never touches pkg/diff
// or the object/index formats. The command layer asks it, per conflicted path,
// whether a structural driver is enabled and — if so — to merge the parsed
// structure instead of raw lines, so two logically independent edits to one
// structured file compose cleanly instead of false-conflicting.
//
// Phase 1 supports JSON. Selection is opt-in per repository (RFC-0025 §4): the
// driver engages only when config sets `merge.driver.<ext> = structured-json`;
// otherwise callers use the existing line/diff3 merge unchanged. On any parse
// failure the caller falls back to the line merge, so a malformed structured
// file can never lose data or trip a parser bug.
package smartmerge

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

// structuralDriver is the config value that enables structural merge for a type.
const structuralDriver = "structured-json"

// StructuralJSON reports whether the JSON structural driver is enabled for path.
// It is opt-in: true only when repo config sets `[merge] driver.json =
// structured-json`. A nil config (or any other value) yields false, so the
// default everywhere stays the line merge.
func StructuralJSON(path string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext != "json" {
		return false
	}
	v, ok := cfg.Get("merge", "", "driver."+ext)
	return ok && v == structuralDriver
}

// MergeJSON performs a three-way structural merge of JSON documents.
//
// Returns:
//   - merged  : the canonical merged document (sorted keys, 2-space indent, one
//     trailing newline) — valid only when clean is true.
//   - clean   : true when the structural merge fully resolved (no divergent leaf).
//   - parseOK : false when any side failed to parse as JSON.
//
// The caller falls back to the line merge whenever parseOK is false (unparseable)
// or clean is false (a genuine same-key conflict Phase 1 leaves to the line
// driver rather than rendering per-key markers — RFC-0025 §6, deferred).
func MergeJSON(base, ours, theirs []byte) (merged []byte, clean bool, parseOK bool) {
	var b, o, t any
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, false, false
	}
	if err := json.Unmarshal(ours, &o); err != nil {
		return nil, false, false
	}
	if err := json.Unmarshal(theirs, &t); err != nil {
		return nil, false, false
	}

	m, conflict := valueMerge(b, o, t)
	if conflict {
		return nil, false, true // parsed, but a leaf genuinely diverges
	}
	// json.Marshal sorts object keys, giving a canonical, order-independent form.
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, false, false
	}
	return append(out, '\n'), true, true
}

// valueMerge three-way merges two present values against their common base,
// returning the merged value and whether a genuine conflict occurred.
func valueMerge(base, ours, theirs any) (any, bool) {
	switch {
	case eq(ours, theirs): // both sides agree (incl. both made the same change)
		return ours, false
	case eq(base, ours): // only theirs changed
		return theirs, false
	case eq(base, theirs): // only ours changed
		return ours, false
	}

	// Both changed differently. If both are objects, merge key by key; only the
	// keys that truly diverge conflict. Anything else (scalars, arrays, a type
	// change) is an atomic conflict in Phase 1.
	om, ook := ours.(map[string]any)
	tm, tok := theirs.(map[string]any)
	if ook && tok {
		bm, _ := base.(map[string]any) // nil if base wasn't an object; treated as empty
		return objectMerge(bm, om, tm)
	}
	return ours, true
}

// objectMerge merges three JSON objects key by key.
func objectMerge(base, ours, theirs map[string]any) (any, bool) {
	out := map[string]any{}
	conflict := false

	seen := map[string]bool{}
	for _, m := range []map[string]any{base, ours, theirs} {
		for k := range m {
			seen[k] = true
		}
	}

	for k := range seen {
		bv, bh := base[k]
		ov, oh := ours[k]
		tv, th := theirs[k]
		present, val, c := keyMerge(bh, bv, oh, ov, th, tv)
		if c {
			conflict = true
		}
		if present {
			out[k] = val
		}
	}
	return out, conflict
}

// keyMerge resolves one key across base/ours/theirs, where the *h booleans mark
// whether the key is present on that side. It returns whether the key survives,
// its merged value, and whether it conflicts.
func keyMerge(bh bool, bv any, oh bool, ov any, th bool, tv any) (present bool, val any, conflict bool) {
	switch {
	case oh == th && (!oh || eq(ov, tv)): // ours == theirs (present-and-equal, or both absent)
		return oh, ov, false
	case bh == oh && (!bh || eq(bv, ov)): // ours == base → take theirs' decision
		return th, tv, false
	case bh == th && (!bh || eq(bv, tv)): // theirs == base → take ours' decision
		return oh, ov, false
	case oh && th: // both present and changed differently → recurse
		var baseVal any
		if bh {
			baseVal = bv
		}
		m, c := valueMerge(baseVal, ov, tv)
		return true, m, c
	default: // one side modified, the other deleted → structural modify/delete conflict
		if oh {
			return true, ov, true
		}
		return th, tv, true
	}
}

// eq is deep structural equality over decoded JSON values.
func eq(a, b any) bool { return reflect.DeepEqual(a, b) }
