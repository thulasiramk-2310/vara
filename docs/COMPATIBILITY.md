# VARA Compatibility Guide

This document defines which parts of the VARA repository format and protocol are
stable (guaranteed not to change in incompatible ways within a major version) and
which are internal (subject to change without notice).

## Stable Components

These on-disk structures are part of the **VARA Repository Format v1** defined in
RFC-0002 and RFC-0003. Tools that read a VARA repository can rely on them.

### Object Store (RFC-0002)
```
.vara/<2hex>/<62hex>   — zstd-compressed objects
```
- **Format**: `<type>\x00<content>` before compression, SHA-256 after
- **Guarantee**: Any SHA-256 content-addressed VARA repository has this layout.
  Object IDs are stable across all tools and versions.
- **Types**: `blob`, `tree`, `commit` — format is stable within major version.

### References (RFC-0004)
```
.vara/HEAD             — "ref: refs/heads/<branch>\n" or "<hexid>\n"
.vara/refs/heads/<br>  — "<hexid>\n"
```
- **Format**: 64-char hex SHA-256 + newline. Stable.
- **Guarantee**: Name validation rules (RFC-0004 §3) are frozen.

### Index (RFC-0005)
```
.vara/index            — serialized staging area
```
- **Format**: Defined in `pkg/index/index.go` (protobuf-style binary).
- **Stability**: Minor format revisions may occur; consumers should re-run
  `vara add` if the index cannot be parsed.

### Transaction Journal (RFC-0006)
```
.vara/journal/txn-<id>.json
```
- **Guarantee**: The `state` field values (`execute`, `verify`, `commit`) are
  stable. Recovery tools can depend on these to determine transaction status.

### Reflog (RFC-0004)
```
.vara/logs/HEAD
.vara/logs/refs/heads/<br>
```
- **Format**: NUL-separated append-only log (old-id, new-id, author, message).
- **Stability**: Stable; consumers must handle missing files gracefully.

---

## Internal Components

These components are **implementation details** and may change between versions
without affecting the stable format guarantee.

### Commit Graph Index (RFC-0013)
```
.vara/graph.idx
```
- **Status**: Derived state — ALWAYS rebuildable from the object store.
- **Guarantee**: `graph.idx` MAY be absent, corrupt, or in a different binary
  format after an upgrade. Any consumer MUST call `LoadOrBuild`, not `Load` alone.
- **Do NOT**: Treat `graph.idx` as authoritative, ship it in a tarball, or assume
  its format is frozen.

### Snapshots (RFC-0009)
```
.vara/snapshots/snap-*.tar.zst
```
- **Status**: Internal safety captures; not versioned, not push-able.
- **Guarantee**: Each snapshot is a valid zstd-compressed tar of the working
  directory at the time of capture. The filename encodes the timestamp and
  operation, but the format may change.

### Lock Files (RFC-0006)
```
.vara/locks/<name>.lock
```
- **Status**: Advisory; present only during operations.
- **Guarantee**: A lock file being present means the resource is held by a live
  process. A stale lock (process died) can be detected and cleared.
  The file format is not defined beyond "existence = locked".

### Graph and Merge Internals (`pkg/graph`, `internal/merge`)
- **Status**: Go library code; not part of any ABI or wire format.
- **Guarantee**: None. These packages may be refactored or replaced.

---

## Version Policy

| Artifact | Versioning | Change Policy |
|----------|-----------|---------------|
| Repository Format | `format-version` in RFC-0003 | Bump major on incompatible change |
| CLI commands | Semantic version (CLI v0.1.0-alpha) | No stability guarantee until v1.0.0 |
| Go packages (`pkg/`) | Internal only | No API stability until v1.0.0 |
| RFCs | See RFC-LIFECYCLE.md | Accepted RFCs are stable |

## Current Versions

- **Repository Format**: v1 (RFC-0003)
- **Protocol**: v1.0.0 (RFC-0002)
- **Implementation**: v0.1.0-alpha
- **CLI**: v0.1.0-alpha

## Upgrade Path

When the repository format changes (format-version bump in RFC-0003):
1. VARA will detect the old format on `vara init` (for a new repo) or on first
   access to an existing repo.
2. A migration command (`vara migrate`) will be provided.
3. The old format will remain readable for at least one major version.

For the current alpha period, **no migration guarantees are made**.
Repositories created with v0.x may not be compatible with future v0.x releases.
