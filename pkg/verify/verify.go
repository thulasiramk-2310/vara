// Package verify implements repository integrity checking for VARA.
//
// RFC:
// VARA-RFC-0003 Repository Layout
// VARA-RFC-0002 Object Format
// VARA-RFC-0004 References
// VARA-RFC-0005 Index
// VARA-RFC-0006 Locking and Transactions
// VARA-RFC-0009 Undo and Recovery
//
// Responsibilities:
// - Verify SHA-256 integrity of every stored object.
// - Confirm tree entries and commit parents resolve to existing objects.
// - Detect cycles in the commit DAG.
// - Validate HEAD and branch references.
// - Parse-validate the index, journal, and snapshot metadata.
//
// This package MUST NOT:
// - Import commands, transaction, locking, or any higher-layer package.
// - Mutate any repository state.
package verify

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// VerifyError is a single integrity violation.
type VerifyError struct {
	Name    string // object ID, ref name, file path, etc.
	Problem string // human-readable description
}

func (e VerifyError) Error() string {
	return fmt.Sprintf("%s: %s", e.Name, e.Problem)
}

// CategoryResult holds the outcome for one verification domain.
type CategoryResult struct {
	Verified int
	Errors   []VerifyError
}

func (c *CategoryResult) ok(name string) {
	c.Verified++
}

func (c *CategoryResult) fail(name, problem string) {
	c.Errors = append(c.Errors, VerifyError{Name: name, Problem: problem})
}

func (c *CategoryResult) healthy() bool { return len(c.Errors) == 0 }

// Report is the structured output of Verify. Every field is populated even
// when verification succeeds. Check Report.Healthy for a single pass/fail.
type Report struct {
	Objects   CategoryResult
	Trees     CategoryResult
	Commits   CategoryResult
	Refs      CategoryResult
	Index     CategoryResult
	Journal   CategoryResult
	Snapshots CategoryResult
	Graph     CategoryResult
	Healthy   bool
}

// hexDirRe matches exactly 2 lowercase hex characters — shard directory names.
var hexDirRe = regexp.MustCompile(`^[0-9a-f]{2}$`)

// hexFileRe matches exactly 62 lowercase hex characters — object file names.
var hexFileRe = regexp.MustCompile(`^[0-9a-f]{62}$`)

// Verify runs all integrity checks against the repository rooted at varaDir and
// returns a complete Report. It never modifies any repository state.
func Verify(varaDir string) (Report, error) {
	var r Report

	store := object.NewStore(varaDir)

	// Phase 1 — verify raw object files and collect IDs by type.
	allObjects, blobs, trees, commits, err := scanObjects(varaDir, store, &r.Objects)
	if err != nil {
		return r, fmt.Errorf("verify: scan objects: %w", err)
	}

	// Phase 2 — verify tree internal references.
	verifyTrees(store, trees, allObjects, &r.Trees)

	// Phase 3 — verify commit internal references and collect the DAG.
	dag := verifyCommits(store, commits, allObjects, &r.Commits)

	// Phase 4 — DAG cycle detection.
	verifyDAG(dag, &r.Graph)

	// Phase 5 — verify refs.
	verifyRefs(varaDir, allObjects, &r.Refs)

	// Phase 6 — verify index.
	verifyIndex(varaDir, &r.Index)

	// Phase 7 — verify journal entries.
	verifyJournal(varaDir, &r.Journal)

	// Phase 8 — verify snapshot archives.
	verifySnapshots(varaDir, &r.Snapshots)

	_ = blobs // blobs have no internal pointers to check

	r.Healthy = r.Objects.healthy() &&
		r.Trees.healthy() &&
		r.Commits.healthy() &&
		r.Refs.healthy() &&
		r.Index.healthy() &&
		r.Journal.healthy() &&
		r.Snapshots.healthy() &&
		r.Graph.healthy()

	return r, nil
}

// scanObjects walks .vara/<2hex>/<62hex> and verifies each file's hash matches
// its path. It categorises objects by type.
func scanObjects(
	varaDir string,
	store *object.Store,
	result *CategoryResult,
) (all, blobs, trees, commits map[types.ObjectID]struct{}, _ error) {
	all = make(map[types.ObjectID]struct{})
	blobs = make(map[types.ObjectID]struct{})
	trees = make(map[types.ObjectID]struct{})
	commits = make(map[types.ObjectID]struct{})

	entries, err := os.ReadDir(varaDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	for _, shard := range entries {
		if !shard.IsDir() || !hexDirRe.MatchString(shard.Name()) {
			continue
		}
		shardDir := filepath.Join(varaDir, shard.Name())
		files, err := os.ReadDir(shardDir)
		if err != nil {
			result.fail(shard.Name(), fmt.Sprintf("cannot read shard directory: %v", err))
			continue
		}
		for _, f := range files {
			if f.IsDir() || !hexFileRe.MatchString(f.Name()) {
				continue
			}
			hexID := shard.Name() + f.Name()
			base, err := types.ParseHex(hexID)
			if err != nil {
				result.fail(hexID, fmt.Sprintf("invalid hex name: %v", err))
				continue
			}
			id := types.ObjectID(base)

			obj, err := store.Read(id)
			if err != nil {
				result.fail(hexID[:7], fmt.Sprintf("read/hash error: %v", err))
				continue
			}
			result.ok(hexID[:7])
			all[id] = struct{}{}

			switch obj.Type() {
			case object.TypeBlob:
				blobs[id] = struct{}{}
			case object.TypeTree:
				trees[id] = struct{}{}
			case object.TypeCommit:
				commits[id] = struct{}{}
			}
		}
	}
	return
}

// verifyTrees checks that every entry in every tree resolves to a known object.
func verifyTrees(
	store *object.Store,
	trees, allObjects map[types.ObjectID]struct{},
	result *CategoryResult,
) {
	for id := range trees {
		obj, _ := store.Read(id)
		tree := obj.(*object.Tree)
		name := id.String()[:7]
		treeOK := true
		for _, entry := range tree.Entries {
			if _, exists := allObjects[entry.Hash]; !exists {
				result.fail(name, fmt.Sprintf("entry '%s' → missing object %s", entry.Name, entry.Hash.String()[:7]))
				treeOK = false
			}
		}
		if treeOK {
			result.ok(name)
		}
	}
}

// verifyCommits checks every commit's tree and parents exist, and builds the DAG map.
// dag maps commitID → slice of parent commitIDs.
func verifyCommits(
	store *object.Store,
	commits, allObjects map[types.ObjectID]struct{},
	result *CategoryResult,
) map[types.ObjectID][]types.ObjectID {
	dag := make(map[types.ObjectID][]types.ObjectID, len(commits))

	for id := range commits {
		obj, _ := store.Read(id)
		c := obj.(*object.Commit)
		name := id.String()[:7]
		commitOK := true

		treeID := types.ObjectID(c.TreeHash)
		if _, exists := allObjects[treeID]; !exists {
			result.fail(name, fmt.Sprintf("tree %s missing", treeID.String()[:7]))
			commitOK = false
		}

		var parents []types.ObjectID
		for _, p := range c.Parents {
			pid := types.ObjectID(p)
			if _, exists := allObjects[pid]; !exists {
				result.fail(name, fmt.Sprintf("parent %s missing", pid.String()[:7]))
				commitOK = false
			}
			parents = append(parents, pid)
		}
		dag[id] = parents

		if commitOK {
			result.ok(name)
		}
	}
	return dag
}

// verifyDAG performs a DFS-based cycle detection on the commit graph.
// A valid commit DAG must have no cycles.
func verifyDAG(dag map[types.ObjectID][]types.ObjectID, result *CategoryResult) {
	// Colors: 0=white(unvisited), 1=grey(on stack), 2=black(done)
	color := make(map[types.ObjectID]int, len(dag))
	cycleFound := false

	var dfs func(id types.ObjectID) bool
	dfs = func(id types.ObjectID) bool {
		color[id] = 1
		for _, parent := range dag[id] {
			if color[parent] == 1 {
				result.fail(id.String()[:7], fmt.Sprintf("cycle detected through parent %s", parent.String()[:7]))
				return true
			}
			if color[parent] == 0 {
				if dfs(parent) {
					return true
				}
			}
		}
		color[id] = 2
		return false
	}

	for id := range dag {
		if color[id] == 0 {
			if dfs(id) {
				cycleFound = true
				break
			}
		}
	}

	if !cycleFound {
		result.Verified = len(dag)
	}
}

// verifyRefs checks HEAD and all branch refs resolve to existing objects.
func verifyRefs(varaDir string, allObjects map[types.ObjectID]struct{}, result *CategoryResult) {
	resolver := refs.NewFSResolver(varaDir)

	// Verify HEAD.
	headID, err := resolver.Resolve("HEAD")
	if err != nil {
		result.fail("HEAD", fmt.Sprintf("cannot resolve: %v", err))
	} else if _, exists := allObjects[types.ObjectID(headID)]; !exists {
		result.fail("HEAD", fmt.Sprintf("points to missing object %s", headID.String()[:7]))
	} else {
		result.ok("HEAD")
	}

	// Verify all branch refs.
	allRefs, err := resolver.List()
	if err != nil {
		result.fail("refs", fmt.Sprintf("cannot list refs: %v", err))
		return
	}
	for _, ref := range allRefs {
		id := types.ObjectID(ref.ObjectID)
		if _, exists := allObjects[id]; !exists {
			result.fail(ref.Name, fmt.Sprintf("points to missing object %s", id.String()[:7]))
		} else {
			result.ok(ref.Name)
		}
	}
}

// verifyIndex checks that the index file parses without error.
func verifyIndex(varaDir string, result *CategoryResult) {
	idxPath := filepath.Join(varaDir, "index")
	data, err := os.ReadFile(idxPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.ok("index") // empty repository
			return
		}
		result.fail("index", fmt.Sprintf("cannot read: %v", err))
		return
	}
	if len(data) == 0 {
		result.ok("index")
		return
	}
	if _, err := index.Deserialize(data); err != nil {
		result.fail("index", fmt.Sprintf("parse error: %v", err))
		return
	}
	result.ok("index")
}

// verifyJournal checks that every journal entry parses as valid JSON with
// the required fields (transaction, state, command).
func verifyJournal(varaDir string, result *CategoryResult) {
	journalDir := filepath.Join(varaDir, "journal")
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return // no journal yet
		}
		result.fail("journal", fmt.Sprintf("cannot read directory: %v", err))
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(journalDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			result.fail(e.Name(), fmt.Sprintf("cannot read: %v", err))
			continue
		}
		var j struct {
			ID      string `json:"transaction"`
			State   string `json:"state"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(data, &j); err != nil {
			result.fail(e.Name(), fmt.Sprintf("invalid JSON: %v", err))
			continue
		}
		if j.ID == "" || j.State == "" || j.Command == "" {
			result.fail(e.Name(), "missing required fields (transaction, state, or command)")
			continue
		}
		result.ok(e.Name())
	}
}

// verifySnapshots opens each snapshot archive, reads the tar header, and
// confirms the archive is structurally valid.
func verifySnapshots(varaDir string, result *CategoryResult) {
	snapsDir := filepath.Join(varaDir, "snapshots")
	entries, err := os.ReadDir(snapsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return // no snapshots yet
		}
		result.fail("snapshots", fmt.Sprintf("cannot read directory: %v", err))
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.zst") {
			continue
		}
		path := filepath.Join(snapsDir, e.Name())
		if err := checkSnapshot(path); err != nil {
			result.fail(e.Name(), err.Error())
		} else {
			result.ok(e.Name())
		}
	}
}

func checkSnapshot(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open: %w", err)
	}
	defer f.Close()

	dec, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("zstd init: %w", err)
	}
	defer dec.Close()

	tr := tar.NewReader(dec)
	// Read at least one header to confirm the archive is non-empty and valid.
	_, err = tr.Next()
	if err != nil && err != io.EOF {
		return fmt.Errorf("tar read: %w", err)
	}
	return nil
}
