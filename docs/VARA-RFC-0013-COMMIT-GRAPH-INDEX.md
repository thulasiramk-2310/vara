VARA RFC: 0013
Title: Commit Graph Index
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0002, RFC-0003, RFC-0007
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

This document defines the Commit Graph Index — a derived, read-only index over
the commit DAG that eliminates repeated per-commit object-store reads during
history traversal, merge-base computation, and generation-number queries.

**Key invariant:**

> The graph index is derived state. A repository remains fully valid if the index
> file does not exist. The index MUST always be rebuildable from the object store
> alone. It MUST NOT become authoritative for any protocol operation.

# 2. Motivation

Benchmarking reveals that `vara history` on a 10 000-commit repository requires
10 000 sequential disk reads (one per commit object), each involving zstd
decompression and SHA-256 verification. This exceeds the 100 ms budget by
~760×.

The graph index eliminates this by:
1. Accumulating all reachable commit metadata in a single binary file.
2. Loading that file in one read (~1.8 MB for 10 000 commits).
3. Performing all graph traversal entirely in-memory.

Individual commit objects are only read when their full content (blob data,
tree entries) is needed — not merely for history or graph operations.

# 3. File Location

```
.vara/
  graph.idx           <- the index file
```

`graph.idx` is a single flat binary file. It is not version-controlled, not
transferred during remote operations (future), and not referenced by any
canonical repository state file.

# 4. Binary Format

All multi-byte integers are big-endian.

## 4.1 Header (16 bytes)

| Offset | Size | Field       | Description                    |
|--------|------|-------------|--------------------------------|
| 0      | 4    | Magic       | ASCII "VAGI"                   |
| 4      | 4    | Version     | uint32 = 1                     |
| 8      | 4    | CommitCount | uint32, number of entries      |
| 12     | 4    | _reserved_  | uint32, must be 0              |

## 4.2 Commit Table

Immediately follows the header. Each entry is exactly 56 bytes:

| Offset | Size | Field        | Description                                      |
|--------|------|--------------|--------------------------------------------------|
| 0      | 32   | ID           | SHA-256 commit hash                              |
| 32     | 4    | Generation   | uint32, 0 for root, max(parents)+1 otherwise     |
| 36     | 8    | Timestamp    | int64 Unix seconds                               |
| 44     | 4    | ParentOffset | uint32, index into the Parent Array              |
| 48     | 4    | ParentCount  | uint32, number of parent IDs in Parent Array     |
| 52     | 4    | MetaOffset   | uint32, byte offset into the Meta Block          |

## 4.3 Parent Array

Immediately follows the Commit Table. A flat sequence of 32-byte commit IDs.
Entry `i` in the commit table references parents at indices
`[ParentOffset, ParentOffset+ParentCount)`.

## 4.4 Meta Block

A packed UTF-8 byte region. Each commit's metadata begins at MetaOffset and
has the format:

```
author \x00 message \x00
```

Both strings are NUL-terminated. Readers stop at the second NUL.

## 4.5 Checksum

The final 32 bytes are the SHA-256 hash of all preceding bytes. Any mismatch
invalidates the index and triggers a rebuild.

# 5. Lifecycle

## 5.1 Build

A full rebuild walks all commits reachable from any branch tip, collects their
metadata from the object store, and writes `graph.idx`. Cost: O(N) object reads
(one per commit), performed once per invalidation cycle.

## 5.2 Invalidation

`graph.idx` is deleted (or truncated to zero) whenever the commit graph changes:
- After `vara commit`
- After `vara merge` (success)

## 5.3 Lazy Rebuild

On first access after invalidation, the consumer calls `LoadOrBuild`. This
function attempts to `Load`; on failure or absence it calls `Build` then
`Load`. The cost is paid once; subsequent operations in the same session use
the warm in-memory index.

## 5.4 Incremental Append (future — RFC-0013v2)

For large repositories, a full rebuild on every commit becomes expensive.
A future version of this RFC may define an append-only segment format with
periodic compaction, similar to Git's commit-graph chain files.

# 6. Access Pattern

```
vara history
    │
    ├─ LoadOrBuild(varaDir, store)
    │       │
    │       ├─ Load(graph.idx)  ← one file read
    │       │       │
    │       │       └─ validate checksum
    │       │
    │       └─ [cache miss] Build → write graph.idx → Load
    │
    └─ BFS from HEAD using in-memory parent pointers
            │
            └─ Format output (no object-store reads needed)
```

# 7. Compatibility

The graph index is explicitly NOT part of the stable repository format. Tools
MUST NOT fail if `graph.idx` is absent or corrupt. They MUST trigger a rebuild
in that case. Any repository-format version bump (RFC-0003) does NOT require a
graph index format change; they evolve independently.

Guaranteed stable (RFC-0002/0003 stable format):
- Object files (.vara/<2hex>/<62hex>)
- Reference files (.vara/refs/)
- HEAD, index, journal

Internal / rebuildable:
- graph.idx
- snapshots (RFC-0009)
- lock files

# 8. Future Extensions

The Commit Table's fixed-size entry format is designed to accommodate future
fields by incrementing the Version field:

- Version 2: Bloom filter offset field (RFC-0014 placeholder)
- Version 2: Pack-file offset for compressed object lookup
- Version 3: Path-history reachability bitmaps

Readers encountering an unknown version MUST discard the index and rebuild.
