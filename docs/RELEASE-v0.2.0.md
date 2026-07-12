# VARA v0.2.0 Release Notes

**Released**: 2026-07-12
**Tag**: `v0.2.0`
**Codename**: Backend Platform Complete

---

## What v0.2.0 Is

v0.1.0-alpha completed the local engine. **v0.2.0 completes the backend
platform**: VARA is no longer just a repository engine but a self-hostable,
authenticated, multi-user version-control platform — engine, network transport,
identity, authorization, repository management, and accounts — every layer
specified in a governing RFC, implemented, and tested.

The defining property of this release: **every capability above the engine was
added without modifying the engine.** Across five platform RFCs (0016–0020) the
`pkg/*` engine and the transport interface have an empty diff, mechanically
enforced by `tests/architecture`. The abstractions held.

This remains pre-1.0: the on-disk object/index format is stable (RFC-0002/0005),
and the transport protocol carries a compatibility promise (see
`docs/COMPATIBILITY.md`), but higher-level surfaces may still evolve before v1.0.

---

## The platform, layer by layer

```
Engine  →  Transport  →  Identity  →  Authorization  →  Repo Management  →  Accounts & Sessions
  ✅           ✅            ✅              ✅                 ✅                     ✅
```

### Engine (RFC-0002–0013) — from v0.1, unchanged

Content-addressed object store (SHA-256 + zstd), index, commit graph + binary
graph index, three-way merge, transactional refs with journaling, three-layer
undo, integrity verification, and garbage collection.

### Transport (RFC-0014, RFC-0016)

- `clone` / `fetch` / `pull` / `push` over a local **and** an HTTP transport,
  selected by URL scheme behind one `Transport` interface.
- `vara serve` — the HTTP binding of the remote protocol (protocol `16.1`, wire
  `1`), with a structured error schema and capability negotiation.
- Compare-and-swap ref updates that **survive the network boundary**: concurrent
  pushes are serialized by the same Refs lock the local engine uses; exactly one
  of two divergent pushes wins (proven by `TestHTTPConcurrentPushOneWins`).

### Identity (RFC-0017)

- Pluggable `IdentitySource` resolving a credential to a tiny `Identity{ID,
  Method}`. Anonymous / Basic / Bearer active; OAuth2/OIDC/mTLS/SSH reserved.
- Authentication terminates **above** the transport; the engine stays
  identity-agnostic. `401` for authentication failures, never conflated with
  authorization.

### Authorization (RFC-0018)

- Capability-based, operation-centric policy (`read` / `create-ref` / `push` /
  `force-push` / `delete-ref`), evaluated from a **server-managed policy outside
  the repository**, default-deny, before any transport method runs.
- Decisions read only identity + policy, never repository state. `403` for a
  denied-but-authenticated request — never swapped with `401`.

### Repository management (RFC-0019)

- A JSON **control plane** (`/_vara/repositories`) for repository lifecycle:
  create / delete / rename / list, with an immutable repository **ID** and a
  mutable name.
- Three separate artifacts per repository — content (engine), policy (authz),
  metadata (this layer) — never mixed; all-or-nothing create, tombstoned hard
  delete, crash-safe state machine; only `Active` repositories are served.
- A **server resource scope** extends authorization to host-level operations.

### Accounts & sessions (RFC-0020, v1.1)

- Durable **accounts**, interactive-login **sessions**, and long-lived **API
  tokens** — all persistent `IdentitySource` implementations that fill RFC-0017's
  interface without changing it.
- **argon2id** password hashing; high-entropy session/token secrets stored only
  as SHA-256 and shown once; **immediate revocation**; constant-time login that
  resists account enumeration; secrets never logged.
- Account admin reuses the RFC-0019 server scope (`manage-accounts`); an account
  may change its own password.

---

## Command surface

| Command | Area | Notes |
|---------|------|-------|
| `init` `add` `status` `commit` `log` `history` `branch` `switch` `merge` `undo` `verify` `gc` | Engine | from v0.1 |
| `clone` `fetch` `pull` `push` `remote` | Transport | local + HTTP |
| `serve` | Server | `--policy` / `--meta` / `--accounts` / `--basic` / `--bearer` |
| `repo` | Hub | create / delete / rename / list / show |
| `login` `logout` `token` `account` | Accounts | sessions, API tokens, account admin |
| `account create --accounts …` | Bootstrap | on-host first-admin, no HTTP/auth |
| `whoami` | Introspection | resolved identity + per-repo capabilities |
| `doctor` | Diagnostics | read-only repo/config/remote health check |

---

## Developer experience

- **`vara doctor`** — a fast, read-only health check across repo, config, and
  remote connectivity/auth, with actionable hints; exits non-zero on failure.
- **`vara whoami`** — shows who the server thinks you are and which capabilities
  you hold (`--repo`, `--repo _server`) — for debugging permissions.
- **On-host bootstrap** — `vara account create --accounts …` creates the first
  admin directly on the filesystem, removing the temporary-anonymous-grant step.

---

## Documentation

RFCs 0000–0020 (all accepted), ADRs, `ARCHITECTURE.md`, `ROADMAP.md`,
`COMPATIBILITY.md` (with the transport compatibility promise),
`IMPLEMENTATION-STATUS.md`, `CONTRIBUTING.md`, and these release notes.

---

## Architecture invariants held across the release

1. The engine is frozen; new capability layers above it (empty `pkg/*` diff).
2. No downward imports — enforced by `tests/architecture`.
3. Single Implementation Principle — one place knows repository layout.
4. Authenticate → Authorize → act; `401` and `403` never swapped.
5. Policy, metadata, and accounts live outside repositories, server-managed.

---

## Not in v0.2.0 (planned)

- Web dashboard / REST framing (future Hub work).
- AI provider layer (RFC-0011, deliberately deferred).
- Pack optimization / delta packs (RFC-0015).
- Namespaces, organizations, teams, collaboration (issues/PRs).
- Config cascade (RFC-0010) — commit author is still a placeholder.

---

## Upgrade notes

Repositories created by v0.1.0-alpha are format-compatible (object/index format
unchanged). New network, identity, authorization, repository-management, and
account features are additive; an existing anonymous `vara serve` deployment
behaves exactly as before unless the new flags are supplied.
