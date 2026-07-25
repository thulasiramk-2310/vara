VARA RFC: 0023
Title: Diff Viewer
Status: Accepted
Version: 0.2.0
Authors: Thulasiram K
Created: 2026-07-25
Last Updated: 2026-07-25
Depends On: RFC-0016, RFC-0017, RFC-0018, RFC-0019, RFC-0021, RFC-0022
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0022 gave the Hub content at a *single* ref — list a tree, read a file. It
deliberately stopped short of *change*: it shows what a repository contains, not
what a commit did. This document adds exactly that — the diff — as another thin
projection over the engine: **which files changed between two points, and how each
changed, line by line**. It answers "what is the difference between *this* ref and
*that* ref, at this path?"

**The one-sentence scope:**

> RFC-0023 adds read-only diff endpoints — a `diff` summary (the changed-file list
> between a base and a head, with per-file add/delete counts), a per-file `diff`
> (unified hunks), and a commit-diff convenience (a commit against its first
> parent) — under the existing repository path, authorized by the same RFC-0018
> `read` capability. The **file level** is a projection of the engine's existing
> `diff.DiffTrees`; the **line level** is a presentational unified diff computed in
> the read layer from the two blob versions. It defines no mutation and no engine
> change.

**Design stance — projection at the file level, presentation at the line level.**

> The set of changed files between two trees *is* engine truth: RFC-0023 reads it
> straight from `diff.DiffTrees` (A/D/M + old/new blob ids), adding nothing. A
> file's *line-level* hunks, by contrast, are a display artifact — how best to
> render a change for a human — not repository state. RFC-0023 computes those in
> `internal/hub` with a small, read-only line diff that is deliberately **separate
> from the engine's merge-oriented Myers** (§11 D2): a viewer diff optimizes for
> readable, stable output; a merge diff optimizes for correct three-way
> reconstruction, and the two must never be conflated. Everything RFC-0023 serves
> is reachable from a clone the same `read` grants, and repository bytes in a diff
> are served inert (§7), exactly as in RFC-0022.

```
   GET /_vara/repositories/demo/diff?base=main~1&head=main
             │  authenticate → authorize `read` (RFC-0018, before any read)
             ▼
   resolve base/head → their trees ; diff.DiffTrees → changed files (engine)
             ▼   (per file, on demand)
   read old+new blobs → line diff → hunks  →  JSON (inert, escaped line data)
```

# 2. Motivation

A user looking at a commit in the Hub can see its message and metadata but not
what it *did* — the second most basic thing a code host does, after browsing
files. Reviewing a change, understanding a regression, reading history with intent
— all require a diff. The engine already computes the file-level change set for
every merge; RFC-0023 projects that same computation for reading, and adds the
one missing display concern, line-level hunks, in the layer where display belongs.
It needs no new capability, no engine primitive, and no wire change.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **Three-dot / merge-base diffs.** v1 compares two trees directly (a "two-dot"
  `base..head`). Comparing `head` against the *merge base* of `base` and `head`
  (git's `base...head`) is future work (§14).
* **Rename & copy detection.** The engine's `DiffTrees` reports a rename as a
  delete + add (content-addressed paths, RFC-0008). Similarity-based rename/copy
  detection is future work; the DTO reserves a `renamed` status and `old_path` for
  it (§10), unused in v1.
* **Word-level / intra-line diffs.** v1 emits line-level hunks. Highlighting the
  changed span *within* a line is a UI enhancement, deferred.
* **Any write.** RFC-0023 is strictly read-only. No patch is applied, staged, or
  committed through this API; changes are made by pushing commits (RFC-0016).
* **Syntax highlighting / rich rendering.** The API returns diff line data and a
  type; rendering is the UI's concern (§7).
* **Binary diffs.** A changed binary file is reported as changed with no hunks
  (§5.2). Delta visualization of binary content is out of scope.
* **Engine / transport changes.** RFC-0023 adds no field to `pkg/*` and no
  RFC-0016 wire message; the line diff lives in `internal/hub`, above the engine.

# 4. Terminology

* **Base / head** — the two refs being compared, each a branch name or commit id
  supplied as a query parameter. The diff is `base → head`: what head changed
  relative to base.
* **File diff** — one entry of the changed-file set: a path, a status
  (added/deleted/modified), and the old/new blob ids, from `diff.DiffTrees`.
* **Hunk** — a contiguous region of a file diff: a run of context, added, and
  deleted lines with the classic `@@ -old +new @@` coordinates.
* **Diff line** — one line of a hunk, tagged `context`, `add`, or `del`; its
  content is inert line data, never a rendered document (§7).
* **Presentational line diff** — the read-layer line diff (§11 D2), distinct from
  the engine's merge diff.

# 5. The Diff API

All endpoints live under the repository path, require `read` (§8), and take `head`
(required) and `base` (optional) as query parameters, each a branch name or commit
id. When `base` is omitted it defaults to `head`'s **first parent** (the empty tree
if `head` is a root commit), so `?head=X` alone shows what commit X did; supply
`base` to compare an arbitrary range. A missing `head` is `400`.

## 5.1 Diff summary (the changed-file set)

```
GET /_vara/repositories/{repo}/diff?head=<ref>&base=<ref>
    → 200 {base, head, base_commit, head_commit, files:[{path, old_path?, status}...], truncated}
```

Lists the files that differ between `base` and `head`, in path order — a **pure
projection of `diff.DiffTrees`** over the two commits' trees, costing one tree diff
and **no blob reads**. Each entry has a `status` (`added` / `deleted` / `modified`;
`renamed` reserved, §3). Like RFC-0022's tree listing (§5.1 there), the summary
deliberately omits per-file line counts (`additions`/`deletions`) and the `binary`
flag: both require reading each changed file's two blob versions, which would turn
the summary into O(files) diffs. They are returned with the per-file diff instead
(§5.2); a summary diffstat is deferred (§14). When the change set exceeds the
file-count cap (§7), `truncated` is set and the list is capped. An empty range
(`base` resolves equal to `head`) is a valid `200` with an empty `files` list.

## 5.2 File diff (unified hunks)

```
GET /_vara/repositories/{repo}/diff/{path...}?head=<ref>&base=<ref>
    → 200 {base, head, path, status, binary, additions, deletions, hunks:[{old_start, old_lines, new_start, new_lines, header, lines:[{type, content}...]}...], truncated}
```

Returns the line-level diff of one file that changed between `base` and `head`,
with its `additions`/`deletions` counts. For a **text** file within the caps (§7),
`hunks` holds the unified diff — each line tagged `context`, `add`, or `del`, with
inert `content`. A **binary** file (either side NUL-containing or invalid UTF-8,
§7) has `binary:true` and empty `hunks`. A text file over the caps has
`truncated:true` and empty `hunks` — the client falls back to the two `blob`/`raw`
views (RFC-0022). **The path must be part of the `base..head` change set**: a path
that is unchanged or absent in that range is `404` — there is no diff to show, and
the summary never lists it. This deliberately conflates "unchanged" and "absent";
both mean "no diff here".

## 5.3 Commit diff (convenience)

```
GET /_vara/repositories/{repo}/commits/{id}/diff
    → 200  (same body as §5.1, with base = first parent of {id}, head = {id})
```

The common case — "what did this commit do?" — without naming refs. It is exactly
`/diff?head={id}` with `base` omitted (§5): `base` defaults to the commit's **first
parent** (the same first-parent rule as RFC-0022 file history, §5.4 there), and a
**root commit** diffs against the empty tree, so every file reads as `added`. It
returns the §5.1 summary; a client then fetches per-file hunks via §5.2 using the
`base_commit`/`head_commit` the summary echoes, so it never recomputes the first
parent.

# 6. Base & Head Resolution

`base` and `head` each resolve as a branch name or a commit id (RFC-0021
resolution, as RFC-0022 §6). An unresolvable ref is `404`. Both name a **commit**,
whose **tree** is what `diff.DiffTrees` compares; the response echoes the resolved
`base_commit` / `head_commit` ids so a caller can see exactly what was compared
even when a branch name moved. The path in §5.2 is resolved within each tree the
same way RFC-0022 resolves a path — over tree *objects*, never the filesystem
(RFC-0022 B4 holds unchanged here).

# 7. Serving Diffs Safely (Normative Core)

A diff is repository bytes rearranged, so it carries the **same same-origin risk as
RFC-0022 §7** and the same mitigation:

* **Diff content is data, never a document.** Both endpoints return line content
  inside JSON strings (`lines[].content`), so the UI renders it into a diff view it
  controls — the server never hands diff bytes to the browser as a page. There is
  no `raw` diff endpoint in v1; a client wanting the raw bytes of either side uses
  RFC-0022 `raw`, which is already inert.
* **Text vs binary reuses RFC-0022's rule.** A side is text only if it is NUL-free
  **and** valid UTF-8; otherwise the file is `binary` with no hunks, so non-text
  bytes never enter a JSON string or a line diff.
* **Bounded on every axis**, mirroring RFC-0022's two-threshold pattern: a per-file
  **input ceiling** (either side larger → `413`, hard); a per-file **soft input
  cap** (either side larger → `truncated:true`, empty hunks, `200` — too big to diff
  inline, use `raw`); a per-file **hunk cap** on the produced diff (over →
  `truncated:true`, empty hunks); and a summary **file-count cap** (§5.1
  `truncated`). A binary side is excluded before any of this. Oversize *text* is
  `truncated`, never mislabeled `binary`. The line diff (Myers, O(n·d)) runs only
  within these bounds, so no request can hang it.
* **Line diff is a presentation concern in the read layer.** It never alters a blob
  id, a tree id, or any engine state; it derives display from bytes the `read`
  capability already exposes (§11 D2). The engine still stores and hashes exact
  bytes.

These rules are why the diff, like the blob, is served only as inert, escaped data
the UI renders — never as executable content on the Hub origin.

# 8. Authorization

Every diff endpoint requires the RFC-0018 **`read`** capability on the repository,
evaluated **before** any object is read — the same capability and ordering as
RFC-0021, RFC-0022, and `clone`. A diff reveals only what a clone the same `read`
grants would: it is the difference of two states the caller may already read. No
new capability is introduced (D1).

# 9. Caching

Every diff response is content-addressed and so has a natural strong `ETag`:

* a **summary** by the pair `(base tree id, head tree id)` — the changed-file set
  is a pure function of the two trees, so an unchanged pair re-validates for free;
* a **file diff** by the pair `(old blob id, new blob id)` — the hunks are a pure
  function of the two blob versions, so the same change re-validates across commits
  that share it (RFC-0022 §9).

`If-None-Match` → `304` exactly as RFC-0021/0022.

# 10. HTTP Binding

New routes (all `GET`, all require `read`):

```
GET /_vara/repositories/{repo}/diff                 ?head=<ref>&base=<ref>   (base optional)
GET /_vara/repositories/{repo}/diff/{path...}       ?head=<ref>&base=<ref>   (base optional)
GET /_vara/repositories/{repo}/commits/{id}/diff
```

The `diff/{path...}` wildcard is scoped under the literal `diff/` prefix, so the
bare `diff` summary route and the RFC-0022 `tree`/`blob`/`raw` routes are
unaffected (ServeMux most-specific-wins, as RFC-0022 §10). `commits/{id}/diff`
extends the RFC-0021 `commits/{id}` namespace additively.

DTOs (added to `internal/protocol`): `DiffFileInfo` (path, old_path?, status — no
counts, §5.1), `DiffSummaryResponse`, `DiffHunk`, `DiffLineInfo`,
`FileDiffResponse` (which carries `additions`/`deletions`/`binary`, §5.2).

Status codes:

| Situation | Status | Code |
|-----------|--------|------|
| OK | 200 | — |
| Not modified (ETag matched) | 304 | — |
| Unknown repo / ref / path, or path unchanged between base..head | 404 | `UNKNOWN_REPOSITORY` / `NOT_FOUND` |
| A file side exceeds the hard input ceiling | 413 | `TOO_LARGE` |
| `head` missing, or a malformed ref | 400 | `MALFORMED_REQUEST` |
| Unauthenticated | 401 | `UNAUTHENTICATED` |
| Lacks `read` | 403 | `UNAUTHORIZED` |

No new code: `TOO_LARGE` was introduced by RFC-0022. The Hub API version stays `1`
(X-VARA-API): these routes are additive to the v1 surface.

# 11. Architectural Constraints (Normative)

Inherits RFC-0021 H1–H9 and RFC-0022 B1–B7 (project the engine, read before act,
additive JSON, same-origin, error-schema reuse, object-graph paths, inert bytes,
bounded reads, engine unchanged). RFC-0023 adds:

* **D1 — Read is the only capability.** Every diff endpoint requires `read`
  (RFC-0018), checked before any object is read; no diff-specific capability.
* **D2 — File level projects the engine; line level is presentation.** The
  changed-file set is `diff.DiffTrees` verbatim (no reinterpretation, extends H8).
  The line diff is a **read-layer presentation routine in `internal/hub`**,
  explicitly separate from the engine's merge Myers: it may change (better hunk
  heuristics, word diff) without touching merge correctness, and merge behavior may
  change without touching the viewer. Neither imports the other. The Single
  Implementation Principle governs repository *layout and semantics* — one place
  knows the on-disk format; a presentational diff algorithm is neither, so a
  viewer-specific line diff is not a second source of truth.
* **D3 — Read-only.** RFC-0023 defines no write; repository content changes only by
  pushing commits (RFC-0016).
* **D4 — Diff bytes are inert on the app origin.** Diff line content is JSON data,
  never a rendered document; there is no raw-diff endpoint (§7). A binary side is
  never fed to the line diff or a JSON string.
* **D5 — Bounded diffs.** Per-file input cap, per-file hunk cap, and summary
  file-count cap (§7); over the hard input ceiling is `413`. One request cannot
  force an unbounded read or an unbounded line-diff.
* **D6 — First-parent commit diff.** The `commits/{id}/diff` convenience compares
  against the first parent (a root commit against the empty tree), matching
  RFC-0022 file history (§5.3).
* **D7 — Engine unchanged.** RFC-0023 adds nothing to `pkg/*` and no RFC-0016 wire
  message. `diff.DiffTrees` is *reused*; the line diff is new code above the
  engine.

# 12. Security Considerations

* **Same-origin diff content is the primary risk** and §7/D4 are its mitigation:
  diff lines are inert JSON data, there is no raw-diff document, and a binary side
  is excluded from the line diff. A repository's `.html`/`.js` can never run on the
  Hub origin through a diff, just as it cannot through a blob (RFC-0022 §12).
* **Resource bounds** (D5): the line diff is the one place RFC-0023 does real
  work (Myers is O(n·d)); the input cap bounds `n` before the algorithm runs, and
  the hunk cap bounds the output, so a pathological large-file or high-churn diff
  degrades to `truncated`/`413`, never a hang.
* **No information beyond clone** (D1): a diff is the difference of two readable
  states; it exposes nothing a clone would not.
* **Base/head are commit-resolved** (§6): a diff can only compare trees reachable
  as commits the `read` capability authorizes; the echoed `base_commit`/`head_commit`
  make the compared points explicit and non-spoofable.

# 13. Testing Strategy

* **Summary** lists exactly the changed files between base..head with correct
  `status` (added/deleted/modified) in path order, from one tree diff with **no
  blob reads** (no counts, no binary flag); an added and a deleted file are
  classified correctly; an empty range (`base` == `head`) is a `200` with an empty
  `files` list.
* **File diff** returns correct hunks + `additions`/`deletions` for a modification
  (context + add + del with right `@@` coordinates); an added file is all-`add`, a
  deleted file all-`del`; a path not in the change set (unchanged or absent) is
  `404`.
* **Commit diff (§5.3)** equals `/diff?head=id` (base omitted → first parent); a
  root commit diffs against the empty tree (all files `added`); a merge diffs
  against its first parent only (D6). `base` omitted on `/diff` defaults the same
  way.
* **Binary (D4)** — a file with a NUL side, and a NUL-free but invalid-UTF-8 side,
  is reported `binary` with empty hunks and never enters a JSON string.
* **Bounds (D5)** — a file over the hard input ceiling is `413`; a file over the
  soft input cap is `truncated` (empty hunks, `200`), not `binary`; an over-hunk-cap
  text diff is `truncated`; an over-file-count summary is `truncated`.
* **Authorization (D1)** — a caller without `read` gets 403 from every diff
  endpoint; with `read`, 200. 401-vs-403 never swapped.
* **Caching (§9)** — summary carries a (base-tree,head-tree) ETag and file diff a
  (old-blob,new-blob) ETag; `If-None-Match` → 304; the same file change across two
  commits shares a file-diff ETag.
* **Architecture (D2/D7)** — the engine diff is empty; `internal/hub` gains a
  presentational line diff but imports no binding layer and does not import the
  engine's merge path; the import test still passes.

# 14. Future Work

* **Merge-base (three-dot) diffs** — compare `head` against the merge base of
  `base` and `head` (`base...head`), the common review default.
* **Rename & copy detection** — populate the reserved `renamed` status and
  `old_path` via similarity detection, collapsing a delete+add pair.
* **Summary diffstat** — per-file `additions`/`deletions` (and `binary`) in the
  summary, deferred from v1 because they cost a blob-pair read per file (§5.1, like
  RFC-0022 tree size); a future revision could add a bounded or cached count.
* **Word-level diffs** — intra-line change spans for a richer view.
* **Shared Myers core** — a future engine revision could export the line-diff core
  so the viewer and the merge path share one implementation (today they are
  deliberately separate, D2), retiring the read-layer copy.
* **Diff of a directory / whole-tree patch download** — a single unified patch for
  a base..head range (served inert / attachment, never a document, §7).
* **Search (RFC-0024)** builds alongside these reads; a diff view links commits to
  the files they touched, which search will make discoverable.
