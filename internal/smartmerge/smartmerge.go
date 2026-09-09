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
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

// Driver config values that enable a structural merge for a file type.
const (
	driverJSON = "structured-json"
	driverYAML = "structured-yaml"
)

// SelectDriver returns the structural driver enabled for path, or "" for the
// default line merge. Opt-in per repo (RFC-0025 §4): a structural driver engages
// only when config sets a matching `merge.driver.<ext>` value; a nil config or
// any other value keeps the line merge.
func SelectDriver(path string, cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	key := ext
	if ext == "yml" {
		key = "yaml" // .yml and .yaml share the `merge.driver.yaml` setting
	}
	v, ok := cfg.Get("merge", "", "driver."+key)
	if !ok {
		return ""
	}
	switch {
	case ext == "json" && v == driverJSON:
		return driverJSON
	case (ext == "yaml" || ext == "yml") && v == driverYAML:
		return driverYAML
	}
	return ""
}

// Merge runs the named structural driver, returning (merged, clean, parseOK) —
// the same contract as the per-format functions. An unknown driver reports
// parseOK=false so the caller falls back to the line merge.
func Merge(driver string, base, ours, theirs []byte) (merged []byte, clean bool, parseOK bool) {
	switch driver {
	case driverJSON:
		return MergeJSON(base, ours, theirs)
	case driverYAML:
		return MergeYAML(base, ours, theirs)
	default:
		return nil, false, false
	}
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

// MergeYAML performs a three-way structural merge of YAML documents, reusing the
// same tree merge as JSON. Output is canonical YAML (yaml.v3 sorts map keys), so
// it is order-independent.
//
// NOTE: decoding into a plain tree drops comments, so a clean structural YAML
// merge does not preserve them (RFC-0025 §12 — comment-preserving merge is
// deferred). The caller falls back to the line merge on parse failure or a
// genuine same-leaf conflict, so this only reformats a file it actually merges.
func MergeYAML(base, ours, theirs []byte) (merged []byte, clean bool, parseOK bool) {
	var b, o, t any
	if err := yaml.Unmarshal(base, &b); err != nil {
		return nil, false, false
	}
	if err := yaml.Unmarshal(ours, &o); err != nil {
		return nil, false, false
	}
	if err := yaml.Unmarshal(theirs, &t); err != nil {
		return nil, false, false
	}

	m, conflict := valueMerge(normalizeYAML(b), normalizeYAML(o), normalizeYAML(t))
	if conflict {
		return nil, false, true
	}
	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, false, false
	}
	return out, true, true
}

// normalizeYAML converts any map[any]any (which some YAML shapes decode to) into
// map[string]any recursively, so the shared tree merge sees the same value model
// as JSON regardless of the decoder's map representation.
func normalizeYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeYAML(val)
		}
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalizeYAML(val)
		}
		return out
	case []any:
		for i, val := range t {
			t[i] = normalizeYAML(val)
		}
		return t
	default:
		return v
	}
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
