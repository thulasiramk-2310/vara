ADR-0006: The Commit Graph Index Is Derived State
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

After implementing RFC-0013 Commit Graph Index (`graph.idx`), a design question
arose: should `graph.idx` become part of the stable repository format — a file
that must be present for the repository to function — or remain a rebuildable
cache that is always optional?

The distinction matters:

- **Authoritative format**: consumers can read `graph.idx` directly and trust it
  as the source of truth for history. The repository is incomplete without it.
  Remote operations must transfer it.
- **Derived state**: `graph.idx` is always reconstructible from the object store.
  It is a performance index, not a protocol artifact. Its absence is a cache miss,
  not a corruption.

# Decision

`graph.idx` is **derived state**. It is never authoritative.

Concretely:
- Any consumer MUST call `graphindex.LoadOrBuild` (not `Load` alone). A missing
  or corrupt index triggers a rebuild, not an error.
- `graph.idx` is deleted (invalidated) after every mutation. The next access pays
  the rebuild cost once.
- The binary format version field exists to discard (not error on) incompatible
  versions — unknown versions trigger a silent rebuild.
- `graph.idx` is excluded from remote transfer (future). Receiving repositories
  build their own index.
- `vara verify` does NOT fail if `graph.idx` is absent.

The invariant is stated in RFC-0013 §1:

> "The graph index is derived state. A repository remains fully valid if the
> index file does not exist. The index MUST always be rebuildable from the
> object store alone. It MUST NOT become authoritative for any protocol
> operation."

# Consequences

**Good:**
- Corruption of `graph.idx` is self-healing: the next `vara history` call
  rebuilds it transparently.
- There is no migration burden when the binary format changes: old `graph.idx`
  files are simply discarded.
- The object store remains the single source of truth. There is no split-brain
  between `graph.idx` and the object store.
- Derived state can be deleted to reclaim disk space without any loss of
  information.

**Bad:**
- Every new clone (or session after long inactivity) pays a cold build cost that
  is O(N) in commits. For very large repositories (millions of commits), this
  could be 10+ minutes.
- Consumers cannot skip the object store entirely; if `graph.idx` is absent, they
  fall back to O(N) object reads.

# Alternatives Considered

**Make `graph.idx` authoritative (like Git's `packed-refs`)**: Once generated,
the index becomes the canonical source. This enables faster reads at the cost of
requiring consistent updates on every write. Rejected because it introduces a
second write path per mutation (objects AND index), complicates conflict
resolution when two processes write simultaneously, and creates a new failure
mode (corrupt index = corrupt repository).

**Store generation numbers in commit objects**: Embed generation numbers directly
in the commit object format. This eliminates the rebuild problem but permanently
increases object size and couples the graph acceleration to the protocol format.
Rejected — commit object format is stable and should not change for a performance
optimization.

# Future Evolution

RFC-0013 §5.4 defines a planned incremental append format. Instead of
invalidating and rebuilding the entire index on every commit, the index would
be extended by one entry per commit and compacted periodically. This will make
the cold build cost O(1) per commit rather than O(N) total — while still keeping
the index as derived state.
