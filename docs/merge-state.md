# Mid-merge state model

Status: implementation note for `vara resolve` / `vara commit` / `vara merge --abort`.
Layer: **commands + `internal/mergestate`** only. It calls the frozen engine
(`pkg/graph`, `pkg/diff`, `pkg/object`, `pkg/index`) through public APIs and
adds no field to the on-disk index format. `pkg/*` and `internal/transport`
stay frozen.

## Why this document exists

VARA's index (`pkg/index.Entry`) has **one blob per path and no stage slots**.
Git tracks conflicts with stage 1/2/3 entries (base/ours/theirs); VARA does not.
The engine renders base/ours/theirs into `<<<<<<<`/`=======`/`>>>>>>>` marker
text inside the working-tree file and returns only the *list of conflicted
paths* (`internal/merge.Output.Conflicts []string`). Marker text is a **lossy
rendering**: once a strategy overwrites it, the three original sides are gone.

Two defects follow from having no structured conflict state:

1. **Data loss.** `vara resolve` inferred "conflicted" by scanning *every*
   tracked file for `<<<<<<<`. A file that merely *contains* marker text
   (documentation, a test fixture such as `internal/conflict/conflict_test.go`)
   was rewritten even though the merge never touched it.
   See `tests/integration/conflict_falsepositive_test.go`.
2. **Not recoverable.** With only a path list, `resolve --theirs` then
   `resolve --ours` is impossible (the first run destroyed the other side), and
   a base-aware `--auto` cannot be built (base content is unknown).

The fix is a content-bearing sidecar that records, per conflicted path, the
**blob IDs of all three sides**. The blobs already live in the object store, so
this costs three IDs per path and reconstructs the full conflict on demand.

## The files that constitute "merge in progress"

| File | Written by | Meaning | Cleared by |
|------|-----------|---------|-----------|
| `.vara/MERGE_HEAD` | merge engine (`pkg/recovery`) | the commit being merged in; its presence is the "a merge is in progress" flag (`recovery.MergeInProgress`) | `commit` on success, or `merge --abort` |
| `.vara/CONFLICTS` | `MergeIntoHEAD` on a `Conflict` result | JSON sidecar: `their_label`, `our_label`, and per-path `{base, ours, theirs}` blob IDs (`""` = that side lacks the path) | `commit` on success, or `merge --abort` |
| `.vara/index` | merge engine | the merged index (conflicted paths hold the marker-rendered blob; cleanly-merged paths hold merged blobs) | rewritten by `commit` / `abort` |
| working tree | merge engine | conflicted files hold marker text; cleanly-merged files hold merged content | rewritten by `resolve` / `abort` |

**HEAD does not move during a conflicted merge.** The branch ref is advanced
only on `FastForward`/`Success` (see `internal/commands/merge.go`). On a
`Conflict` result HEAD still points at the pre-merge commit, so:

> **pre-merge tracked state == the tree of the current HEAD.**

`ORIG_HEAD` is therefore unnecessary for abort in VARA and is not written; this
note records that deliberately so a future reader does not assume it exists.

## `.vara/CONFLICTS` format (v1)

```json
{
  "version": 1,
  "our_label": "main",
  "their_label": "feature",
  "entries": [
    { "path": "shared.txt",
      "kind": "content",
      "base":   { "present": true,  "blob": "<64-hex>" },
      "ours":   { "present": true,  "blob": "<64-hex>" },
      "theirs": { "present": true,  "blob": "<64-hex>" },
      "resolved": false },
    { "path": "doc.txt",
      "kind": "modify_delete",
      "base":   { "present": true,  "blob": "<64-hex>" },
      "ours":   { "present": true,  "blob": "<64-hex>" },
      "theirs": { "present": false },
      "resolved": false }
  ]
}
```

Each side is a `{present, blob}` pair so that **absent on that side**
(`present:false`) is distinct from **present but empty** (`present:true` with
`blob` = the hash of empty content) — modify/delete resolution depends on that
distinction, which three raw blob strings could not express.

`kind` is decided when the sidecar is written:
- **content** — both `ours` and `theirs` are present (edit/edit or add/add).
  Resolvable by re-rendering the three sides and choosing/combining lines.
- **modify_delete** — exactly one of `ours`/`theirs` is present. "Combine a file
  with its absence" is meaningless, so it is resolvable only by an explicit
  `--ours`/`--theirs` choice.

`resolved` is the authoritative per-path resolution flag. It is what `vara commit`
checks; marker scanning cannot stand in for it, because modify/delete and add/add
conflicts carry **no markers** in the working tree (the engine's structural
branch writes nothing), so a scan would wrongly report them resolved.

Produced in `MergeIntoHEAD`'s `Conflict` branch by recomputing
`graph.MergeBase(store, ourCommit, theirCommit)` (the same call the engine
makes) and reading each conflicted path out of the base / ours / theirs trees.
Written atomically, mode 0644, next to `MERGE_HEAD`.

`entries` never records modify/delete-free deletions; `merge_touched` (below)
captures every path the merge changed relative to pre-merge HEAD.

### `merge_touched`

```json
"merge_touched": ["a.txt", "sub/b.txt"]
```

The symmetric difference between the post-merge index and the pre-merge HEAD tree,
captured at conflict time: cleanly-merged, merge-added, and merge-deleted paths
(content-conflicted paths land here too; modify/delete ones do not, because the
engine writes nothing for them — `entries` covers those). Abort reads it to tell a
merge-authored change from one the user makes *afterward*. It must be captured at
conflict time and not recomputed from the current index, because a later
`vara resolve`/`vara add`/`vara rm` mutates the index — e.g. a user `vara rm` of an
unrelated file also makes it differ from HEAD, and inferring "involved" from the
live index would misread that as merge-authored and silently restore it.

### Frozen-engine fix: empty-base `ThreeWayMerge` (formerly a hang)

`pkg/diff.ThreeWayMerge` used to **not terminate** on an empty base: both sides
become zero-width insertion blocks at position 0 that the merge loop never
advances past. The engine avoided it only by routing add/add to its structural
branch — but an empty *base blob* (a previously-empty file modified on both sides)
would have hit it. This is now fixed at the source with a behavior-preserving
early return (identical additions merge clean; differing ones become one
whole-file conflict). It is an additive change to frozen `pkg/diff`, justified as a
proven correctness bug. Resolve calls `ThreeWayMerge` uniformly; no caller-side
empty-base workaround remains.

## Command semantics against this model

### `vara merge` (conflict re-decide, above the frozen engine)

After the engine returns a `Conflict`, the command layer **re-decides every flagged
conflict** from the stored base/ours/theirs blobs using the base-aware,
order-independent renderer (`internal/conflict.Render`), in `refineConflicts`
(`internal/commands/merge.go`). For each content path it re-renders the working tree
and index:
- **fully auto-resolves** (no overlap — e.g. disjoint edits the engine over-reports,
  or an order-dependent adjacent-edit conflict): written clean, **not** recorded as a
  conflict;
- **still conflicts**: written in the configured marker style and recorded unresolved;
- **binary** (any side): never line-merged — ours is kept and the path recorded
  unresolved for an explicit `--ours`/`--theirs`.

modify/delete paths are carried through structurally. If **nothing** remains
unresolved, the merge is **finalized as a normal two-parent commit** (clearing the
engine's MERGE_HEAD) via the shared `concludeMerge` — so a disjoint-adjacent merge
"just works" regardless of which branch is current, fixing the engine's Myers
order-dependence without touching `pkg/diff` or `internal/merge`. On an unexpected
error it falls back to the engine's markers + record-only sidecar.

**Marker style** (`internal/conflict.MarkerStyle`) is resolved by
`configuredMarkerStyle`: repo config `merge.conflictStyle` (`merge`|`diff3`|`zdiff3`)
if set, else **zdiff3** (default). `zdiff3` shows the `||||||| base` section with
lines common to both sides hoisted out of the markers as context; `diff3` shows the
base section without hoisting; `merge` is classic 2-way. `vara resolve` takes
`--merge`/`--diff3`/`--zdiff3` to override per-invocation.

### `vara resolve`
- The **authoritative set** of conflicted paths is the sidecar's entries — never
  a worktree marker scan. Non-conflicted files are never touched (fixes defect 1).
- **Content** entries reconstruct the conflict from the stored blobs each run, so
  resolve is **idempotent and re-runnable** (fixes defect 2):
  - `--ours` / `--theirs` → write the stored side's blob content verbatim.
  - `--auto` → **base-aware three-way merge** (`internal/conflict.MergeThreeWay`)
    of the three stored blobs. It settles every region only one side changed —
    including edits that touch adjacent-but-disjoint base lines, which the engine's
    coarser (and, on adjacent edits, ours/theirs-order-dependent) merge reports as
    a single conflict — and leaves genuine same-region disagreements, plus
    hunk-level modify/delete (one side empties a region the other edits), as
    markers rather than guessing a side. `Remaining == 0` ⟺ the result is
    marker-free ⟺ the entry is marked resolved.
  - `--union` → re-render the engine's conflict (`diff.ThreeWayMerge`) and keep
    both sides of each hunk via `internal/conflict.Resolve`.
  - **binary** content: `--auto`/`--union` refuse (need an explicit choice);
    `--ours`/`--theirs` write the whole side verbatim.
  - Because every render/merge comes from the stored blobs, not the worktree bytes,
    changing your mind and re-running a different strategy always works.
- **Modify/delete** entries: `--ours`/`--theirs` materialize the chosen side —
  writing its blob, or **deleting the file** (index entry → StateDeleted) when the
  chosen side deleted it, never inventing an empty file. `--union`/`--auto`
  **refuse** and report that an explicit choice is required.
- A settled entry's `resolved` flag is set and the sidecar rewritten; the sidecar
  stays in place (entries retained, flags updated) until commit or abort.
- `vara add` of a conflicted path also sets its `resolved` flag — the Git-style
  "add means resolved" — but only when the staged content is **marker-free**, so a
  half-edited conflict can never be marked resolved.
- Legacy fallback: if `MERGE_HEAD` exists but no sidecar (a merge begun by an
  older binary), fall back to the old marker scan **restricted to files that
  actually contain markers**, preserving backward behavior without widening it.

### `vara commit`
- Refuses while any sidecar entry is **unresolved** (`unresolvedConflicts` returns
  `State.UnresolvedPaths()`). The `resolved` flag — not a marker scan — is the
  gate, so marker-less modify/delete and add/add conflicts are correctly blocked.
- Adds `MERGE_HEAD` as the second parent → two-parent merge commit.
- On success clears **both** `MERGE_HEAD` and `.vara/CONFLICTS`.

### `vara merge --continue`

The Git-parity way to conclude a resolved merge, alongside `--abort`
(`internal/commands/mergeconclude.go`). Errors if no merge is in progress, or
(naming them) if any sidecar entry is still unresolved — the same
`unresolvedConflicts` gate `commit` uses. Otherwise it concludes via `RunCommit`
with the given `-m` message or the default `Merge branch '<theirLabel>'`.

### `vara diff`

Shows a unified diff (`internal/conflict.FormatUnified`, built on the same LCS
change blocks, reusing the base-aware machinery — no engine change): working tree vs
index by default, index vs HEAD with `--staged`/`--cached`. **Conflict-aware:** while
a merge is in progress it first surfaces the sidecar's unmerged paths as an
ours-vs-theirs diff (a plain diff of a marker-laden file is noise), then diffs the
rest.

### `vara status`
Overlays the sidecar on the ordinary working-tree scan (the scanner has no
conflict concept, and marker-less modify/delete and add/add conflicts leave the
worktree looking merely modified or clean, so the scan alone cannot surface them).
When a merge is in progress it prints a banner — "You have unmerged paths." while
any entry is unresolved, or "All conflicts fixed but you are still merging." once
all are — then two sidecar-driven sections:
- **Unmerged paths** — each unresolved entry, classified by kind into a git-style
  label/short code: content with a base → `both modified`/`UU`; content with no
  base (add/add) → `both added`/`AA`; modify/delete → `deleted by them`/`UD` (ours
  present) or `deleted by us`/`DU` (theirs present).
- **Resolved (staged for the merge commit)** — entries whose `resolved` flag is set.

Merge-owned paths are removed from the ordinary Modified/Deleted/Staged/Untracked
buckets so each is reported once. Legacy fallback: `MERGE_HEAD` but no sidecar
lists marker-bearing tracked files as `both modified` (the same restricted scan
`vara resolve` falls back to). No engine or scanner change — the overlay is built
in the command layer and rendered by `internal/status`.

### `vara merge --abort`
Restores the repository to the pre-merge state.

**Invariant:** after a successful abort, every tracked file is byte-identical to
its content at the current HEAD (== pre-merge HEAD), the index matches HEAD's
tree, and neither `MERGE_HEAD` nor `.vara/CONFLICTS` remains.

**Clobber guard (must not silently discard user work).** "Merge-involved" is read
from the sidecar — a recorded conflict (`entries`) or a `merge_touched` path —
never inferred from the current index. For every path in HEAD ∪ current-index that
is **not** merge-involved, abort refuses (naming it) when the user changed it after
the conflict:
- present in HEAD, **absent on disk** → user deleted it (restore would resurrect it);
- present in HEAD, **on disk but differing** → user edited it;
- **not in HEAD, present on disk** → a new file the user added (restore would delete it).

`vara rm` of an unrelated file is exactly the first case; because involvement comes
from the sidecar and not the live index, the guard sees it. Untracked files absent
from both HEAD and the index are never touched, matching Git's `merge --abort`.

### `vara rm` and deletions

`vara add` never stages a deletion, and there was no other way to record one, which
left the deletion side of a modify/delete conflict unreachable from the CLI.
`vara rm <pathspec>...` removes tracked files and stages the deletion (index entry →
StateDeleted). During a merge, removing a conflicted path records the deletion as
that path's resolution (sets its `resolved` flag) — the hand-resolution exit for
modify/delete, alongside `vara resolve --ours/--theirs`.

**Steps** (inside a Refs+Index transaction, snapshot first):
1. Error if `!recovery.MergeInProgress` → "no merge in progress".
2. Build the HEAD tree's `path → blobID` map; build the current index's map.
3. Run the clobber guard; on any hit, refuse and return the names.
4. Check out the HEAD tree into the working directory (reusing `checkoutTree`),
   removing tracked files absent from HEAD, and write the HEAD-shaped index.
5. Remove `MERGE_HEAD` and `.vara/CONFLICTS`.
6. Commit the transaction.
```
