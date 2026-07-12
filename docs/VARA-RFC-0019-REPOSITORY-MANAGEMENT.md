VARA RFC: 0019
Title: Repository Management & Ownership
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-12
Depends On: RFC-0016, RFC-0017, RFC-0018
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0016 moves objects to and from a repository. RFC-0017 says *who* is calling.
RFC-0018 says *what* they may do *to a repository that already exists*. Every one
of those documents assumes the repository is already there. This document defines
the layer that brings a repository into existence, removes it, renames it, lists
it, and records who owns it — the **control plane** of a VARA host, and the first
RFC of VARA Hub.

**The one-sentence scope:**

> RFC-0019 defines a server-managed lifecycle for repositories — `create`,
> `delete`, `rename`, `list` — gives each repository an immutable identity, a
> mutable name, ownership, visibility, and a lifecycle state, exposed as a JSON
> control-plane distinct from the git data-plane and authorized through RFC-0018
> against a new **server** resource scope. It answers "which repositories exist
> on this host, what is each one, and who administers it?" — nothing about object
> storage (the engine), nothing about moving objects (RFC-0016).

**Design stance — management is a binding above the engine, never inside it.**

> Creating a repository is `repository.Init` plus a policy seed plus a metadata
> record. Deleting one is a tombstone plus removals. The control plane
> *orchestrates* these primitives; it never reimplements repository layout
> (RFC-0003) or re-derives what `Init` already does. This is the Single
> Implementation Principle (RFC-0016 §9.1) applied to lifecycle: exactly one
> piece of code knows how a `.vara` directory is shaped, and the control plane
> calls it.

```
        HTTP Request  (control plane: /_vara/repositories/...)
             │
             ▼
    ┌─────────────────┐
    │  Identity       │  ← RFC-0017: who is calling? (or 401)
    └─────────────────┘
             │
             ▼
    ┌─────────────────┐
    │  Authorization  │  ← RFC-0018: may they? — resource is the SERVER for
    │                 │    create/list, the REPOSITORY for delete/rename (or 403)
    └─────────────────┘
             │  (allowed)
             ▼
    ┌─────────────────────────────────────┐
    │  Repository Manager                 │  ← RFC-0019: orchestrates lifecycle
    │   content  → repository.Init        │     over THREE artifacts:
    │   policy   → RFC-0018 seed/drop/move │       content · policy · metadata
    │   metadata → id/name/state/visibility│
    └─────────────────────────────────────┘
             │
             ▼
        repository.Init / os        ← engine + filesystem: lifecycle-agnostic
```

# 2. Motivation

Today a VARA host serves whatever repositories an operator has `vara init`-ed by
hand into the server root, and RFC-0018 policy files are hand-authored beside
them. There is no way for a *caller* to create a repository, no record of who may
administer one, no stable handle to reference a repository by after it is renamed,
and no answer to "what is hosted here?" that a client can ask. Every collaboration
feature above this point — a web UI, a CLI `vara repo create`, issues, pull
requests, org/team management — needs these four verbs, a stable id, an owner, and
a "what is this" record. RFC-0019 is the smallest layer that turns a directory of
repositories into a *managed* set.

It is deliberately the **first** VARA Hub RFC because everything else in Hub sits
on it: accounts (RFC-0020) feed identities into RFC-0017; a REST API (RFC-0021) is
a presentation over these operations; a dashboard (RFC-0022) renders them;
collaboration (RFC-0023, issues/PRs) references repositories *by their immutable
id*. None of that can be specified until "a repository can be created, owned,
identified, and destroyed" is nailed down.

# 3. Non-Goals

Explicitly **out of scope**, each deferred to a later RFC:

* **Accounts, registration, login, sessions.** RFC-0019 consumes an
  already-resolved `Identity` (RFC-0017). An "owner" is just a subject ID; how a
  human obtains that identity is RFC-0020. There is no user database here.
* **Namespaces / `owner/repo` paths.** v1 keeps a **flat** name space: one
  segment, `<root>/<name>`, matching RFC-0018's flat `<name>.json`. Nested
  ownership paths (`alice/website`) — and the id-addressed storage that best
  supports them — are future work (§14).
* **Web UI, REST framing.** The control plane is JSON over the RFC-0016 preamble.
  A full REST surface and its rendering are RFC-0021/0022.
* **Teams, orgs, transfer-of-ownership, multiple owners.** v1 has a single owner
  per repository, set at creation. Reassignment is future work.
* **Repository content operations.** Branch listing, commits, diffs — those are
  the data-plane (RFC-0016) or the local engine, not management.
* **Quotas, size limits, forking.** Future work (§14).
* **Soft delete / recycle bin.** v1 delete is a **hard** delete (§5.6); a
  restorable soft delete is reserved.
* **Engine changes.** RFC-0019 adds no field, method, or concept to `pkg/*`. It
  is proven, like RFC-0017/0018 before it, by an unchanged engine.

# 4. Terminology

* **Control plane** — the JSON management API of RFC-0019 (`/_vara/...`), distinct
  from the **data plane** (the git transport of RFC-0016). The two share a host
  and an auth pipeline but never share an endpoint.
* **Repository Manager** — the server-side component that performs lifecycle
  operations by orchestrating `repository.Init`, the filesystem, the RFC-0018
  policy, and the metadata record.
* **Repository ID** — an opaque, **immutable** identifier assigned at creation
  (e.g. `repo_9f3a…`, 128 bits of randomness) that never changes for the life of
  the repository, including across rename. It is the stable handle future Hub
  features reference.
* **Name** — the human-facing, **mutable** single-segment identifier used in URLs
  and on disk (`<root>/<name>`). Rename changes the name; it never changes the ID.
* **Metadata** — the record answering *"what is this repository?"*: id, name,
  visibility, state, description, timestamps. Metadata is separate from policy
  (which answers *"who may?"*) and from content (the objects/refs). See §6.
* **Owner** — the subject recorded as administering a repository, seeded at
  creation with the repository's management capabilities. **Ownership is a
  presentation concept derived from policy: the authorization system is
  authoritative; ownership is informational only.** No code branches on
  `subject == owner`.
* **Server resource** — the RFC-0018 resource scope naming the host itself,
  written `*`, used to authorize operations not about one existing repository
  (`create-repo`, `list-repos`). Policy in `<policy-root>/_server.json` (§7.2).
* **State** — the repository's lifecycle position: `Creating`, `Active`,
  `Archived`, `Deleting` (§5.4). `Creating`/`Deleting` are on-disk tombstones.

# 5. The Lifecycle Model

## 5.1 Operations (closed vocabulary, operation-centric)

Exactly four operations, each mapping to one observable control-plane request.
Following RFC-0018's rule — *a capability exists only if it corresponds to an
observable operation* — each has exactly one primary required capability (§7.1),
and no operation is a client-side synonym for another.

| Operation | Meaning                                             | Idempotent |
|-----------|-----------------------------------------------------|------------|
| `create`  | Bring a new, empty repository into existence        | No (409 if the name is taken) |
| `delete`  | Hard-delete an existing repository (all artifacts)  | Effectively (404 if absent) |
| `rename`  | Change a repository's name; the ID is preserved      | No |
| `list`    | Enumerate the repositories visible to the caller     | Yes (read-only) |

`create` is not "idempotent create": re-creating a taken name is a `409` conflict,
never a silent success, so a caller is never misled about whether it just created
the repository or collided with someone else's.

## 5.2 Three artifacts, three concerns

A managed repository is **three separate artifacts**, each owned by a different
layer and answering a different question:

| Artifact | Location                       | Owner / RFC        | Question    |
|----------|--------------------------------|--------------------|-------------|
| Content  | `<root>/<name>/.vara/…`        | engine (RFC-0003)  | the objects |
| Policy   | `<policy-root>/<name>.json`    | RFC-0018           | who may?    |
| Metadata | `<meta-root>/<id>.json`        | RFC-0019 (this)    | what is it? |

They are kept in step by the manager and by the lifecycle invariants (§5.5–5.7).
Crucially, **policy and metadata are never mixed** (M9): a policy file contains
only capability grants; a metadata file contains only descriptive fields and never
a capability. This is why "owner" is derived — the *authoritative* grant lives in
policy, and metadata's `owner` field is a rendered summary of it.

Content and policy are **name-addressed** (preserving RFC-0018's accepted keying
and the data-plane path resolution unchanged). Metadata is **id-addressed**, so
the same logical repository keeps one metadata record across a rename. The `name`
field inside metadata is the join key from the id back to the name-addressed
artifacts; the manager maintains name→id resolution and guarantees a name maps to
at most one non-`Deleting` repository.

## 5.3 Identity: immutable ID, mutable name

Every repository has an immutable **ID** assigned at creation and a mutable
**name**. Rename (§5.7) changes only the name. Anything that must survive a rename
— a future issue tracker, pull requests, web permalinks, audit logs — references
the **ID**; anything human-facing (URLs today, listings) uses the **name**. This
split is why RFC-0019 is worth doing before the collaboration RFCs: it gives them
a stable anchor from day one, so a repository rename can never orphan an issue or
break a permalink.

> **Storage note (deliberate).** v1 stores content and policy *by name*, not by
> id, so RFC-0018's frozen `<name>.json` policy keying and RFC-0016's data-plane
> path resolution need no change. Id-addressed physical storage (which would make
> rename a pure metadata update with no tree move) is cleaner but would churn an
> accepted layer; it is reserved to the namespaces work (§14), where it pays for
> itself.

## 5.4 Repository state machine

Metadata carries a `state`. The legal states and transitions:

```
   (nonexistent)
        │  create
        ▼
     Creating ───────► (rollback) ──► (nonexistent)      [failed create]
        │  content+policy seeded
        ▼
      Active ◄────────► Archived        [archive / unarchive — reserved, §14]
        │  delete
        ▼
     Deleting ─────────► (nonexistent)                    [artifacts removed]
```

* **`Creating`** and **`Deleting`** are on-disk tombstones that make create and
  delete crash-safe (§5.5, §5.6). They are never servable and never listed as
  live.
* **`Active`** is the only state the **data plane will serve** (M10): a repository
  is reachable over RFC-0016 iff its metadata state is `Active`. A `Creating`,
  `Deleting`, or `Archived` repository returns the data-plane's 404/409 as
  appropriate, so a half-built or archived repository is never mistaken for a live
  one.
* **`Archived`** (read-only, still present) and its transitions are **reserved**;
  v1 need not expose archive/unarchive endpoints, but the state and its meaning
  are fixed now so soft delete and archival land later without a model change.

Metadata-gated serving applies only when a Repository Manager is configured. A
bare RFC-0016 transport server with no Hub serves by directory existence exactly
as today — RFC-0016/0018 behavior is unchanged for non-Hub deployments.

## 5.5 Create is all-or-nothing, tombstoned

`create` must produce all three artifacts or none — an `Active` repository must
never exist without both a policy and metadata, and content must never be servable
without a policy (which, being `DenyAll` by absence, would strand it). The manager:

1. Allocate an ID; write metadata `{id, name, state:"Creating", …}`. This claims
   the name — a concurrent `create` of the same name sees it taken → `409`.
2. Materialize content via `repository.Init(<root>/<name>)` (engine owns this).
3. Seed the policy `<policy-root>/<name>.json` for the owner (§7.3).
4. Flip metadata `state:"Active"`.

If any step fails, the manager rolls back every artifact written so far (policy,
content, metadata) and the repository does not exist. A process crash between
steps leaves metadata in `Creating`: because `Creating` is never servable and
never listed live, such a record is a harmless orphan that a sweep (or the next
create of that name) reclaims. This is the same journaled "tombstone then commit"
shape the engine uses for refs (RFC-0006) — applied to lifecycle. (Normative: M6.)

## 5.6 Delete is a hard delete, tombstoned

v1 `delete` is a **hard** delete: it removes the content tree, the policy, and the
metadata. To stay crash-safe it tombstones first:

1. Flip metadata `state:"Deleting"`. From this instant the repository is logically
   gone — the data plane refuses it (not `Active`), and it is excluded from live
   listings.
2. Remove the content tree.
3. Remove the policy file.
4. Remove the metadata record.

A crash after step 1 leaves `Deleting`, which a sweep (or a retried delete)
resumes — the operation is effectively idempotent and can never resurrect a
partially deleted repository. Soft delete (retain content, mark restorable) is
reserved (§14); v1 does not promise recoverability after `delete`. (Normative: M7.)

## 5.7 Rename changes the name, preserves the ID

`rename` moves the **name-addressed** artifacts — `<root>/<old>` → `<root>/<new>`
and `<policy-root>/<old>.json` → `<policy-root>/<new>.json` — and updates the
`name` field in the (id-addressed) metadata, preserving content, policy, **and
ID** verbatim. Because a rename makes a name that did not exist begin to exist, it
is authorized as the conjunction of two capabilities (§7.1): `rename-repo` on the
**old** repository *and* `create-repo` on the **server**. Renaming onto a taken
name is a `409` conflict, never an overwrite. The ID never changes, so every
id-keyed reference elsewhere remains valid across the rename.

## 5.8 List returns a stable, ordered set

`list` returns the descriptors (§6.1) of the repositories the caller may
enumerate, **excluding** any in `Creating` or `Deleting` (tombstones are never
live). In v1, enumeration is gated by a single `list-repos` capability on the
server resource (all-or-nothing visibility). Results are returned in a **stable,
deterministic order: ascending lexicographic (byte-order) by `name`**, so
identical calls return identical sequences and clients can page predictably.
(Ordering by creation time, and per-repository filtered visibility — show only
what the caller can `read` — are reserved query options, §14; v1 does not promise
them, so a client must not infer read access from presence in the list.)

# 6. Repository Metadata & Visibility

## 6.1 Metadata schema (the descriptor)

The metadata record is also the **descriptor** the control plane returns:

```json
{
  "id":          "repo_9f3a1c7e0b5d4a28",
  "name":        "website",
  "owner":       "alice",
  "visibility":  "private",
  "state":       "active",
  "description": "",
  "archived":    false,
  "created_at":  "2026-07-12T10:00:00Z",
  "updated_at":  "2026-07-12T10:00:00Z"
}
```

* `id` — immutable (§5.3). `name` — mutable (rename). `owner` — derived from
  policy (§4), informational.
* `state` — §5.4. `archived` is a convenience mirror of `state == "archived"`.
* Timestamps are RFC 3339 UTC. Additional fields (size, default branch, topics)
  are additive and negotiated like any other growth (COMPATIBILITY).
* A metadata file contains **no capability grants** — those live only in policy
  (M9).

## 6.2 Visibility (closed enum)

`visibility` is a closed enum: **`private`** | **`public`**. v1 defaults every
repository to `private` and treats all repositories as private for authorization
— **authorization in v1 is driven purely by RFC-0018 policy, never by the
visibility field.** `public` is reserved: a later RFC MAY define a rule such as
"a `public` repository grants `read` to `anonymous`," at which point visibility
becomes an input the authorization layer consults. Fixing the enum now means that
rule can be added without a schema change, and clients can round-trip the field
today. Visibility is metadata (*what is this?*), never policy (*who may?*); the two
stay separate (M9).

# 7. Authorization of Management (extends RFC-0018)

RFC-0019 does not introduce a second authorization mechanism. It **extends**
RFC-0018's evaluator with (a) a new resource scope and (b) new capability tokens.
The pipeline, default-deny invariant, policy storage, and 401-vs-403 split are
inherited unchanged.

## 7.1 New capabilities (closed) and required-capability map

| Capability     | Resource scope | Granted operation                        |
|----------------|----------------|------------------------------------------|
| `create-repo`  | server (`*`)   | `POST /_vara/repositories`               |
| `list-repos`   | server (`*`)   | `GET /_vara/repositories`                |
| `delete-repo`  | repository     | `DELETE /_vara/repositories/{repo}`      |
| `rename-repo`  | repository     | `POST /_vara/repositories/{repo}/rename` (old name) |
| `admin`        | repository     | read/modify policy & metadata (§7.4)     |
| `archive`      | repository     | archive/unarchive — **reserved** (§5.4)  |

Required-capability map (analogue of RFC-0018 §8.1):

| Request                                   | Required capability(ies)                          |
|-------------------------------------------|---------------------------------------------------|
| `GET /_vara/repositories`                 | `list-repos` on `*`                               |
| `POST /_vara/repositories`                | `create-repo` on `*`                              |
| `GET /_vara/repositories/{repo}`          | `admin` on `{repo}` **or** `read` on `{repo}`     |
| `DELETE /_vara/repositories/{repo}`       | `delete-repo` on `{repo}`                          |
| `POST /_vara/repositories/{repo}/rename`  | `rename-repo` on `{repo}` **and** `create-repo` on `*` |
| `PUT /_vara/repositories/{repo}/policy`   | `admin` on `{repo}` (§7.4)                         |

A request requiring two capabilities is denied unless **both** are held, evaluated
before any effect — exactly as RFC-0018 authorizes before the transport opens.

## 7.2 The server resource (`*`)

Server-scoped operations (`create-repo`, `list-repos`) are authorized against the
reserved resource id `*` ("this host"), with policy in
`<policy-root>/_server.json` (same shape as a repository policy, RFC-0018 §7.1). A
missing `_server.json` is `DenyAll`: on a fresh host nobody may create or list
until an operator grants those capabilities to a bootstrap subject (§11). This
preserves default-deny at the server scope.

## 7.3 Ownership is a capability seed

When identity `S` creates repository `R`, the manager seeds
`<policy-root>/R.json` granting `S` the repository-scoped management capabilities
plus full data-plane access:

```json
{ "version": 1, "subjects": { "S": [
  "read", "create-ref", "push", "force-push", "delete-ref",
  "delete-repo", "rename-repo", "admin"
] } }
```

`S` is rendered as `owner` in the descriptor. **Ownership is a presentation
concept derived from policy; the authorization system is authoritative, ownership
is informational only.** There is no `if subject == owner` check anywhere —
ownership is expressed entirely as capabilities, so authorization stays a pure
`(subject, action, resource)` evaluation with no special cases (RFC-0018 A5
preserved).

## 7.4 Policy administration (`admin`)

A repository's policy is editable through the control plane by a subject holding
`admin` on that repository (initially the owner): `PUT
/_vara/repositories/{repo}/policy` replaces the body (validated against RFC-0018
§7.1, fail-fast on unknown capabilities). An `admin` subject **cannot** escalate
the server scope — editing a repository policy can never grant
`create-repo`/`list-repos`, which live only in `_server.json`. v1 MAY defer
implementing `PUT .../policy` (operators edit files directly), but the capability
and route are reserved so the model is complete.

# 8. Control-Plane API (HTTP Binding)

All control-plane routes live under the reserved prefix `/_vara/` so they can
never collide with a data-plane repository route (`/{repo}/info/refs`).

## 8.1 Endpoints

```
GET    /_vara/repositories               → 200 {repositories:[descriptor...]}   (stable order, §5.8)
POST   /_vara/repositories               → 201 descriptor        (body {name, visibility?, description?})
GET    /_vara/repositories/{repo}        → 200 descriptor
DELETE /_vara/repositories/{repo}        → 204
POST   /_vara/repositories/{repo}/rename → 200 descriptor        (body {new_name})
PUT    /_vara/repositories/{repo}/policy → 204                   (body: policy §7.4, reserved)
POST   /_vara/repositories/{repo}/archive→ 200 descriptor        (reserved, §5.4)
```

The same request preamble as the data-plane runs first: echo protocol headers,
version check, **authenticate** (401), then **authorize** (403) per §7.1. The
control plane reuses `internal/server`'s preamble verbatim — one authentication
path, one authorization path, two families of routes.

## 8.2 Status codes

| Situation                                  | Status | Code (RFC-0016 §8.6 schema)     |
|--------------------------------------------|--------|---------------------------------|
| Created                                    | 201    | —                               |
| Deleted / policy replaced                  | 204    | —                               |
| Name already taken (create / rename-onto)  | 409    | `REPOSITORY_EXISTS`             |
| No such repository (get/delete/rename)     | 404    | `UNKNOWN_REPOSITORY`            |
| Malformed name/body/visibility             | 400    | `MALFORMED_REQUEST`             |
| Unauthenticated                            | 401    | `UNAUTHENTICATED` (RFC-0017)    |
| Unauthorized                               | 403    | `UNAUTHORIZED` (RFC-0018)       |

`REPOSITORY_EXISTS` is the one new request-level code; the rest are inherited. A
repository in `Creating`/`Deleting` reads as `UNKNOWN_REPOSITORY` to the control
plane's `GET`/`DELETE`/`rename` (tombstones are not live).

# 9. Where the Manager Runs (layering)

```
internal/commands            (vara repo create, vara serve ...)
       │
internal/server              (data-plane + control-plane handlers, auth preamble)
       │            \
internal/repomanager  internal/transport
       │       \            │
internal/repository  internal/authz  internal/identity   ← RFC-0017/0018 layer
       │
pkg/*  (engine)              ← unchanged
```

`internal/repomanager` imports `internal/repository` (to call `Init`) and
`internal/authz` (to seed/read policy); it owns the metadata store. It sits
**above** the engine and **beside** the transport, both below `internal/server`.
It never imports `internal/server` or `internal/commands` (no upward imports), and
the engine never imports the manager — mechanically checkable by extending
`tests/architecture/imports_test.go` (§13).

# 10. Repository Name Validation & Reserved Names

Names are validated identically on the control plane and the data plane:

* one path segment: no `/`, `\`, or `..`;
* not `.`, `..`, or empty;
* charset `[A-Za-z0-9._-]+` (portable, filesystem-safe, valid `<name>.json`);
* **must not begin with `_`** (reserves `_vara`, `_server`, future control names);
* **reserved names, rejected outright:** `.vara`, `.git` (and any dotfile-looking
  name) — so a repository can never shadow an engine or tooling directory.

Two reservations are load-bearing for the future:

* The **`_` prefix** makes `/_vara/...` and `_server.json` unspoofable — no
  repository name can begin with `_`.
* The **`/` separator** is reserved for namespaces (`owner/repo`, §14). Rejecting
  it in v1 names guarantees a later namespace scheme is a pure extension, not a
  migration that has to disambiguate legacy names containing slashes.

# 11. Bootstrapping a Host

A fresh host has no `_server.json`, so `create-repo`/`list-repos` are `DenyAll` and
no repository can be created over the wire — default-deny all the way up. An
operator bootstraps by either:

* **(a)** writing `_server.json` granting a bootstrap subject `create-repo` (and
  `list-repos`); that subject then creates repositories over the API; or
* **(b)** running a local admin command (`vara repo create` against the server
  root on the host's own filesystem), which the operator is trusted to run
  directly, bypassing the wire — the on-host equivalent of `vara init`, and the
  only path that also writes the initial metadata for a hand-placed repository.

Path (b) means an operator is never locked out of their own host by an empty
policy; path (a) keeps *remote* creation default-deny. This mirrors how RFC-0018
lets an operator author policy files directly on the host.

# 12. Architectural Constraints (Normative)

* **M1 — Engine unchanged.** RFC-0019 adds nothing to `pkg/*`. Content creation is
  `repository.Init`; the manager adds no layout knowledge.
* **M2 — Single Implementation of layout.** The manager never constructs a `.vara`
  directory itself; it calls `repository.Init`. Exactly one place knows the layout
  (RFC-0003).
* **M3 — Control plane never reaches the engine for content.** Lifecycle touches
  the content tree, the policy, and the metadata; it never opens objects, refs, or
  the commit graph. Reading repository *content* remains the data-plane's job.
* **M4 — Management is authorized before it acts.** Every lifecycle effect is
  gated by RFC-0018 authorization evaluated *before* the effect, against the scope
  in §7.1 (extends RFC-0018 A2/A10).
* **M5 — Authorization reads only identity + policy.** Deciding a management
  request reads the caller's identity and the server/repository policy — never
  content and never metadata (RFC-0018 A5 preserved; ownership is a capability
  grant, not a metadata lookup).
* **M6 — Create is all-or-nothing.** An `Active` repository never exists without
  both policy and metadata; a failed create rolls back every artifact (§5.5). The
  `Creating` tombstone makes this crash-safe.
* **M7 — Delete is tombstoned and hard.** Delete flips `Deleting` first, then
  removes content, policy, metadata (§5.6); a crash resumes, never resurrects.
* **M8 — Default-deny at the server scope.** A missing `_server.json` denies all
  server-scoped operations (§7.2). Remote repository creation is default-deny.
* **M9 — Metadata and policy never mix; ownership is derived.** A policy file
  holds only capabilities; a metadata file holds only descriptive fields. No code
  branches on `subject == owner`; ownership is rendered from policy.
* **M10 — Only `Active` is served.** The data plane serves a repository iff its
  metadata state is `Active` (when a manager is configured). Tombstoned and
  archived repositories are never mistaken for live ones.
* **M11 — Immutable ID.** A repository's ID is assigned once and never changes,
  including across rename. Only the name is mutable.
* **M12 — No upward imports.** `internal/repomanager` imports only downward;
  nothing in the engine or transport imports it.

# 13. Testing Strategy

* **Lifecycle.** create → get → list → rename → delete round-trip; create-taken →
  409; delete-absent → 404; rename-onto-taken → 409; name validation rejects
  `_`-prefixed, `.git`/`.vara`, slashes, and unsafe charsets.
* **Identity/rename (M11).** After rename, the ID is unchanged and the name is new;
  a reference captured by ID before the rename still resolves after it.
* **Atomicity (M6/M7).** Inject a policy-seed failure mid-create and assert all
  artifacts rolled back (no `Active`, no orphan content, no live metadata). Inject
  a crash after the `Deleting` flip and assert a resumed delete completes and never
  resurrects the repository.
* **State gating (M10).** A `Creating` or `Deleting` repository is not served by
  the data plane (404/409) and not returned by `list`; an `Active` one is.
* **Authorization (M4/M8).** Server-scope: empty `_server.json` → remote create
  403; after granting `create-repo` → 201. Repo-scope: non-owner delete → 403 and
  the repository still exists; owner delete → 204. Rename denied unless both
  `rename-repo` (old) and `create-repo` (server) are held.
* **401 vs 403 (inherited).** Bad credential to any control-plane route → 401;
  valid identity lacking the capability → 403 — never swapped.
* **Ownership seed (M9).** After create, the owner can immediately `push` and
  `admin`; a different identity cannot, with no code path comparing to `owner`.
  Assert no metadata file contains a capability and no policy file contains a
  descriptive field.
* **Listing order (§5.8).** `list` returns names in ascending byte order,
  identically across repeated calls.
* **Architecture (M1/M12).** Extend `tests/architecture/imports_test.go`: assert
  `pkg/*` and `internal/transport` do not import `internal/repomanager`, and that
  `internal/repomanager` imports neither `internal/server` nor `internal/commands`.
  Assert (as for RFC-0017/0018) that the engine diff is empty after implementation.

# 14. Future Work

* **Namespaces / ownership paths** — `owner/repo`, nested policy roots, and the
  **id-addressed physical storage** (§5.3) that makes rename a pure metadata update
  and namespaces a clean extension.
* **Accounts & sessions (RFC-0020)** — how a human acquires the identity that
  becomes an owner; feeds RFC-0017 sources.
* **REST API (RFC-0021) & Web Dashboard (RFC-0022)** — presentation layers over
  these operations.
* **Collaboration (RFC-0023)** — issues and pull requests, referencing
  repositories by their immutable **ID** (§5.3).
* **Soft delete & archival** — activate the `Archived` state and a restorable
  delete (§5.4, §5.6); reserved capability `archive` (§7.1).
* **Public visibility semantics** — the authorization rule that makes `public`
  meaningful (§6.2).
* **Ownership transfer & multiple admins** — reassigning `owner`, `admin` for
  several subjects, team-owned repositories.
* **Filtered & ordered `list`** — return only repositories the caller can `read`;
  creation-time ordering; pagination (§5.8).
* **`vara repo` CLI family** — `create`/`delete`/`rename`/`list` over the control
  plane, plus the on-host bootstrap command (§11b).
