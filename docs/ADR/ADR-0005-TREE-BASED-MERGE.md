ADR-0005: Tree-Based Merge Instead of Patch-Based Merge
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

The three-way merge algorithm (RFC-0008) must determine, for each file, what
changed on each side and how to combine those changes. Two approaches exist:

1. **Patch-based merge**: Record commits as diffs (patches) against their parent.
   A merge replays patches on top of the base. Used by systems like Darcs and the
   `git apply` pathway.
2. **Tree-based merge**: Represent each commit as a complete tree snapshot. A
   merge computes file-level diffs between the base tree and each side's tree,
   then applies the results to the working directory. Used by Git, Mercurial, and
   Subversion.

# Decision

VARA uses **tree-based three-way merge** (RFC-0008).

`Merge()` resolves the merge base commit (LCA via BFS ancestors), reads the base
tree, the ours tree, and the theirs tree from the object store, then calls
`diff.DiffTrees(base→ours)` and `diff.DiffTrees(base→theirs)`. For files modified
on both sides, `diff.ThreeWayMerge` applies the Myers O(ND) diff3 algorithm at
the line level. For files changed on only one side, the change is applied
directly.

# Consequences

**Good:**
- The approach is correct regardless of how many intermediate commits led from
  the base to each tip. Replaying a sequence of patches is fragile when patches
  do not apply cleanly to the base (the "evil merge" problem).
- Each commit stores a complete tree, not a delta. Tree lookup is O(depth),
  which is fast for shallow trees. Merge never needs to replay a chain of patches.
- The content-addressed object store provides structural sharing between trees:
  files and subtrees that did not change between commits share their object IDs,
  so DiffTrees terminates early on equal subtrees.
- The line-level three-way merge (diff3) is well understood, has a known failure
  mode (conflict markers), and is expected by users familiar with Git.

**Bad:**
- Tree-based merge requires storing a full snapshot per commit. For repositories
  with large binary files, this is space-inefficient. VARA mitigates this through
  zstd compression and content deduplication in the object store.
- A pure patch-based merge can handle rename tracking more naturally (a rename is
  a single patch operation). Tree-based merge treats a rename as a delete + add
  and may produce spurious conflicts. VARA currently has no rename detection;
  this is a known limitation.
- The LCA computation (graph traversal) is O(commits). For very deep histories
  this is expensive; RFC-0013 Commit Graph Index mitigates this by caching
  generation numbers for fast LCA queries.

# Alternatives Considered

**Operational transformation (OT)**: Used by collaborative editing systems like
Google Docs. Handles concurrent edits without a shared base. Not applicable to
version control where a common ancestor always exists and the history is the
source of truth.

**Patch-based (Darcs, pijul)**: Records commits as patch theory operations that
commute. Provides a cleaner mathematical foundation for merge. Rejected because
patch commutation is complex to implement correctly, user-facing semantics differ
from Git (no conflict markers — conflicts become unapplied patches), and there is
no established tooling ecosystem to reference.

**Rebasing as the merge primitive**: Instead of a merge commit, replay ours on
top of theirs. Simpler history but non-associative (rebase A into B ≠ rebase B
into A) and rewrites commit hashes, breaking any external reference to the
original commits. VARA supports `vara merge --ff` (fast-forward) only; a rebase
command is a possible future addition.
