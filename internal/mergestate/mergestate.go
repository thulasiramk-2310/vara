// Package mergestate persists the content-bearing conflict sidecar
// (.vara/CONFLICTS) that accompanies .vara/MERGE_HEAD during a conflicted merge.
//
// VARA's index has one blob per path and no stage 1/2/3 slots, and the merge
// engine returns only the list of conflicted *paths*. For content (edit/edit and
// add/add) conflicts it renders the sides into lossy `<<<<<<<` marker text; for
// structural (modify/delete) conflicts it writes nothing at all — the working
// tree is simply left at the "ours" state. Marker text is therefore neither
// lossless nor even present for every conflict, so it cannot be the source of
// truth for what is unresolved.
//
// This sidecar is that source of truth. Per conflicted path it records:
//   - the conflict Kind (content vs modify/delete),
//   - each side (base/ours/theirs) as a Side that distinguishes "absent on that
//     side" (Present=false) from "present but empty" (Present=true, Blob=hash of
//     ""), which modify/delete resolution depends on, and
//   - a Resolved flag the resolver sets when the path has been settled.
//
// `vara resolve` reconstructs conflicts from the recorded blobs (making it
// re-runnable and base-aware), `vara commit` refuses while any entry is
// unresolved, and `vara merge --abort` uses the entry set as the definitive list
// of merge-involved paths. It lives above the frozen engine and stores only blob
// IDs the engine already produced; it adds no field to the index format.
// See docs/merge-state.md.
package mergestate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FileName is the sidecar's basename inside the .vara directory.
const FileName = "CONFLICTS"

// Version is the on-disk schema version of the sidecar.
const Version = 1

// Kind classifies a conflict. A path is only ever recorded under one kind.
type Kind string

const (
	// KindContent is an edit/edit or add/add conflict: the path exists on both
	// ours and theirs with differing content. It is resolvable by re-rendering
	// the three sides and choosing/combining lines (ours/theirs/union/auto).
	KindContent Kind = "content"
	// KindModifyDelete is a structural conflict: exactly one of ours/theirs has
	// the path and the other deleted it. "Combine a file with its absence" is
	// meaningless, so it is resolvable only by an explicit --ours/--theirs choice.
	KindModifyDelete Kind = "modify_delete"
)

// Side is one side (base/ours/theirs) of a conflicted path. Present=false means
// the path does not exist on that side — which is distinct from a present but
// empty file (Present=true with Blob set to the hash of empty content).
type Side struct {
	Present bool   `json:"present"`
	Blob    string `json:"blob,omitempty"` // hex blob ID; meaningful only when Present
}

// Entry records one conflicted path.
type Entry struct {
	Path     string `json:"path"`
	Kind     Kind   `json:"kind"`
	Base     Side   `json:"base"`
	Ours     Side   `json:"ours"`
	Theirs   Side   `json:"theirs"`
	Resolved bool   `json:"resolved"`
}

// State is the full sidecar payload.
type State struct {
	Version    int     `json:"version"`
	OurLabel   string  `json:"our_label"`
	TheirLabel string  `json:"their_label"`
	Entries    []Entry `json:"entries"`
	// MergeTouched lists every path the merge changed relative to the pre-merge
	// HEAD (cleanly-merged and merge-deleted paths, plus content-conflicted ones).
	// Captured at conflict time so `vara merge --abort` can tell a merge-authored
	// change from a change the user made afterwards — the current index cannot,
	// since a later `vara rm`/`vara add` mutates it. Modify/delete conflicts are
	// NOT here (the engine writes nothing for them); Entries covers those.
	MergeTouched []string `json:"merge_touched"`
}

// MergeInvolved reports whether a path was part of the merge — either a recorded
// conflict or a merge-touched path.
func (s *State) MergeInvolved(path string) bool {
	for _, p := range s.MergeTouched {
		if p == path {
			return true
		}
	}
	_, ok := s.Lookup(path)
	return ok
}

// Paths returns every conflicted path in record order.
func (s *State) Paths() []string {
	out := make([]string, 0, len(s.Entries))
	for _, e := range s.Entries {
		out = append(out, e.Path)
	}
	return out
}

// UnresolvedPaths returns the paths whose entries are not yet resolved.
func (s *State) UnresolvedPaths() []string {
	var out []string
	for _, e := range s.Entries {
		if !e.Resolved {
			out = append(out, e.Path)
		}
	}
	return out
}

// AllResolved reports whether every entry has been resolved.
func (s *State) AllResolved() bool {
	for _, e := range s.Entries {
		if !e.Resolved {
			return false
		}
	}
	return true
}

// Lookup returns the entry for path, if present.
func (s *State) Lookup(path string) (Entry, bool) {
	for _, e := range s.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return Entry{}, false
}

// SetResolved marks path's entry resolved (or not). It reports whether a matching
// entry existed. Mutates the receiver; persist with Write.
func (s *State) SetResolved(path string, resolved bool) bool {
	for i := range s.Entries {
		if s.Entries[i].Path == path {
			s.Entries[i].Resolved = resolved
			return true
		}
	}
	return false
}

func path(varaDir string) string { return filepath.Join(varaDir, FileName) }

// Write persists the sidecar atomically (temp file + rename), mode 0644.
func Write(varaDir string, s *State) error {
	if s.Version == 0 {
		s.Version = Version
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	p := path(varaDir)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Read loads the sidecar. The bool is false (with nil error) when no sidecar
// exists, i.e. either no merge is in progress or the merge was begun by an older
// binary that did not write one.
func Read(varaDir string) (*State, bool, error) {
	data, err := os.ReadFile(path(varaDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, false, err
	}
	return &s, true, nil
}

// Clear removes the sidecar. A missing file is not an error.
func Clear(varaDir string) error {
	err := os.Remove(path(varaDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
