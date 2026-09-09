VARA RFC: 0025
Title: Structural Merge & First-Class Conflicts
Status: Draft
Version: 0.1.0
Authors: Thulasiram K
Created: 2026-09-09
Last Updated: 2026-09-09
Depends On: RFC-0005, RFC-0006, RFC-0008
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

VARA's merge is line-based (RFC-0008): three-way diff3 over lines, with conflicts
recorded in the content sidecar (`.vara/CONFLICTS`) and resolved by `vara resolve`.
Line merging cannot see the *structure* of a file, so two logically independent
edits to the same structured document (e.g. two different keys in one JSON file)
are reported as a conflict even though they compose cleanly.

This RFC adds **structural (semantic) merge** — a pluggable per-type merge driver
that merges parsed structure instead of raw lines — and lays out **first-class
conflicts**, a model in which a conflicted state can be carried and resolved later
rather than blocking every subsequent operation.

It is an **additive layer above the frozen engine**: `pkg/diff` and the on-disk
object/index formats are unchanged. The default behavior for every file type is
exactly today's line merge; structural merge only engages for registered types.

# 2. Scope & Non-Goals

**In scope (Phase 1):**
- A merge-driver abstraction selected by file type, with the current line/diff3
  merge as the default driver.
- Structural drivers for **JSON, YAML, and TOML** (sharing one node-level
  three-way merge core; other structured types plug into the same framework).
- Deterministic, reproducible output and a safe fallback to line merge.

**Per-driver conflict rendering:** JSON and YAML render genuine same-key clashes
as per-key markers (only the diverging key is wrapped). TOML currently renders
only the clean merge; a TOML conflict falls back to the line merge (per-key TOML
markers need table-header rendering — deferred). TOML adds one dependency
(`github.com/pelletier/go-toml/v2`, pure-Go, no CGO).

**YAML note:** decoding to a plain tree drops comments, so a clean structural
YAML merge does not preserve them (see §12); the driver is opt-in per repo and
falls back to the line merge on any parse failure or genuine conflict, so it only
reformats a file it actually merges. YAML requires one dependency
(`gopkg.in/yaml.v3`, pure-Go, no CGO).

**In scope (Phase 2, specified but deferred):**
- **First-class conflicts**: committing a conflicted state and resolving later.
- Additional structured drivers (YAML, TOML).

**Non-goals:**
- Changing `pkg/diff`, the object format (RFC-0002), or the index format (RFC-0005).
- Language-aware / AST merge of source code (a possible future driver, not here).
- LLM-assisted resolution — that is RFC-0011's domain and layers on top of this.

# 3. Architecture

Structural merge is realized in a new package `internal/smartmerge`, above the
frozen engine, and is invoked from the same command-layer seam that already
re-decides conflicts (`internal/commands` merge/resolve). It never replaces the
engine; it chooses, per path, *which driver* renders the merge.

```
merge/resolve (internal/commands)
        │  per conflicted path
        ▼
  smartmerge.Select(path, cfg)  ──▶ driver
        │                             ├── line     (default; internal/conflict, diff3/zdiff3)
        │                             └── json     (structural; this RFC)
        ▼
  driver.Merge(base, ours, theirs) ─▶ (mergedBytes, []Conflict)
```

The line driver is the existing `internal/conflict` path (RFC-0008 semantics,
unchanged). A structural driver is tried only for registered types and **falls
back to the line driver** whenever any side fails to parse, so a malformed
structured file can never lose data or block on a parser bug.

# 4. Driver Selection

Selection is by file extension, overridable through repository config (RFC-0010):

```
[merge]
  # driver.<ext> = <driver-name>   (default table below)
  driver.json = structured-json
  driver.yaml = line              # example override: force line merge
```

Resolution order: explicit config entry → built-in default table → `line`. An
unknown or disabled driver resolves to `line`. This guarantees no file type
silently changes behavior without either a shipped default or an explicit opt-in.

# 5. Structural Merge Algorithm (JSON, Phase 1)

Given `base`, `ours`, `theirs` byte streams for one path:

1. **Parse** all three into a document tree. Any parse error on any side →
   fall back to the line driver for this path.
2. **Three-way tree merge**, applied recursively:
   - **Objects (maps):** merge key by key.
     - key changed on only one side → take that side's value.
     - key changed on both sides to the *same* value → take it.
     - key added on only one side → include it.
     - key deleted on one side, unchanged on the other → delete it.
     - key changed on both sides to *different* values, or added on both to
       different values → **conflict at that key** (recurse first; only leaves
       that truly diverge conflict).
     - key modified on one side and deleted on the other → **conflict** (a
       structural modify/delete, mirroring RFC-0008 §5 semantics).
   - **Arrays:** Phase 1 treats a changed array as an ordered scalar — if both
     sides changed it differently, it is one conflict on that array. (Element-wise
     array merging is an open question, §12.)
   - **Scalars:** equal → take; differ → conflict.
3. **Serialize** the merged tree canonically (stable key order, fixed indentation)
   so the output is byte-reproducible regardless of `ours`/`theirs` order — the
   same order-independence guarantee the line merger already provides.

**Formatting trade-off.** Structural merge reserializes the document, so
whitespace/key-order in the working file may change even where content did not.
This is acceptable for machine-owned files (lockfiles, manifests) and is the
reason structural merge is **opt-in per type**, defaulting on only for types
where canonical form is expected. Flagged as a decision in §12.

# 6. Conflict Representation

Structural conflicts reuse the existing sidecar (`internal/mergestate`), so
`commit`'s resolved-flag gate, `status`, `abort`, and `resolve` all keep working
unchanged. Two additions:

- The sidecar entry gains an optional `driver` field (`line` | `structured-json`)
  and, for structural conflicts, a list of **conflict locations** (JSON Pointer
  paths, e.g. `/build/target`) so `status`/`resolve` can point at the exact key.
- The working-tree rendering of a structural conflict is the merged document with
  a conflict block *only around the diverging value*, marked with the same
  `<<<<<<< / ||||||| / ======= / >>>>>>>` family so existing tooling, the commit
  gate, and `vara resolve` continue to recognize it. **Implemented for JSON and
  YAML** (clean keys stay merged; only the diverging key is wrapped, with a `base`
  section and an empty side for a deleted key). Both `vara merge` and
  `vara resolve --auto` produce it. For YAML, only the map "spine" leading to a
  conflict is hand-emitted; every clean subtree and scalar is rendered by
  `yaml.Marshal`, so quoting/nesting stay correct (comments are still dropped,
  §12). `resolve --ours/--theirs` writes the whole chosen side (blob-based).

No new on-disk *object* format; the sidecar is command-layer state (RFC-0008
follow-up), so this remains outside the freeze.

# 7. First-Class Conflicts (Phase 2 — specified, deferred)

Today `vara commit` refuses while any sidecar entry is unresolved. Phase 2 makes a
conflicted state a *committable, carryable* value:

- `vara commit --allow-conflicts` records a commit whose sidecar conflicts are
  preserved as commit-associated state, marking the commit "conflicted."
- Later, `vara resolve` on a conflicted commit produces a follow-up resolving
  commit; `status`/`log` surface the conflicted marker.
- Default behavior is unchanged (commit still refuses unresolved conflicts unless
  the flag is given), so this is strictly opt-in and backward compatible.

Detailed semantics (how conflicted state travels through `push`/`pull`, whether it
lives in the commit object or a parallel ref) are deferred to a Phase 2 revision
of this RFC, because the commit object format is frozen (RFC-0002) and any change
there needs its own justification.

# 8. CLI Surface

No new top-level commands in Phase 1. `vara merge`, `vara resolve`, `vara status`,
and `vara diff` behave as today; structural merge simply produces fewer, more
precise conflicts for registered types. New config keys under `[merge]` (§4).
Phase 2 adds the `--allow-conflicts` flag to `commit`.

# 9. Freeze & Compatibility

- `pkg/*` and `internal/transport` are **untouched**. New code lives in
  `internal/smartmerge` and small call-site additions in `internal/commands`.
- The **default for every existing file type is line merge**, so repositories and
  existing behavior are unaffected until a structural driver is enabled for a type.
- Fallback-to-line on parse failure guarantees no regression for malformed inputs.

# 10. Testing

- Unit: JSON three-way merge across the §5 cases (independent keys, same-key
  divergence, add/add, modify/delete, nested objects, arrays, malformed→fallback,
  order-independence: `merge(b,o,t) == merge(b,t,o)` modulo side labels).
- Integration: a real `vara merge` of a JSON file with independent-key edits
  **auto-resolves** where the line merger would have conflicted; a same-key clash
  still conflicts, gates commit, and resolves per-key.
- Golden: canonical serialization output pinned.

# 11. Phasing

- **Phase 1:** JSON structural driver + selection + sidecar `driver`/locations +
  fallback. (This RFC's committable scope.)
- **Phase 2:** first-class conflicts; YAML/TOML drivers.

# 12. Open Questions

1. **Array merging.** Element-wise (LCS on array items) vs whole-array-as-scalar.
   Phase 1 uses whole-array; element-wise is a candidate for a point release.
2. **Formatting preservation.** Canonical reserialize (simple, reproducible) vs
   minimal-edit rewrite (preserves original formatting, much harder). Phase 1
   chooses canonical for machine-owned files.
3. **Comment-bearing formats** (YAML/TOML): where do comments attach on a merge?
   Deferred with the YAML driver.

# 13. Prior Art (neutral)

Structural/semantic three-way merge and "first-class / carryable conflict" models
have been explored across the version-control and program-analysis literature;
this RFC adapts those techniques to VARA's frozen-engine, sidecar-based
architecture. No competitive analysis is part of this document.
