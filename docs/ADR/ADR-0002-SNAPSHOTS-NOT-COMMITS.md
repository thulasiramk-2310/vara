ADR-0002: Snapshots Are Not Commits
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

Before a destructive operation (switch, merge, undo), VARA captures the current
working directory so the user can recover if something goes wrong. There are two
natural representations for this saved state:

1. **Use the commit object format**: Save a commit that records the current tree.
   The commit would appear in history and could be checked out with `vara switch`.
2. **Use a separate snapshot archive**: Save a `.tar.zst` of the working directory
   to a named file under `.vara/snapshots/`. The snapshot is not a commit object
   and is not visible to `vara history`.

# Decision

VARA uses **separate snapshot archives** (RFC-0009), not commits.

Snapshots are `.tar.zst` files stored under `.vara/snapshots/<timestamp>-<op>.tar.zst`.
They capture the working directory state at the time of the operation. They are
read by `pkg/recovery` for inspection and by `internal/undo` for restoration.
They are never referenced by any branch ref or parent pointer.

# Consequences

**Good:**
- History stays clean. Safety snapshots are taken frequently (before every
  switch and merge); if each became a commit, `vara history` would be polluted
  with noise that the user never authored.
- Snapshots capture working-directory state including un-staged changes, which
  commits cannot (commits only record the tree as staged in the index).
- Snapshots can be garbage-collected by age or count without affecting any ref.
- The undo layer has three independent tools (journal → reflog → snapshot) for
  progressively deeper rollback; having snapshots as commits would collapse the
  reflog and snapshot layers together.

**Bad:**
- Snapshots cannot be pushed to a remote or shared between machines.
- A snapshot of 50 000 files at 10 KB each is 500 MB — the archive format adds
  zstd compression overhead versus storing only a tree diff.
- Users cannot `vara switch` to a snapshot; they must use `vara undo`.

# Alternatives Considered

**Stash commits (git-style)**: Git's `stash` creates actual commits on a detached
ref, which makes stashed state addressable by hash and push-able. VARA may adopt
this for a future `vara stash` command. For automatic pre-operation safety
captures, however, the extra ref management and history pollution costs outweigh
the benefits. The auto-snapshot is a safety net, not a user-visible artifact.

**Journal-only recovery**: Rely solely on the write-ahead journal for recovery.
The journal can roll back incomplete ref updates but cannot restore working-tree
content that was overwritten before the crash. Snapshots fill this gap.
