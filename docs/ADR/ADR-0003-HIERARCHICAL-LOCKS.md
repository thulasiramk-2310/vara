ADR-0003: Hierarchical Lock Ordering
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

VARA operations mutate multiple shared resources concurrently (for example, two
`vara commit` processes in the same repository). Without ordering, two processes
can each hold one lock and wait for the other, causing deadlock.

The resources requiring locks are:
- `refs` — branch references and HEAD
- `index` — the staging area
- `objects` — the object store (content-addressed; writes are idempotent)

# Decision

VARA uses **a fixed global acquisition order**: `refs` before `index`.
Object-store writes are lock-free because objects are content-addressed and
immutable after creation — writing the same object twice is a no-op.

The order is enforced by `internal/locking` and validated by `internal/transaction`.
Any call to `transaction.Begin` that requests locks in a different order returns an
error at compile time via constant ordering checks.

Concretely: `NameRefs = 0`, `NameIndex = 1`. Callers always acquire in ascending
order.

# Consequences

**Good:**
- Deadlock is impossible by construction: any two processes that both need
  `refs + index` will both try to acquire `refs` first; one wins and the other
  waits. No cycle can form.
- The object store requires no locking, which keeps the hot write path
  (blob ingestion during `vara add`) lock-free.
- Lock acquisition code is trivial; the locking package only needs to implement
  file-based advisory locks, not a priority or dependency graph.

**Bad:**
- A process that only needs the index (e.g., `vara status`) still cannot run
  concurrently with a process that holds the index lock, even though it would not
  touch refs. The lock granularity is coarser than necessary for read-only
  operations.
- Adding a new resource (e.g., a packed-refs lock for RFC-0014) requires adding it
  to the ordering table and auditing all callers.

# Alternatives Considered

**Lock-free MVCC**: Use compare-and-swap on ref files (atomic rename + read-verify).
This is what Git's `packed-refs` transactions attempt. It avoids explicit lock
files but requires retry loops and is harder to reason about under contention.
Rejected for the initial implementation; the transaction layer can be upgraded
later without changing the RFC.

**Single global lock**: One `.vara/LOCK` file that all operations must hold.
Simpler than hierarchical ordering but serialises all concurrent operations,
making `vara add` on large trees block `vara status`. Rejected because it creates
a bottleneck that is hard to remove later without breaking the lock protocol.

**Per-resource locks with deadlock detection**: Allow arbitrary acquisition orders;
detect cycles at runtime and abort one process. Adds significant complexity and
non-determinism. Rejected — the fixed ordering is provably sufficient for the
current resource set.
