# VARA Architecture

This document is the map of the VARA engine. Read it before diving into the RFCs
or the source code. It explains how the layers fit together and traces the exact
path a request takes through the system.

---

## Layer Map

```
┌─────────────────────────────────────────────────┐
│                  cmd/vara                        │  Argument parsing only.
│           vara init / add / commit / …           │  No business logic.
└───────────────────┬─────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────┐
│             internal/commands                    │  RFC-0012
│   RunInit  RunAdd  RunCommit  RunSwitch  …       │  One function per command.
│   RunMerge  RunUndo  RunVerify  RunHistory        │  Coordinates all layers below.
└──────────┬────────────────────────┬─────────────┘
           │                        │
┌──────────▼────────┐  ┌────────────▼──────────────┐
│internal/transaction│  │   internal/locking         │  RFC-0006
│  Write-ahead       │  │  O_EXCL file locks         │
│  journal           │  │  Acquisition order:        │
│  StateExecute      │  │  refs → index (always)     │
│  StateVerify       │  │                            │
│  StateCommit       │  └────────────────────────────┘
└──────────┬─────────┘
           │
┌──────────▼──────────────────────────────────────┐
│             internal/repository                  │  RFC-0003
│   Repository.Init() — creates .vara/ layout      │
│   Locates VaraDir from working path              │
└──────────┬──────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────┐
│          pkg/refs   pkg/reflog                   │  RFC-0004
│   FSResolver — resolve / update branch refs       │
│   ValidateName — RFC-0004 §3 name rules          │
│   ReflogManager — append-only HEAD log           │
└──────────┬──────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────┐
│     pkg/graph     pkg/graphindex                 │  RFC-0007, RFC-0013
│   MergeBase — BFS least-common-ancestor          │
│   graphindex.LoadOrBuild — binary graph.idx      │
│   16 ms history on 10k commits (warm path)       │
└──────────┬──────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────┐
│          pkg/index   pkg/scanner                 │  RFC-0005
│   Index — in-memory staging area                 │
│   Scanner — fingerprint + blob-hash detection    │
│   Fingerprint (ModTime) fast path               │
│   Blob hash slow path: SHA-256("blob\0"+content) │
└──────────┬──────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────┐
│              pkg/object                          │  RFC-0002
│   Blob / Tree / Commit                           │
│   Store.Write — serialize → hash → compress      │
│   Store.Read  — decompress → parse → verify      │
│   Objects are immutable after creation (0444)    │
└──────────┬──────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────┐
│         pkg/hash   pkg/compression               │
│   SHA-256 (RFC-0002)   zstd (RFC-0002)           │
└─────────────────────────────────────────────────┘
```

Supporting packages (used by multiple layers):

```
pkg/diff       Myers O(ND) diff, diff3 three-way merge    RFC-0008
pkg/builder    BuildTree, BuildCommit                      RFC-0007
pkg/snapshot   tar.zst working-directory archive           RFC-0009
pkg/recovery   Read-only journal/reflog/snapshot scan      RFC-0009
internal/undo  Three-layer undo decision tree              RFC-0009
pkg/verify     Repository integrity report                 —
pkg/ignore     .varaignore parsing and matching            —
internal/merge Three-way merge engine                      RFC-0008
```

---

## Object Identity

Every object in VARA is identified by:

```
object_id = SHA-256(serialize(object))
```

where `serialize` prepends a type header:

```
"blob\x00"   + raw_content    → blob object
"tree\x00"   + encoded_entries → tree object
"commit\x00" + encoded_fields  → commit object
```

This means two files with identical bytes produce different IDs if they are
stored as different types (ADR-0007). Any code that computes a blob ID from file
content MUST use `SHA-256("blob\x00" + content)`, not just `SHA-256(content)`.

Objects are stored at:

```
.vara/<first-2-hex-chars>/<remaining-62-hex-chars>
```

written at mode 0444 (read-only), and never modified after creation.

---

## Flow: `vara commit`

```
RunCommit(ctx, message)
    │
    ├─ builder.BuildTree(ctx.Index, store)
    │       │
    │       ├─ for each entry in index:
    │       │     store.Write(blob) ← blob already exists (idempotent)
    │       └─ store.Write(tree)
    │
    ├─ builder.BuildCommit(store, treeID, parents, author, message)
    │       └─ store.Write(commit)
    │
    ├─ Update HEAD / branch ref  ← atomic write (tmp → rename)
    │
    ├─ ctx.Index.Entries → State = StateUnmodified
    │
    ├─ Write index to .vara/index  ← atomic write
    │
    └─ graphindex.Invalidate(varaDir)  ← delete graph.idx
             next RunHistory call rebuilds it
```

Key invariant: **the index is authoritative**. `BuildTree` reads only the index,
never the working directory. A file not in the index does not appear in the commit.

---

## Flow: `vara switch <branch>`

```
RunSwitch(ctx, targetBranch)
    │
    ├─ refs.ValidateName(targetBranch)
    │
    ├─ resolver.Resolve("refs/heads/"+targetBranch) → targetCommitID
    │
    ├─ snapshot.Create(varaDir, rootDir, "switch", currentCommitHex)
    │       └─ writes .vara/snapshots/snap-<ts>-switch-<commit7>.tar.zst
    │           (safety net, excluded from transaction)
    │
    ├─ transaction.Begin(varaDir, "switch", NameRefs, NameIndex)
    │       └─ acquires locks in order: refs → index  (deadlock-free)
    │
    ├─ txn.SetState(StateExecute)  ← journal: operation in progress
    │
    ├─ checkoutTree(store, targetCommit.TreeHash, rootDir, newIdx)
    │       └─ for each blob in target tree:
    │             read blob → write file to disk → record in newIdx
    │             (ModTime recorded as fingerprint)
    │
    ├─ remove stale files (in oldPaths but not in newPaths)
    │
    ├─ ctx.Index = newIdx  ← update in-memory state
    │
    ├─ write .vara/index  ← atomic write
    │
    ├─ txn.SetState(StateVerify)
    │
    ├─ write .vara/HEAD  ← atomic: "ref: refs/heads/<branch>\n"
    │
    ├─ txn.SetState(StateCommit)
    │
    ├─ txn.Commit()  ← release locks, mark journal complete
    │
    └─ return "Switched to branch '<branch>'\n"
```

The snapshot is taken BEFORE the transaction. If the transaction succeeds, the
snapshot is an unnecessary archive. If it fails mid-checkout, the snapshot lets
`vara undo` restore the pre-switch state (ADR-0002).

---

## Flow: `vara merge <branch>`

```
RunMerge(ctx, branchName)
    │
    ├─ resolve ourCommitID (HEAD) and theirCommitID
    │
    ├─ snapshot.Create(...)  ← safety net
    │
    ├─ transaction.Begin(...)
    │
    ├─ graph.MergeBase(store, ourCommitID, theirCommitID)
    │       └─ BFS from OUR ancestors, then BFS from THEIR tip
    │           first common ancestor = merge base
    │
    ├─ [base == ours] → FastForward
    │       checkoutTree(theirTree)  →  advance ref
    │
    ├─ [base == theirs] → Already up-to-date, no-op
    │
    └─ ThreeWay(in, base)
            │
            ├─ diff.DiffTrees(base→ours)   → ourChanges
            ├─ diff.DiffTrees(base→theirs) → theirChanges
            │
            └─ for each changed path:
                  ├─ only ours changed  → applyChange(ours)
                  ├─ only theirs changed → applyChange(theirs)
                  ├─ both made same change → applyChange(ours)
                  ├─ both modified (different) →
                  │     diff.ThreeWayMerge(base, ours, theirs)
                  │     Myers diff + diff3 → merge or conflict markers
                  └─ structural conflict → add to conflicts list

            [conflicts > 0] → write MERGE_HEAD, return Conflict
            [no conflicts]  → build merge commit, advance ref
```

---

## Flow: `vara verify`

`vara verify` scans the repository in eight ordered phases. Each phase can report
errors independently; a failure in one phase does not stop subsequent phases.

```
verify.Verify(varaDir)
    │
    ├─ Phase 1: Objects
    │     scan .vara/<2hex>/<62hex> files
    │     for each: decompress → recompute SHA-256 → compare to filename
    │
    ├─ Phase 2: Trees
    │     for each tree object: verify all entry hashes exist in object store
    │
    ├─ Phase 3: Commits
    │     for each commit object: verify TreeHash exists, verify each Parent exists
    │
    ├─ Phase 4: DAG
    │     DFS from all commit tips, white/grey/black coloring
    │     grey node revisited = cycle (should be impossible in correct repo)
    │
    ├─ Phase 5: Refs
    │     for each file in .vara/refs/: verify it parses as a valid commit ID
    │
    ├─ Phase 6: Index
    │     for each index entry: verify the BlobID exists in object store
    │
    ├─ Phase 7: Journal
    │     scan .vara/journal/txn-*.json
    │     flag any transaction stuck in StateExecute (incomplete, needs recovery)
    │
    └─ Phase 8: Snapshots
          list .vara/snapshots/ entries
          report count and sizes (no content verification)
```

---

## Transaction Engine

Every mutating command uses the same pattern (RFC-0006):

```
txn = transaction.Begin(varaDir, opName, lock1, lock2, ...)
│
│   Locks are acquired in alphabetical order by name.
│   This guarantees no deadlock: two concurrent operations
│   that both need "index" and "refs" will both try "index"
│   first; one blocks, one proceeds.
│
txn.SetState(StateExecute)      ← journal: "started, may be incomplete"
│
│   ... perform mutations using atomic writes (tmp → rename) ...
│
txn.SetState(StateVerify)       ← journal: "mutations done, verifying"
│
│   ... update refs ...
│
txn.SetState(StateCommit)       ← journal: "about to release locks"
│
txn.Commit()                    ← releases locks, journal entry becomes complete
│
defer txn.Rollback()            ← no-op after Commit(); rolls back if panicked
```

On crash recovery, `pkg/recovery.ScanJournal` reads all `txn-*.json` files:
- `StateExecute`: incomplete mutation — the ref was never updated. Repository is
  consistent at the previous state. No action needed.
- `StateVerify`: mutations completed but ref update may be partial. A forward pass
  can finish the rename; or the operation can be retried safely (idempotent writes).
- `StateCommit`: operation completed. Nothing to do.

---

## Commit Graph Index (RFC-0013)

The commit graph index is the answer to: "why is `vara history` 16 ms instead of
76 seconds on 10,000 commits?"

```
Without RFC-0013:
    RunHistory → for each commit: store.Read() → decompress → parse
    Cost: O(N) disk reads, each ~2.9 ms on NTFS = 29 s for 10k commits

With RFC-0013 (warm path):
    RunHistory → graphindex.Load(graph.idx) → one file read → in-memory BFS
    Cost: 1 file read (~1.5 MB) + O(N) in-memory operations = 16 ms for 10k commits

Cold path (graph.idx absent):
    RunHistory → graphindex.Build(store) → O(N) reads → write graph.idx → Load
    Cost: same as without RFC-0013, paid once per invalidation cycle
```

`graph.idx` is **derived state** (ADR-0006). It is deleted after every `commit`
or `merge` and rebuilt on next access. It cannot corrupt a repository: if it is
absent or has a bad checksum, `LoadOrBuild` rebuilds it silently.

---

## Key Invariants

These invariants must be preserved by any change to the engine:

1. **Object immutability**: once written at 0444, an object is never modified or
   deleted. Object IDs are stable forever.

2. **Index authority**: `vara commit` reads only the index, never the working
   directory. What is staged is what is committed.

3. **Atomic writes**: every file mutation uses `write-to-tmp → rename`. A partial
   write is never visible to readers.

4. **Lock ordering**: refs lock is always acquired before index lock. No exception.

5. **Graph index is derived**: `graph.idx` is always rebuildable. Code must call
   `LoadOrBuild`, never `Load` alone.

6. **Object identity includes header**: blob IDs are `SHA-256("blob\x00"+content)`.
   Any code computing a blob ID must replicate this exactly.

---

## Where to Start

| Task | Start here |
|------|-----------|
| Add a new command | `internal/commands/`, follow existing pattern |
| Change merge behaviour | `internal/merge/merge.go`, `pkg/diff/` |
| Change object format | `pkg/object/`, update RFC-0002 |
| Add a new ref type | `pkg/refs/`, update RFC-0004 |
| Improve history performance | `pkg/graphindex/`, update RFC-0013 |
| Add a new integrity check | `pkg/verify/verify.go` |
| Add recovery logic | `internal/undo/undo.go`, `pkg/recovery/` |
| Change the index format | `pkg/index/`, update RFC-0005 |

For anything that changes the on-disk format or command behaviour, write or update
the governing RFC before writing code.
