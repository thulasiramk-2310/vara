// Package graphindex implements VARA-RFC-0013.
//
// RFC:
// VARA-RFC-0013 Commit Graph Index
//
// Responsibilities:
// - Build the commit graph index from the object store (O(N) object reads, once).
// - Persist the index as .vara/graph.idx in the binary format specified by RFC-0013.
// - Load and checksum-validate the index in a single file read.
// - Provide O(1) commit lookup and in-memory BFS for history traversal.
// - Expose Invalidate() so the caller deletes the index after mutations.
//
// This package MUST NOT:
// - Import commands, transaction, locking, refs, or any higher-layer package.
// - Treat the index as authoritative; it is always rebuildable.
package graphindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

const (
	indexFile = "graph.idx"
	magic     = "VAGI"
	version   = uint32(1)
	entrySize = 32 + 4 + 8 + 4 + 4 + 4 // 56 bytes — see RFC-0013 §4.2
)

// Entry is the in-memory representation of one commit in the index.
type Entry struct {
	ID         types.CommitID
	Generation uint32
	Timestamp  int64
	Parents    []types.CommitID
	Author     string
	Message    string
}

// Index is the in-memory commit graph index.
type Index struct {
	byID map[types.CommitID]*Entry
	all  []*Entry
}

// Lookup returns the Entry for a commit ID, or nil if not present.
func (idx *Index) Lookup(id types.CommitID) *Entry {
	return idx.byID[id]
}

// All returns all entries in the index (order is undefined).
func (idx *Index) All() []*Entry { return idx.all }

// Len returns the number of commits in the index.
func (idx *Index) Len() int { return len(idx.all) }

// LoadOrBuild tries to load graph.idx; if absent or corrupt it calls Build
// first. This is the primary entry point for consumers.
func LoadOrBuild(varaDir string, store *object.Store) (*Index, error) {
	idx, err := Load(varaDir)
	if err == nil {
		return idx, nil
	}
	if err := Build(varaDir, store); err != nil {
		return nil, fmt.Errorf("graphindex: build: %w", err)
	}
	return Load(varaDir)
}

// Build walks all commits reachable from HEAD and all branch tips, collects
// metadata, and writes graph.idx (RFC-0013 §5.1).
func Build(varaDir string, store *object.Store) error {
	tips, err := collectTips(varaDir, store)
	if err != nil {
		return fmt.Errorf("collect tips: %w", err)
	}
	if len(tips) == 0 {
		// Empty repository — write an empty but valid index.
		return writeIndex(varaDir, nil)
	}

	// BFS from all tips.
	visited := make(map[types.CommitID]bool)
	queue := make([]types.CommitID, 0, len(tips))
	for _, tip := range tips {
		if !visited[tip] {
			visited[tip] = true
			queue = append(queue, tip)
		}
	}

	// First pass: collect all commit objects.
	rawCommits := make(map[types.CommitID]*object.Commit)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		obj, err := store.Read(types.ObjectID(id))
		if err != nil {
			return fmt.Errorf("read commit %s: %w", id.String()[:7], err)
		}
		c, ok := obj.(*object.Commit)
		if !ok {
			return fmt.Errorf("%s is not a commit", id.String()[:7])
		}
		rawCommits[id] = c
		for _, p := range c.Parents {
			if !visited[p] {
				visited[p] = true
				queue = append(queue, p)
			}
		}
	}

	// Second pass: compute generation numbers (memoised DFS).
	genCache := make(map[types.CommitID]uint32, len(rawCommits))
	var computeGen func(id types.CommitID) uint32
	computeGen = func(id types.CommitID) uint32 {
		if g, ok := genCache[id]; ok {
			return g
		}
		c := rawCommits[id]
		if len(c.Parents) == 0 {
			genCache[id] = 0
			return 0
		}
		var max uint32
		for _, p := range c.Parents {
			if g := computeGen(p); g > max {
				max = g
			}
		}
		genCache[id] = max + 1
		return max + 1
	}
	for id := range rawCommits {
		computeGen(id)
	}

	// Build Entry slice.
	entries := make([]*Entry, 0, len(rawCommits))
	for id, c := range rawCommits {
		parents := make([]types.CommitID, len(c.Parents))
		copy(parents, c.Parents)
		entries = append(entries, &Entry{
			ID:         id,
			Generation: genCache[id],
			Timestamp:  c.Timestamp,
			Parents:    parents,
			Author:     c.Author,
			Message:    c.Message,
		})
	}

	return writeIndex(varaDir, entries)
}

// Load reads and validates graph.idx, returning the in-memory Index.
// Returns an error if the file is absent, corrupt, or has a bad checksum.
func Load(varaDir string) (*Index, error) {
	path := filepath.Join(varaDir, indexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("graphindex: load: %w", err)
	}
	return parseIndex(data)
}

// Invalidate deletes graph.idx so the next access triggers a rebuild.
// Safe to call when the file does not exist.
func Invalidate(varaDir string) {
	os.Remove(filepath.Join(varaDir, indexFile))
}

// writeIndex serialises entries and writes graph.idx atomically.
func writeIndex(varaDir string, entries []*Entry) error {
	var buf bytes.Buffer

	// Header.
	buf.WriteString(magic)
	binary.Write(&buf, binary.BigEndian, version)
	binary.Write(&buf, binary.BigEndian, uint32(len(entries)))
	binary.Write(&buf, binary.BigEndian, uint32(0)) // reserved

	// Build parent array and meta block as we go.
	var parentBuf bytes.Buffer
	var metaBuf bytes.Buffer
	parentOffset := uint32(0)
	metaOffset := uint32(0)

	type commitFixed struct {
		id           [32]byte
		generation   uint32
		timestamp    int64
		parentOffset uint32
		parentCount  uint32
		metaOffset   uint32
	}
	fixed := make([]commitFixed, len(entries))

	for i, e := range entries {
		copy(fixed[i].id[:], e.ID[:])
		fixed[i].generation = e.Generation
		fixed[i].timestamp = e.Timestamp
		fixed[i].parentOffset = parentOffset
		fixed[i].parentCount = uint32(len(e.Parents))
		fixed[i].metaOffset = metaOffset

		for _, p := range e.Parents {
			parentBuf.Write(p[:])
			parentOffset++
		}

		metaBuf.WriteString(e.Author)
		metaBuf.WriteByte(0)
		metaBuf.WriteString(e.Message)
		metaBuf.WriteByte(0)
		metaOffset = uint32(metaBuf.Len())
	}

	// Write Commit Table.
	for _, f := range fixed {
		buf.Write(f.id[:])
		binary.Write(&buf, binary.BigEndian, f.generation)
		binary.Write(&buf, binary.BigEndian, f.timestamp)
		binary.Write(&buf, binary.BigEndian, f.parentOffset)
		binary.Write(&buf, binary.BigEndian, f.parentCount)
		binary.Write(&buf, binary.BigEndian, f.metaOffset)
	}

	// Write Parent Array.
	buf.Write(parentBuf.Bytes())

	// Write Meta Block.
	buf.Write(metaBuf.Bytes())

	// Write Checksum.
	sum := sha256.Sum256(buf.Bytes())
	buf.Write(sum[:])

	// Atomic write.
	path := filepath.Join(varaDir, indexFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("graphindex: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("graphindex: rename: %w", err)
	}
	return nil
}

// parseIndex deserialises a graph.idx byte slice into an Index.
func parseIndex(data []byte) (*Index, error) {
	if len(data) < 48 { // minimum: 16-byte header + 32-byte checksum
		return nil, fmt.Errorf("graphindex: file too short")
	}

	// Validate checksum.
	body := data[:len(data)-32]
	storedSum := data[len(data)-32:]
	computed := sha256.Sum256(body)
	if !bytes.Equal(computed[:], storedSum) {
		return nil, fmt.Errorf("graphindex: checksum mismatch — index corrupt")
	}

	r := bytes.NewReader(body)

	// Header.
	var magicBuf [4]byte
	r.Read(magicBuf[:])
	if string(magicBuf[:]) != magic {
		return nil, fmt.Errorf("graphindex: bad magic")
	}
	var ver, count, reserved uint32
	binary.Read(r, binary.BigEndian, &ver)
	binary.Read(r, binary.BigEndian, &count)
	binary.Read(r, binary.BigEndian, &reserved)

	if ver != version {
		return nil, fmt.Errorf("graphindex: unsupported version %d", ver)
	}

	// Commit Table.
	type rawEntry struct {
		id           [32]byte
		generation   uint32
		timestamp    int64
		parentOffset uint32
		parentCount  uint32
		metaOffset   uint32
	}
	raws := make([]rawEntry, count)
	totalParents := uint32(0)
	for i := uint32(0); i < count; i++ {
		r.Read(raws[i].id[:])
		binary.Read(r, binary.BigEndian, &raws[i].generation)
		binary.Read(r, binary.BigEndian, &raws[i].timestamp)
		binary.Read(r, binary.BigEndian, &raws[i].parentOffset)
		binary.Read(r, binary.BigEndian, &raws[i].parentCount)
		binary.Read(r, binary.BigEndian, &raws[i].metaOffset)
		totalParents += raws[i].parentCount
	}

	// Parent Array.
	parentData := make([][32]byte, totalParents)
	for i := uint32(0); i < totalParents; i++ {
		r.Read(parentData[i][:])
	}

	// Meta Block — rest of the body.
	metaBlock := make([]byte, r.Len())
	r.Read(metaBlock)

	// Build in-memory index.
	byID := make(map[types.CommitID]*Entry, count)
	all := make([]*Entry, count)

	for i, raw := range raws {
		parents := make([]types.CommitID, raw.parentCount)
		for j := uint32(0); j < raw.parentCount; j++ {
			parents[j] = types.CommitID(parentData[raw.parentOffset+j])
		}

		author, msg := readMeta(metaBlock, raw.metaOffset)
		e := &Entry{
			ID:         types.CommitID(raw.id),
			Generation: raw.generation,
			Timestamp:  raw.timestamp,
			Parents:    parents,
			Author:     author,
			Message:    msg,
		}
		byID[e.ID] = e
		all[i] = e
	}

	return &Index{byID: byID, all: all}, nil
}

// readMeta extracts "author\x00message\x00" from the meta block at offset.
func readMeta(meta []byte, offset uint32) (author, message string) {
	if int(offset) >= len(meta) {
		return "", ""
	}
	rest := meta[offset:]
	nul1 := bytes.IndexByte(rest, 0)
	if nul1 < 0 {
		return string(rest), ""
	}
	author = string(rest[:nul1])
	rest = rest[nul1+1:]
	nul2 := bytes.IndexByte(rest, 0)
	if nul2 < 0 {
		return author, string(rest)
	}
	return author, string(rest[:nul2])
}

// collectTips resolves HEAD and all branch refs to commit IDs.
func collectTips(varaDir string, store *object.Store) ([]types.CommitID, error) {
	var tips []types.CommitID
	seen := make(map[types.CommitID]bool)

	addTip := func(hexID string) {
		hexID = strings.TrimSpace(hexID)
		if len(hexID) < 64 {
			return
		}
		base, err := types.ParseHex(hexID[:64])
		if err != nil {
			return
		}
		id := types.CommitID(base)
		if !seen[id] {
			seen[id] = true
			tips = append(tips, id)
		}
	}

	// HEAD (direct or symbolic).
	headData, err := os.ReadFile(filepath.Join(varaDir, "HEAD"))
	if err == nil {
		content := strings.TrimSpace(string(headData))
		if strings.HasPrefix(content, "ref: ") {
			refPath := strings.TrimPrefix(content, "ref: ")
			refData, err := os.ReadFile(filepath.Join(varaDir, filepath.FromSlash(refPath)))
			if err == nil {
				addTip(string(refData))
			}
		} else {
			addTip(content)
		}
	}

	// All branch refs.
	headsDir := filepath.Join(varaDir, "refs", "heads")
	entries, err := os.ReadDir(headsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(headsDir, e.Name()))
			if err == nil {
				addTip(string(data))
			}
		}
	}

	return tips, nil
}
