// Package gc reclaims unreferenced objects (RFC-0014 §12). Interrupted transfers
// (clone/fetch/push) can leave objects in the store that no reference points at;
// these are inert but consume space. gc sweeps them.
//
// RFC:
// VARA-RFC-0014 Remote Protocol (§10 atomicity leaves inert objects, §12 gc)
//
// Safety: the reachable set is the object closure of every root — all
// references, HEAD, and every commit named in HEAD's reflog — so objects that
// `vara undo` might restore are preserved. Anything outside that closure is
// provably unreferenced and safe to delete.
package gc

import (
	"os"
	"path/filepath"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/transfer"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Result summarizes a gc run.
type Result struct {
	Scanned    int   // loose objects found on disk
	Reachable  int   // objects reachable from the roots
	Removed    int   // objects deleted (0 on a dry run)
	BytesFreed int64 // bytes reclaimed (estimated on a dry run)
}

// Collect finds unreferenced objects and, if apply is true, removes them.
// With apply=false it performs a dry run: nothing is deleted, but Removed and
// BytesFreed report what would be.
func Collect(varaDir string, apply bool) (Result, error) {
	var res Result
	store := object.NewStore(varaDir)

	// 1. Reachable closure from all roots.
	roots := gatherRoots(varaDir, store)
	reachable := map[types.ObjectID]bool{}
	if len(roots) > 0 {
		ids, err := transfer.Enumerate(store, roots, nil)
		if err != nil {
			return res, err
		}
		for _, id := range ids {
			reachable[id] = true
		}
	}
	res.Reachable = len(reachable)

	// 2. Every loose object on disk.
	objs, err := listLooseObjects(varaDir)
	if err != nil {
		return res, err
	}
	res.Scanned = len(objs)

	// 3. Anything not reachable is garbage.
	for _, o := range objs {
		if reachable[o.id] {
			continue
		}
		res.Removed++
		res.BytesFreed += o.size
		if apply {
			if err := os.Remove(o.path); err != nil && !os.IsNotExist(err) {
				return res, err
			}
		}
	}
	return res, nil
}

// gatherRoots collects every commit a live repository could need: all refs,
// HEAD, and the commits recorded in HEAD's reflog. Only IDs that resolve to a
// readable commit are included, so a dangling ref never aborts gc.
func gatherRoots(varaDir string, store *object.Store) []types.CommitID {
	seen := map[types.CommitID]bool{}
	var roots []types.CommitID
	add := func(c types.CommitID) {
		if (c == types.CommitID{}) || seen[c] {
			return
		}
		obj, err := store.Read(types.ObjectID(c))
		if err != nil {
			return
		}
		if _, ok := obj.(*object.Commit); ok {
			seen[c] = true
			roots = append(roots, c)
		}
	}

	r := refs.NewFSResolver(varaDir)
	if list, err := r.List(); err == nil {
		for _, ref := range list {
			add(ref.ObjectID)
		}
	}
	if head, err := r.Resolve("HEAD"); err == nil {
		add(head)
	}
	if entries, err := reflog.NewManager(varaDir).Read("HEAD"); err == nil {
		for _, e := range entries {
			add(e.OldID)
			add(e.NewID)
		}
	}
	return roots
}

type looseObject struct {
	id   types.ObjectID
	path string
	size int64
}

// listLooseObjects enumerates every object file on disk. Objects live at
// .vara/<2-hex>/<62-hex>; the store shares the .vara root with refs/, logs/,
// etc., but those directory names are never two hex characters, so a two-hex
// shard directory unambiguously holds objects.
func listLooseObjects(varaDir string) ([]looseObject, error) {
	entries, err := os.ReadDir(varaDir)
	if err != nil {
		return nil, err
	}
	var out []looseObject
	for _, e := range entries {
		if !e.IsDir() || !isHex(e.Name(), 2) {
			continue
		}
		shard := e.Name()
		files, err := os.ReadDir(filepath.Join(varaDir, shard))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !isHex(f.Name(), 62) {
				continue // skip .tmp files and anything malformed
			}
			id, err := types.ParseHex(shard + f.Name())
			if err != nil {
				continue
			}
			path := filepath.Join(varaDir, shard, f.Name())
			var size int64
			if info, err := f.Info(); err == nil {
				size = info.Size()
			}
			out = append(out, looseObject{id: types.ObjectID(id), path: path, size: size})
		}
	}
	return out, nil
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
