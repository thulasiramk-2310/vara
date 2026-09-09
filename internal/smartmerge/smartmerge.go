// Package smartmerge implements the structural (semantic) merge driver from
// VARA-RFC-0025. It sits ABOVE the frozen engine (never touches pkg/diff or the
// object/index formats): the command layer asks it, per conflicted path, whether
// a structural driver is enabled and to merge parsed structure instead of raw
// lines, so two logically independent edits to one structured file compose
// cleanly instead of false-conflicting.
//
// A clean structural merge auto-resolves. A genuine same-key divergence is
// rendered (for JSON) as a per-key conflict: the clean keys stay merged and only
// the diverging value is wrapped in <<<<<<< / ||||||| / ======= / >>>>>>> markers,
// so the conflict points at the exact key. YAML currently renders only the clean
// case; a YAML conflict falls back to the line merge (RFC-0025 §12).
//
// Selection is opt-in per repo (RFC-0025 §4): a driver engages only when config
// sets `merge.driver.<ext> = structured-json|structured-yaml`. On any parse
// failure the caller falls back to the line merge, so a malformed structured file
// can never lose data or trip a parser bug.
package smartmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

// Driver config values that enable a structural merge for a file type.
const (
	driverJSON = "structured-json"
	driverYAML = "structured-yaml"
)

// Result is the outcome of a structural merge.
type Result struct {
	Merged    []byte // clean merged doc, or a per-key conflict-marker rendering
	Conflicts int    // number of diverging locations (0 == clean)
	ParseOK   bool   // false when a side failed to parse
	Rendered  bool   // true when Merged holds a usable per-key marker rendering
	//          for a conflicted merge; when false with Conflicts>0 the caller
	//          falls back to the line merge (e.g. YAML has no renderer yet).
}

// absentT marks a side that lacks a key (a structural modify/delete).
type absentT struct{}

var absent = absentT{}

// conflictMark is a sentinel placed in the merged tree where a value diverges.
// Any of its sides may be `absent` (that side deleted the key).
type conflictMark struct {
	base, ours, theirs any
}

// SelectDriver returns the structural driver enabled for path, or "" for the
// default line merge. Opt-in per repo: a structural driver engages only when
// config sets a matching `merge.driver.<ext>` value.
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

// Merge runs the named structural driver.
func Merge(driver string, base, ours, theirs []byte, ourLabel, theirLabel string) Result {
	switch driver {
	case driverJSON:
		return MergeJSON(base, ours, theirs, ourLabel, theirLabel)
	case driverYAML:
		return MergeYAML(base, ours, theirs)
	default:
		return Result{}
	}
}

// MergeJSON performs a three-way structural merge of JSON documents. A clean
// merge returns canonical JSON (sorted keys, 2-space indent); a conflict returns
// a per-key marker rendering (Rendered=true).
func MergeJSON(base, ours, theirs []byte, ourLabel, theirLabel string) Result {
	var b, o, t any
	if json.Unmarshal(base, &b) != nil || json.Unmarshal(ours, &o) != nil || json.Unmarshal(theirs, &t) != nil {
		return Result{} // ParseOK false → caller falls back to line merge
	}
	m, n := mergeTree(b, o, t)
	if n == 0 {
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return Result{} // unexpected; fall back
		}
		return Result{Merged: append(out, '\n'), Conflicts: 0, ParseOK: true}
	}
	return Result{Merged: renderJSON(m, ourLabel, theirLabel), Conflicts: n, ParseOK: true, Rendered: true}
}

// MergeYAML performs a three-way structural merge of YAML documents, reusing the
// same tree merge. Only the clean case is rendered (canonical YAML — yaml.v3
// sorts keys); a genuine conflict returns Rendered=false so the caller falls back
// to the line merge. Comments are dropped by decoding (RFC-0025 §12).
func MergeYAML(base, ours, theirs []byte) Result {
	var b, o, t any
	if yaml.Unmarshal(base, &b) != nil || yaml.Unmarshal(ours, &o) != nil || yaml.Unmarshal(theirs, &t) != nil {
		return Result{}
	}
	m, n := mergeTree(normalizeYAML(b), normalizeYAML(o), normalizeYAML(t))
	if n == 0 {
		out, err := yaml.Marshal(m)
		if err != nil {
			return Result{}
		}
		return Result{Merged: out, Conflicts: 0, ParseOK: true}
	}
	return Result{Conflicts: n, ParseOK: true} // Rendered false → line-merge fallback
}

// normalizeYAML converts any map[any]any into map[string]any recursively, so the
// shared tree merge sees the same value model as JSON.
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

// mergeTree three-way merges two present values against their common base,
// returning the merged tree (with conflictMark sentinels at diverging spots) and
// the number of conflicts.
func mergeTree(base, ours, theirs any) (any, int) {
	switch {
	case eq(ours, theirs): // both agree (incl. both made the same change)
		return ours, 0
	case eq(base, ours): // only theirs changed
		return theirs, 0
	case eq(base, theirs): // only ours changed
		return ours, 0
	}
	om, ook := ours.(map[string]any)
	tm, tok := theirs.(map[string]any)
	if ook && tok {
		bm, _ := base.(map[string]any) // nil if base wasn't an object → treated as empty
		return objectMerge(bm, om, tm)
	}
	// Scalars, arrays, or a type change that both sides altered → atomic conflict.
	return conflictMark{base: base, ours: ours, theirs: theirs}, 1
}

func objectMerge(base, ours, theirs map[string]any) (any, int) {
	out := map[string]any{}
	total := 0

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
		total += c
		if present {
			out[k] = val
		}
	}
	return out, total
}

func keyMerge(bh bool, bv any, oh bool, ov any, th bool, tv any) (present bool, val any, conflicts int) {
	switch {
	case oh == th && (!oh || eq(ov, tv)): // ours == theirs
		return oh, ov, 0
	case bh == oh && (!bh || eq(bv, ov)): // ours == base → take theirs
		return th, tv, 0
	case bh == th && (!bh || eq(bv, tv)): // theirs == base → take ours
		return oh, ov, 0
	case oh && th: // both present, changed differently → recurse
		var baseVal any
		if bh {
			baseVal = bv
		}
		m, c := mergeTree(baseVal, ov, tv)
		return true, m, c
	default: // one side modified, the other deleted → structural modify/delete
		return true, conflictMark{
			base:   sideOrAbsent(bh, bv),
			ours:   sideOrAbsent(oh, ov),
			theirs: sideOrAbsent(th, tv),
		}, 1
	}
}

func sideOrAbsent(has bool, v any) any {
	if has {
		return v
	}
	return absent
}

// --- per-key JSON conflict renderer ----------------------------------------

func renderJSON(v any, ourLabel, theirLabel string) []byte {
	var b bytes.Buffer
	writeNode(&b, v, "", ourLabel, theirLabel)
	b.WriteByte('\n')
	return b.Bytes()
}

// writeNode emits a JSON value. conflictMark nodes become marker blocks; clean
// keys/elements are serialized canonically. Marker lines sit at column 0 (git
// convention); the result is intentionally not valid JSON while conflicted.
func writeNode(b *bytes.Buffer, v any, indent, ourLabel, theirLabel string) {
	switch t := v.(type) {
	case conflictMark:
		writeConflict(b, t, indent, ourLabel, theirLabel)
	case map[string]any:
		if len(t) == 0 {
			b.WriteString("{}")
			return
		}
		keys := sortedKeys(t)
		b.WriteString("{\n")
		child := indent + "  "
		for i, k := range keys {
			kb, _ := json.Marshal(k)
			last := i == len(keys)-1
			if cm, ok := t[k].(conflictMark); ok {
				b.WriteString(child)
				b.Write(kb)
				b.WriteString(":\n")
				writeConflict(b, cm, child, ourLabel, theirLabel) // ends with '\n'
			} else {
				b.WriteString(child)
				b.Write(kb)
				b.WriteString(": ")
				writeNode(b, t[k], child, ourLabel, theirLabel)
				if !last {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
		}
		b.WriteString(indent + "}")
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		child := indent + "  "
		for i, el := range t {
			last := i == len(t)-1
			if cm, ok := el.(conflictMark); ok {
				writeConflict(b, cm, child, ourLabel, theirLabel)
			} else {
				b.WriteString(child)
				writeNode(b, el, child, ourLabel, theirLabel)
				if !last {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
		}
		b.WriteString(indent + "]")
	default:
		sb, _ := json.Marshal(t)
		b.Write(sb)
	}
}

func writeConflict(b *bytes.Buffer, cm conflictMark, indent, ourLabel, theirLabel string) {
	b.WriteString("<<<<<<< " + ourLabel + "\n")
	writeSide(b, cm.ours, indent)
	b.WriteString("||||||| base\n")
	writeSide(b, cm.base, indent)
	b.WriteString("=======\n")
	writeSide(b, cm.theirs, indent)
	b.WriteString(">>>>>>> " + theirLabel + "\n")
}

// writeSide renders one side of a conflict, or nothing when that side is absent
// (a deletion), at the given indent.
func writeSide(b *bytes.Buffer, v any, indent string) {
	if _, isAbsent := v.(absentT); isAbsent {
		return
	}
	sb, err := json.MarshalIndent(v, indent, "  ")
	if err != nil {
		return
	}
	b.WriteString(indent)
	b.Write(sb)
	b.WriteByte('\n')
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// eq is deep structural equality over decoded values.
func eq(a, b any) bool { return reflect.DeepEqual(a, b) }
