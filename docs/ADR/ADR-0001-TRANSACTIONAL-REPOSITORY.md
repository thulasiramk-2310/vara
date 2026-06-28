ADR-0001: Transactional Repository Design
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

Every mutating VARA operation (commit, switch, merge) touches multiple files that
must be changed atomically: the object store, the index, HEAD, and branch refs.
On a crash between two of these writes the repository ends up in an inconsistent
state — for example, a branch ref updated but the index not yet written, or the
reverse.

Two broad strategies exist:

1. **Write-ahead journaling**: Record intent before acting; replay or roll back on
   recovery.
2. **Copy-on-write with atomic swap**: Build new state in a temp location then
   rename into place; no journal required.

# Decision

VARA uses **write-ahead journaling backed by atomic file rename** (RFC-0006).

Each operation begins by writing a journal entry (`StateExecute`), performs its
mutations using `tmp → rename` for each individual file, advances the journal to
`StateVerify` once all mutations are complete, then commits (`StateCommit`). On
recovery, `pkg/recovery` inspects the journal: `StateExecute` entries are
incomplete and safe to ignore; `StateVerify` entries need a forward-pass to finish
the rename; `StateCommit` entries are complete.

File-level atomicity is achieved by writing to a `.tmp` sibling and renaming.
POSIX `rename(2)` is atomic; Windows `MoveFileEx` with
`MOVEFILE_REPLACE_EXISTING` provides equivalent semantics on NTFS.

# Consequences

**Good:**
- Crash at any point leaves the repository either fully committed or fully
  uncommitted — no partial states visible to readers.
- Recovery does not require reading object content; it only replays ref updates
  stored in the journal, which are small strings.
- The journal doubles as an audit log; `vara verify` can inspect it for
  incomplete transactions.

**Bad:**
- Every operation acquires locks before mutating. Lock acquisition can time-out
  or dead-lock if a process crashes while holding a lock.
- The journal is an extra write per operation; on latency-sensitive benchmarks this
  adds a small fixed overhead (~100 µs on an NVMe drive).
- Recovery logic is non-trivial and must be exercised by integration tests.

# Alternatives Considered

**Pure copy-on-write (git-style)**: Git builds new pack files and then swaps
pointers. This works for the object store but not for the index or HEAD, which are
single files that must be rewritten in place. Git uses the same lock-then-rename
approach for those files anyway — so the per-file atomicity is equivalent; the
difference is that VARA makes the multi-file transaction boundary explicit via the
journal.

**SQLite-backed repository**: Stores all repository state in a single SQLite
database, which provides ACID transactions natively. Rejected because it adds a
heavy dependency, complicates the on-disk format, and makes the repository
unreadable by standard file tools.
