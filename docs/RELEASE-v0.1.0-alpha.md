# VARA v0.1.0-alpha Release Notes

**Released**: 2026-06-28
**Tag**: `v0.1.0-alpha`

---

## What v0.1.0-alpha Is

This release marks the point where the **local repository engine** is
feature-complete. Every core operation — init, add, commit, branch, switch,
merge, history, undo, and verify — is implemented, specified in a governing RFC,
covered by integration tests, and measured by benchmarks.

This is an alpha: the remote protocol, the AI layer, and the pack file format are
not yet implemented. The on-disk format of the object store and index is stable;
everything else is subject to change before v1.0.

---

## Delivered in v0.1.0-alpha

### Core engine (RFC-specified, tested)

| Command | RFC | Description |
|---------|-----|-------------|
| `vara init` | RFC-0003 | Initialize repository |
| `vara add` | RFC-0005 | Stage files into index |
| `vara commit` | RFC-0007 | Create commit from staged index |
| `vara branch` | RFC-0004 | Create and list branches |
| `vara switch` | RFC-0006 | Change branches with transaction safety |
| `vara merge` | RFC-0008 | Three-way merge with conflict markers |
| `vara history` | RFC-0013 | Commit log from graph index |
| `vara undo` | RFC-0009 | Three-layer undo: journal → reflog → snapshot |
| `vara verify` | — | Eight-phase repository integrity check |
| `vara status` | RFC-0005 | Working directory vs index comparison |
| `vara log` | RFC-0007 | Commit metadata display |

### Object store (RFC-0002)

- SHA-256 content-addressed storage
- zstd compression per object
- Three object types: blob, tree, commit
- Immutable writes (0444), no modification after creation
- Object identity includes serialized type header: `SHA-256("blob\x00" + content)`

### Merge engine (RFC-0008)

- Fast-forward detection
- Three-way merge via `DiffTrees(base→ours)` + `DiffTrees(base→theirs)`
- Myers O(ND) line diff
- diff3 line-level content merge
- Conflict markers for both-modified paths
- MERGE_HEAD written for commit after resolution

### Recovery system (RFC-0009)

- **Layer 1**: Journal rollback (incomplete transactions, crash recovery)
- **Layer 2**: Reflog restore (last known good HEAD per branch)
- **Layer 3**: Snapshot archive (tar.zst, taken before every destructive operation)

### Commit Graph Index (RFC-0013)

- Binary `graph.idx` format: VAGI header + 56-byte fixed entries + parent array + meta block + SHA-256 checksum
- `LoadOrBuild`: warm path = 1 file read + in-memory BFS
- **16 ms warm history on 10,000 commits** (was 75.8 s without RFC-0013)
- Derived state: automatically rebuilt on corruption or absence

### Transactional storage (RFC-0006)

- Write-ahead journal with three states: Execute → Verify → Commit
- O_EXCL file locks (POSIX-compatible)
- Lock acquisition order: refs → index (deadlock-free by construction)
- Atomic rename for all file mutations

### Scanner (RFC-0005)

- ModTime fingerprint fast path (O(1) per file when unchanged)
- Content hash slow path: SHA-256("blob\x00" + content) verification
- Sub-resolution write guard (NTFS 100 ns timestamp)

### Repository verification

Eight independent phases: Objects, Trees, Commits, DAG, Refs, Index, Journal, Snapshots.
Each phase reports independently; one failure does not halt others.

### Fuzz testing

Five fuzz targets: `FuzzRefName`, `FuzzJournalParser`, `FuzzCommitObject`,
`FuzzTreeBlob`, `FuzzVerifyOnGarbage`. Corpus-driven; regression cases preserved.

---

## Performance

Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS:

| Command | Result | Budget |
|---------|--------|--------|
| `vara init` | 2.8 ms | 20 ms ✅ |
| `vara commit` | 15.4 ms | 200 ms ✅ |
| `vara switch` | 220 ms | 500 ms ✅ |
| `vara history` (10k commits, warm) | **16.02 ms** | 100 ms ✅ |

---

## Architecture Documentation

- `docs/ARCHITECTURE.md` — layer map, complete command flows, transaction engine, graph index design, key invariants, where to start for each type of change
- `docs/ADR/` — seven Architecture Decision Records explaining the *why* behind each major design choice
- `docs/` — ten accepted RFC specifications governing every subsystem

---

## Known Limitations

- **No remote protocol**: push, fetch, clone are not implemented
- **No pack files**: every object is a standalone zstd-compressed file (RFC-0014 deferred)
- **No AI layer**: RFC-0015+ not yet designed
- **Single-user only**: locking assumes one writer at a time; distributed writes are not safe
- **No garbage collection**: deleted branches leave orphaned objects
- **Windows NTFS only tested**: Linux and macOS untested in this alpha (CI matrix will cover these before v0.2)

---

## What v0.2 Will Focus On

1. CI results on Linux and macOS (GitHub Actions matrix: ubuntu, windows, macos × Go 1.21–1.23)
2. Coverage tracking (target: 90%+ on `pkg/object`, `pkg/refs`, `pkg/verify`)
3. Long-running stress tests (100k commits, 1M files)
4. Public API review — lock down exported surface before any remote work begins
5. RFC-0013v2: incremental graph index (append per commit, periodic compaction)
6. Memory profiling under large-repository conditions

Remote protocol design begins in v0.3.

---

## How to Try VARA

```sh
git clone https://github.com/thulasiramk-2310/vara
cd vara
go build -o vara ./cmd/vara

vara init
vara add .
vara commit -m "first commit"
vara history
vara verify
```

VARA is safe to try on disposable repositories. Do not use it for repositories
you cannot afford to lose — the remote protocol does not exist yet, so there is
no offsite backup path.

---

## Feedback

Open an issue at `github.com/thulasiramk-2310/vara/issues`.

If the repository is in a bad state, run `vara verify` first and include the
output in the bug report.
