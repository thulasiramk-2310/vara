# VARA Roadmap

```
   Engine  →  Transport  →  Platform  →  Applications
  (objects)   (clone/push)  (identity·   (UI · CLI ·
                             authz·repos·  collaboration)
                             accounts)
```

VARA is a transactional, content-addressed distributed version control system,
built RFC-first: every capability is specified, reviewed, and accepted as an RFC
*before* it is implemented, and each new capability is added **above** the frozen
layers below it — never by reshaping them.

This document is the map. Read it after the README.

## The mental model

VARA is four layers. Each depends only on the layer beneath it; nothing reaches
downward.

```
        Applications          web UI · CLI · desktop · AI · integrations
             │
        VARA Hub              accounts · sessions · collaboration · orgs
             │
        VARA Platform         identity · authorization · repository management
             │
        VARA Engine           objects · refs · graph · transactions · merge · transport
```

The defining property, validated four RFCs running (0016→0019): **new capability
is absorbed without changing the engine.** `pkg/*` and the transport interface
have an empty diff across the entire Platform layer.

## Versions

| Version  | Focus                                                     | Status         |
| -------- | --------------------------------------------------------- | -------------- |
| **v0.1** | Engine + Distributed Transport                            | ✅ Complete     |
| **v0.2** | Hub Backend (Identity, Authorization, Repo Management, Accounts) | 🚧 In Progress |
| **v0.3** | Organizations + Teams + REST API                          | Planned        |
| **v0.4** | Pull Requests + Issues + Reviews                          | Planned        |
| **v1.0** | Stable Platform                                           | Future         |

## RFC status

### ✅ Accepted & Implemented

| RFC | Title | Code |
|-----|-------|------|
| 0002 | Object Format | `pkg/object`, `pkg/hash`, `pkg/compression` |
| 0003 | Repository Layout | `internal/repository` |
| 0004 | References | `pkg/refs` |
| 0005 | Index | `pkg/index` |
| 0006 | Locking & Transactions | `internal/locking`, `internal/transaction` |
| 0007 | Commit Graph | `pkg/graph` |
| 0008 | Merge Algorithm | `pkg/diff`, `internal/merge` |
| 0009 | Undo & Recovery | `pkg/snapshot`, `pkg/recovery`, `internal/undo` |
| 0012 | Command Specification | `internal/commands`, `cmd/vara` |
| 0013 | Commit Graph Index | `pkg/graphindex` |
| 0014 | Remote Protocol | `internal/transport` (Local), `pkg/transfer`, clone/fetch/pull/push/gc |
| 0016 | Remote Transport Protocol (HTTP Binding v1) | `internal/protocol`, `internal/transport` (HTTP), `internal/server`, `vara serve` |
| 0017 | Identity & Authentication | `internal/identity` |
| 0018 | Authorization & Repository Policy | `internal/authz` |
| 0019 | Repository Management & Ownership | `internal/repomanager`, control plane in `internal/server`, `vara repo` |

### 🚧 Accepted, Implementation Pending

| RFC | Title | State |
|-----|-------|-------|
| 0020 | Accounts & Sessions | Accepted (v1.0.0) — the last foundational Platform RFC; `internal/identity` credential store + sources next |

### 📋 Planned

| RFC | Title | Focus |
|-----|-------|-------|
| 0021 | REST / Management API | The control-plane surface a UI and CLI both consume |
| 0022 | Web Dashboard | Presentation over the management API |
| 0023 | Collaboration | Issues & pull requests, referencing repositories by immutable ID |
| —    | Organizations & Teams | Multi-owner namespaces refining who an owner is |

### 🧊 Deferred (specified as future work, not yet scheduled)

- **RFC-0010 Configuration** — full env/flags/global/system cascade; wire real
  author into commits (partial: `pkg/config` handles remotes).
- **RFC-0011 AI Provider** — provider-agnostic AI layer.
- **RFC-0015 Pack Optimization** — delta packs, thin packs, full boundary
  subtraction.
- **Repository namespaces** (RFC-0019 §14) — `owner/repo` paths and the
  id-addressed physical storage that best supports them.
- **Soft delete / archival** (RFC-0019 §5.4) — activate the `Archived` state and
  a restorable delete.
- **Public visibility semantics** (RFC-0019 §6.2) — the authorization rule that
  makes `public` meaningful.
- **Policy administration over the wire** (RFC-0019 §7.4) — `PUT .../policy`.

## Architecture invariants (never violated as VARA grows)

1. **The engine is frozen.** New capability layers above it; a correctness bug is
   the only reason to reopen a foundational layer.
2. **No downward imports.** Lower layers never import higher ones — enforced
   mechanically by `tests/architecture`.
3. **Single Implementation Principle.** Exactly one place knows repository layout;
   bindings delegate to it, never reimplement it.
4. **Authenticate → Authorize → act.** Every request resolves identity, then
   authorization, before any effect — 401 and 403 are never swapped.
5. **Policy, metadata, and accounts live outside repositories** — server-managed,
   never pushable.
