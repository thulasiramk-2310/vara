# VARA Implementation Status

This document tracks the maturity of the VARA protocol specification versus the
actual codebase implementation.

| RFC | Title | Spec Status | Code Status | Notes |
|-----|-------|-------------|-------------|-------|
| 0000 | Glossary | Accepted | ✅ | Informational only |
| 0001 | Vision | Accepted | ✅ | Informational only |
| 0002 | Object Format | Accepted | ✅ | `pkg/object`, `pkg/hash`, `pkg/compression` |
| 0003 | Repository Layout | Accepted | ✅ | `internal/repository` |
| 0004 | References | Accepted | ✅ | `pkg/refs`, `pkg/reflog` |
| 0005 | Index | Accepted | ✅ | `pkg/index`, `pkg/scanner` |
| 0006 | Locking | Accepted | ✅ | `internal/locking`, `internal/transaction` |
| 0007 | Commit Graph | Accepted | ✅ | `pkg/graph`, `pkg/builder` |
| 0008 | Merge Algorithm | Accepted | ✅ | `internal/merge`, `pkg/diff` |
| 0009 | Undo & Recovery | Accepted | ✅ | `internal/undo`, `pkg/recovery`, `pkg/snapshot` |
| 0010 | Configuration | Draft | 🚧 | `pkg/config` — remotes + INI implemented; resolution cascade pending |
| 0011 | AI Provider | Draft | ❌ | Not started |
| 0012 | Command Specification | Accepted | ✅ | `internal/commands`, `cmd/vara` |
| 0013 | Commit Graph Index | Accepted | ✅ | `pkg/graphindex` — RFC-0013 |
| —   | Repository Integrity | Planned | ✅ | `pkg/verify`, `internal/commands/verify.go` |
| 0014 | Remote Protocol | Draft | 🚧 | `pkg/transfer`, `internal/transport`, remote commands — local transport done; network deferred |
| 0015 | Pack Optimization | Planned | ❌ | Delta packs, boundary subtraction — future work |
| 0016 | Network Transport | Planned | ❌ | `vara serve`, `vara://` transport — future work |

## Commands Implemented

| Command | RFC | Status |
|---------|-----|--------|
| `vara init` | RFC-0012 §2 | ✅ |
| `vara add` | RFC-0012 §2 | ✅ |
| `vara commit` | RFC-0012 §2 | ✅ |
| `vara status` | RFC-0012 §2 | ✅ |
| `vara history` | RFC-0012 §2 | ✅ Fast path via RFC-0013 graph index |
| `vara branch` | RFC-0012 §2 | ✅ |
| `vara switch` | RFC-0012 §2 | ✅ |
| `vara merge` | RFC-0012 §2, RFC-0008 | ✅ |
| `vara undo` | RFC-0012 §2, RFC-0009 | ✅ |
| `vara verify` | RFC-0012 §2 | ✅ |
| `vara log` | RFC-0012 §2 | ✅ alias for history |
| `vara remote` | RFC-0014 §4 | ✅ add/remove/list |
| `vara clone` | RFC-0014 §9.1 | ✅ local transport |
| `vara fetch` | RFC-0014 §9.2 | ✅ local transport |
| `vara pull` | RFC-0014 §9.3 | ✅ fast-forward + three-way merge |
| `vara push` | RFC-0014 §9.4 | ✅ fast-forward check + `--force`; concurrent-push safe (Refs lock) |
| `vara gc` | RFC-0014 §12 | ✅ reclaim unreferenced objects; `--dry-run` |

## Performance Benchmarks

Measured on AMD Ryzen 9 8945HS, 10 000-commit repository:

| Command | Budget | Last Measured |
|---------|--------|---------------|
| `vara history` (10k commits) | ≤ 100 ms | **16.3 ms warm** (RFC-0013 graph index, was 75.8 s — 4,651× faster) |
| `vara status` (10k files) | ≤ 200 ms | pending |
| `vara add` (1k files) | ≤ 500 ms | pending |
| `vara commit` | ≤ 100 ms | pending |
| `vara switch` | ≤ 200 ms | pending |

## Correctness Infrastructure

- Integration tests: `tests/integration/` (switch, merge, verify, undo)
- Fuzz tests: `tests/fuzz/` (ref names, journal parser, tree/blob/commit objects, verify)
- Benchmarks: `benchmarks/commands/`
- Golden tests: `tests/golden/`

## Known Limitations

- Author identity is hardcoded as `User <user@example.com>` (RFC-0010 not implemented)
- No remote protocol (RFC-0014 deferred)
- No rename detection in merge
- No pack-file compaction (objects grow unboundedly)
- No `vara stash` command
