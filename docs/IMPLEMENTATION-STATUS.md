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
| 0016 | Remote Transport Protocol (HTTP Binding v1) | Accepted | ✅ | `internal/protocol`, `internal/transport` (HTTPTransport), `internal/server`, `vara serve`; HTTP parity suite green — anonymous v1 |
| 0017 | Identity & Authentication | Accepted | ✅ | `internal/identity` (Anonymous/Basic/Bearer/Multi); wired into `internal/server` preamble; 401 |
| 0018 | Authorization & Repository Policy | Accepted | ✅ | `internal/authz` (capability policy, file store, atomic reload); wired into `internal/server`; 403 |
| 0019 | Repository Management & Ownership | Accepted | ✅ | First VARA Hub RFC; `internal/repomanager` (three-artifact atomic lifecycle, immutable ID, state machine), control plane in `internal/server` (`/_vara/repositories`), server (`*`) resource scope in `internal/authz`, `vara serve --meta` + `vara repo` CLI; engine untouched |
| 0020 | Accounts & Sessions | Accepted | ✅ | Durable accounts + sessions + API tokens as persistent RFC-0017 `IdentitySource` impls in `internal/identity` (argon2id via `golang.org/x/crypto`, SHA-256 token hashes, immediate revocation, constant-time login); control plane `/_vara/sessions|tokens|accounts` in `internal/server`; `manage-accounts` server-scope; `vara serve --accounts` + `vara login/logout/token/account`; engine untouched |
| 0021 | Hub Read & Management API | Accepted | ⏳ | Read-only content endpoints (summary/branches/commits/commit) projecting the engine (H8), ETag + cursor pagination, httpOnly-cookie browser sessions, `vara serve --hub` same-origin static serving, `X-VARA-API` versioning; planned `internal/hub` projection layer; not yet implemented |

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
| `vara clone` | RFC-0014 §9.1 | ✅ local + HTTP transport |
| `vara fetch` | RFC-0014 §9.2 | ✅ local + HTTP transport |
| `vara pull` | RFC-0014 §9.3 | ✅ fast-forward + three-way merge |
| `vara push` | RFC-0014 §9.4 | ✅ fast-forward check + `--force`; concurrent-push safe (Refs lock); local + HTTP |
| `vara serve` | RFC-0016/0017/0018/0019/0020 | ✅ HTTP binding + identity/authz/repo-mgmt/accounts; `--addr`/`--root`/`--policy`/`--meta`/`--accounts`/`--basic`/`--bearer` |
| `vara gc` | RFC-0014 §12 | ✅ reclaim unreferenced objects; `--dry-run` |
| `vara repo` | RFC-0019 §8 | ✅ create/delete/rename/list/show over the control plane |
| `vara login` / `logout` | RFC-0020 §8.1 | ✅ session login (prints token once) / logout |
| `vara token` | RFC-0020 §8.2 | ✅ create/list/revoke API tokens |
| `vara account` | RFC-0020 §8.3 | ✅ create/disable/delete/passwd (manage-accounts + self) |
| `vara whoami` | RFC-0020 §8.5 | ✅ resolved identity + per-repo capabilities (`--repo`, `--repo _server`) |
| `vara doctor` | — (DX) | ✅ read-only health check: repo/config/remote; exits non-zero on failure |

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
