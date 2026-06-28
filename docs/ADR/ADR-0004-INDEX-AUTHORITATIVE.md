ADR-0004: The Index Is Authoritative for Staging
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

When building a commit, VARA must know which file versions to include. There are
two models:

1. **Working-directory authoritative**: Walk the filesystem at commit time, hash
   every file, and build the tree from whatever is on disk. The index is merely a
   cache for `status` display.
2. **Index authoritative**: The index is the explicit staging area. A commit
   records exactly what was staged via `vara add`. The working directory may
   differ from the index without those differences being committed.

# Decision

VARA uses an **index-authoritative staging model** (RFC-0005).

`vara add` stores blob IDs in the index. `vara commit` builds the tree directly
from the index — it never re-reads the working directory. The working directory
can contain un-staged changes that do not appear in a commit until explicitly
added.

# Consequences

**Good:**
- Users have deterministic control over what goes into a commit. A half-finished
  change to `foo.c` does not accidentally end up in the commit if only `bar.c` was
  staged.
- `vara commit` is O(1) in working-directory size: it only traverses the index,
  not the filesystem.
- Partial-file staging (staging a subset of changes within a file) is possible in
  principle by writing a patched blob directly to the index — no filesystem write
  required.
- The scanner's fingerprint + blob-hash fast path makes `vara status` O(N) in
  changed files rather than O(total files), because unmodified files are skipped
  after a fingerprint match.

**Bad:**
- Users must remember to `vara add` before `vara commit`; forgetting leaves
  changes out of the commit. This is a common source of user confusion.
- The index introduces a third state (working tree / index / HEAD) that tools must
  display clearly. `vara status` must diff working-tree-vs-index AND
  index-vs-HEAD to give a complete picture.
- If the index file is corrupted or deleted, staged changes are lost even if the
  working directory is intact.

# Alternatives Considered

**Mercurial / Fossil working-directory model**: Always commit what is on disk.
No staging area. Simpler mental model for beginners. Rejected because it
precludes partial commits and makes large monorepos impractical (every commit
must hash the entire working tree).

**Two-mode hybrid (git add -p)**: Default to working-directory commits with an
opt-in staging area. This would require VARA to support two commit modes with
different semantics, complicating both the implementation and the user model.
Rejected in favour of the cleaner single-model approach.
