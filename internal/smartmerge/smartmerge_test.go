package smartmerge

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

// decode re-parses merged bytes so assertions compare structure, not whitespace.
func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("merged output is not valid JSON: %v\n%s", err, b)
	}
	return v
}

func TestMergeJSONIndependentKeys(t *testing.T) {
	// The headline case: compact JSON the line merger conflicts on, because both
	// edits are on the same line — but they touch different keys.
	base := []byte(`{"a":1,"b":2}`)
	ours := []byte(`{"a":10,"b":2}`)
	theirs := []byte(`{"a":1,"b":20}`)

	merged, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || !clean {
		t.Fatalf("expected a clean structural merge, got clean=%v parseOK=%v", clean, parseOK)
	}
	want := map[string]any{"a": float64(10), "b": float64(20)}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONSameKeyConflicts(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":10}`)
	theirs := []byte(`{"a":20}`)
	_, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK {
		t.Fatal("expected parseOK")
	}
	if clean {
		t.Fatal("same key changed differently must NOT be clean (falls back to line merge)")
	}
}

func TestMergeJSONOneSideEqualsBase(t *testing.T) {
	base := []byte(`{"a":1,"b":2}`)
	ours := []byte(`{"a":1,"b":2}`) // unchanged
	theirs := []byte(`{"a":1,"b":99}`)
	merged, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || !clean {
		t.Fatalf("clean=%v parseOK=%v", clean, parseOK)
	}
	want := map[string]any{"a": float64(1), "b": float64(99)}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONAddDistinctKeys(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":1,"x":true}`)
	theirs := []byte(`{"a":1,"y":false}`)
	merged, clean, _ := MergeJSON(base, ours, theirs)
	if !clean {
		t.Fatal("adding distinct keys should merge clean")
	}
	want := map[string]any{"a": float64(1), "x": true, "y": false}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONModifyDeleteKeyConflicts(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":2}`) // modified
	theirs := []byte(`{}`)    // deleted
	_, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || clean {
		t.Fatalf("modify/delete of a key must conflict, got clean=%v parseOK=%v", clean, parseOK)
	}
}

func TestMergeJSONNestedIndependent(t *testing.T) {
	base := []byte(`{"cfg":{"a":1,"b":2}}`)
	ours := []byte(`{"cfg":{"a":9,"b":2}}`)
	theirs := []byte(`{"cfg":{"a":1,"b":8}}`)
	merged, clean, _ := MergeJSON(base, ours, theirs)
	if !clean {
		t.Fatal("independent nested-key edits should merge clean")
	}
	want := map[string]any{"cfg": map[string]any{"a": float64(9), "b": float64(8)}}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONMalformedFallsBack(t *testing.T) {
	_, _, parseOK := MergeJSON([]byte(`{`), []byte(`{"a":1}`), []byte(`{"a":2}`))
	if parseOK {
		t.Fatal("unparseable input must report parseOK=false so the caller falls back to line merge")
	}
}

// TestMergeJSONOrderIndependent: swapping ours/theirs yields identical bytes.
func TestMergeJSONOrderIndependent(t *testing.T) {
	base := []byte(`{"a":1,"b":2}`)
	x := []byte(`{"a":10,"b":2}`)
	y := []byte(`{"a":1,"b":20}`)
	m1, c1, _ := MergeJSON(base, x, y)
	m2, c2, _ := MergeJSON(base, y, x)
	if !c1 || !c2 {
		t.Fatal("both directions should be clean")
	}
	if string(m1) != string(m2) {
		t.Fatalf("structural merge must be order-independent:\n%s\nvs\n%s", m1, m2)
	}
}

func TestStructuralJSONSelection(t *testing.T) {
	if StructuralJSON("x.json", nil) {
		t.Fatal("nil config must disable structural merge")
	}
	cfg := config.New()
	if StructuralJSON("x.json", cfg) {
		t.Fatal("unset config must default to line merge (opt-in)")
	}
	cfg.Set("merge", "", "driver.json", "structured-json")
	if !StructuralJSON("x.json", cfg) {
		t.Fatal("enabled config must select the structural driver for .json")
	}
	if !StructuralJSON("dir/Config.JSON", cfg) {
		t.Fatal("extension match must be case-insensitive")
	}
	if StructuralJSON("notes.txt", cfg) {
		t.Fatal("non-json paths must stay on the line merge")
	}
	cfg.Set("merge", "", "driver.json", "line")
	if StructuralJSON("x.json", cfg) {
		t.Fatal("an explicit non-structural value must disable it")
	}
}
