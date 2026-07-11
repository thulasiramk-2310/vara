VARA RFC: 0018
Title: Authorization & Repository Policy
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-12
Depends On: RFC-0016, RFC-0017
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0017 established *who* a caller is. This document decides *what that caller
is allowed to do* to a repository. It defines an authorization model, a
capability vocabulary, where policy is stored and evaluated, and the HTTP status
code for a denied-but-authenticated request.

**The one-sentence scope:**

> RFC-0018 evaluates `(subject, action, resource) → allow | deny` from a
> server-managed policy, and rejects a denied request with `403` *before* it
> reaches the transport. It answers "may this subject perform this action on this
> repository?" — nothing about who the subject is (RFC-0017) and nothing about
> how the repository stores data (the engine).

**Authorization decisions are capability-based and operation-centric: a
capability exists only if it corresponds to an observable transport operation.**
This is why the vocabulary is `read` / `create-ref` / `push` / `force-push` /
`delete-ref` (each a distinguishable operation the server can enforce) and not
`clone` / `fetch` / `pull` (client verbs that are indistinguishable on the wire —
§5.1). A protocol must never define a permission it cannot enforce.

**Design stance — policy lives above the transport, never in the engine.**

> Authorization is evaluated in the server binding, after identity is resolved
> (RFC-0017) and before any `Transport` method is invoked. The decision is a pure
> function of the caller's identity and the server's policy; it reads no
> repository content. The transport still receives exactly the same request it
> receives today — authorization only decides whether the request is *allowed to
> reach it*. By the time `Local.ReceivePack` runs, the request has already been
> authorized, and the engine never learns that authorization exists.

```
        HTTP Request
             │
             ▼
    ┌─────────────────┐
    │  Identity       │  ← RFC-0017: who is calling? (or 401)
    └─────────────────┘
             │
             ▼
    ┌─────────────────┐
    │  Authorization  │  ← RFC-0018: may they? (subject+policy → allow/deny, or 403)
    └─────────────────┘
             │  (allowed)
             ▼
        HTTPTransport handler
             │
             ▼
        Local.ReceivePack()   ← engine: authorization-agnostic, unchanged
```

# 2. Motivation

An authenticated server (RFC-0017) still lets any identity do anything — a
resolved subject is not yet a permission. To host repositories for more than one
trust level, a server must decide, per request, whether the identified caller may
perform the requested action: read the repository, push a branch, force-push,
delete a ref. RFC-0018 is the smallest layer that turns "I know who you are" into
"you may / may not do this," while keeping that decision entirely out of the
version-control engine.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **Identity** — who the caller is, and credential validation. Entirely RFC-0017.
  RFC-0018 consumes an already-resolved `Identity`; it never authenticates.
* **Roles, teams, organizations, ownership** — `admin`/`maintainer`/`owner` are
  UI/hosting concepts, not engine or protocol concepts (§5.3). RFC-0018 knows
  only capabilities.
* **Branch/ref-pattern protection** — per-branch rules ("only alice may push to
  `main`") are a natural extension but are reserved (§5.2); v1 resources are
  whole repositories.
* **Pull-request permissions, review gates, merge policies, enterprise policy** —
  Hub concerns built atop this primitive.
* **Policy administration API / UI** — how policy is *edited* is a hosting
  concern; RFC-0018 defines how it is *stored and evaluated*.
* **New engine code or new RFC-0016 message shapes** — RFC-0018 adds an
  authorization step to the existing HTTP binding and changes nothing below
  `internal/transport`.

# 4. Terminology

* **Subject** — the identity ID resolved by RFC-0017 (e.g. `alice`, or
  `anonymous`). The actor a decision is made about.
* **Action** — a member of the closed **capability** vocabulary (§5.1): what the
  caller is trying to do (`read`, `push`, `force-push`, `delete-ref`).
* **Resource** — what the action targets. In v1 this is a **repository**
  (identified by its served name). Ref-pattern resources are reserved (§5.2).
* **Capability** — a permission token granted to a subject for a resource. The
  unit of policy. Not a role.
* **Policy** — the server-managed mapping of subjects to capabilities for a
  resource (§7).
* **Decision** — the result of evaluation: `allow` or `deny`. Default is deny.

# 5. The Authorization Model

A decision is a pure function:

```
Decide(subject, action, resource, policy) -> allow | deny
```

Authorization is **capability-based**: policy grants a subject a set of
capabilities on a resource, and an action is allowed iff the subject holds the
capability that action requires. There are no roles to expand, no inheritance,
no priorities — a subject either holds a capability or does not.

## 5.1 Capability vocabulary (closed)

Capabilities are a closed enumeration, so policy files are portable and a server
cannot invent ad-hoc permissions:

| Capability | Grants | Protocol operation(s) |
|------------|--------|-----------------------|
| `read` | Discover refs and fetch objects | `GET /info/refs`, `POST /fetch` |
| `create-ref` | Create a reference that does not yet exist | `POST /receive` — update with `old` = zero |
| `push` | Fast-forward an existing reference | `POST /receive` — update, `old` non-zero, `force` = false |
| `force-push` | Move a reference non-fast-forward (rewrite history) | `POST /receive` — update with `force` = true |
| `delete-ref` | Remove a reference | `POST /receive` — deletion, `new` = zero (reserved until the transport supports deletes) |

This is the **complete, closed** v1 vocabulary; a server cannot invent tokens
outside it (§7.1).

**On client verbs.** `clone`, `fetch`, and `pull` are deliberately *not*
separate capabilities: they are **indistinguishable on the wire** — each reduces
to `GET /info/refs` + `POST /fetch` — so a server physically cannot enforce
"fetch but not clone." All three map to the single `read` capability. Write
operations, by contrast, are distinguishable by the shape of each ref update
(§8.1), so they get distinct capabilities.

**No capability implies another.** `create-ref` does not imply `push`; `push`
does not imply `create-ref`, `force-push`, or `delete-ref`; and no write
capability implies `read`. A well-formed policy granting a write capability will
normally also grant `read`, but the model never assumes it — every capability is
an explicit grant (default-deny, A4).

## 5.2 Resources (v1: repository)

In v1 the resource is the whole repository named in the request path
(RFC-0016 §8.1). A capability granted to a subject applies to every ref in that
repository. **Ref-pattern resources** — granting `push` on `refs/heads/feature/*`
but not `refs/heads/main` — are reserved for a future revision; they are the
mechanism branch protection will be built on, and are deliberately deferred so v1
stays a whole-repository model.

## 5.3 No roles

There is intentionally no `admin`, `maintainer`, `owner`, or any other role. Those
are aggregations of capabilities that belong to a hosting/UI layer, which may
present "give alice maintainer access" and expand it into a capability set when
it writes policy. The authorization engine only ever answers "does this subject
hold this capability?" — it never reasons about roles. This is the RFC-0018
analogue of RFC-0017 keeping permissions out of `Identity`.

## 5.4 Reserved extension points

The v1 decision is `(subject, action, resource) → allow | deny` over a flat
capability set. The following are **reserved, not implemented** — named now so a
later revision can add them additively (a new optional policy field, a new
capability, a negotiated capability token) without a model change:

* **Conditions** — a grant qualified by predicates rather than unconditional.
* **Time windows** — grants valid only during an interval.
* **Network constraints** — grants qualified by client IP / CIDR.
* **Organization / group scoping** — capabilities granted to a group of subjects.
* **Ref-pattern / branch rules** — per-branch capabilities (§5.2), the basis for
  branch protection.

v1 evaluates none of these; a v1 policy file expresses only unconditional
subject→capability grants (§7.1).

# 6. Where Authorization Runs

The canonical per-request pipeline is fixed (A10):

```
Request → Authenticate → Authorize → Transport → Engine
          (RFC-0017)     (RFC-0018)   (RFC-0016)  (unchanged)
```

The authorization check runs in the server binding, in the request preamble,
positioned exactly between identity resolution (RFC-0017) and opening the
transport (RFC-0016 §9). The sequence per request:

1. **Authenticate** → resolve `Identity` (RFC-0017), or `401`.
2. **Determine the required capability** from the request: the endpoint, and for
   `receive`, each ref update's shape (§8.1).
3. **Load policy** for the target repository from the server policy store (§7).
4. **Decide** `Decide(subject, action, resource, policy)` for the required
   capability (for `receive`, for *every* update's required capability).
5. On `allow`, proceed to open the transport and run the operation. On `deny`,
   reject with `403` (§8.2) — **the transport is never invoked**.

Because the decision is made before `transport.OpenLocal`, a denied request never
touches the repository's object store, ref store, or lock — it is rejected at the
edge.

## 6.1 Request-level, all-or-nothing (v1)

A `receive` may carry several ref updates requiring different capabilities. In v1
authorization is **request-level**: the server computes the set of capabilities
the whole request requires and denies (`403`) if the subject lacks any of them —
before the transport applies anything. This is default-deny and simple: a mixed
batch is authorized as a unit.

Per-ref authorization (allow some updates in a batch, deny others, mirroring
RFC-0016 §7.4's per-ref results) is a future refinement. v1's all-or-nothing rule
never partially applies a batch on authorization grounds, which is the safe
default.

# 7. Policy Storage

This is the load-bearing decision of RFC-0018.

**Policy is a server-managed store, outside the repository.** It is not stored in
`.vara`, is never part of a repository's history, and is never transferred by
clone/fetch/push. A repository holds version-control data only; the hosting layer
owns permissions.

```
<served-root>/
    demo/              ← a served repository (RFC-0016 --root)
        .vara/         ← version-control data ONLY; no policy here
    project/
        .vara/

<policy-root>/
    demo.json          ← policy for repository "demo"
    project.json       ← policy for repository "project"
```

Why outside the repository (rejecting the two alternatives):

* **Not `.vara/policy` (in-repo).** If policy lived in the repository, a `push`
  could rewrite the very rules that authorize pushing — a privilege-escalation
  hazard and a layering violation (the engine would carry authorization data).
  Portability is not worth that.
* **Not an opaque in-process table only.** A file-backed store per repository is
  inspectable, backup-able, and server-owned, and it maps cleanly onto a future
  database in the Hub without changing the model.

The `<policy-root>` location is server configuration (e.g. a `vara serve`
`--policy` flag); a server with no policy store configured is the anonymous
RFC-0016 server (everyone may do everything — trusted-network mode).

## 7.1 Policy file format

One JSON file per repository, mapping subjects to capability sets. `anonymous` is
an ordinary subject key (consistent with RFC-0017 treating anonymous as a real
identity):

```jsonc
{
  "version": 1,
  "subjects": {
    "anonymous": ["read"],
    "alice":     ["read", "create-ref", "push"],
    "bob":       ["read", "create-ref", "push", "force-push", "delete-ref"]
  }
}
```

* A subject's capabilities are exactly those listed. There is no inheritance.
* A subject **not listed** has **no capabilities** (default deny, §5/A4).
* An **unknown capability string invalidates the policy** (fail-fast). A policy
  file containing a token outside the closed vocabulary (§5.1) is a configuration
  error: the server MUST refuse to load it and MUST NOT serve the affected
  repository with a partially-understood policy. Because an invalid policy is not
  a grant, its net effect is default-deny (A4) — the server fails **closed**,
  never open. Rationale: policy is server-owned config, not a forward-compat wire
  message; silently ignoring an unrecognized capability could mask a typo
  (`push` mistyped as `psuh`) that a fail-fast load surfaces immediately.

## 7.2 Policy ownership and loading

Exactly one component reads and parses policy: **the authorization layer in the
server binding**. Neither the engine nor the transport ever touches it (A1, A9):

```
Policy store (files)
        │  load + parse (authorization layer only)
        ▼
Immutable parsed policy
        │  Decide(subject, action, resource)
        ▼
allow / deny
```

The engine never loads policy; the transport never parses policy; the decision is
made from an **immutable, already-parsed** policy value so a request cannot
observe a half-written file.

## 7.3 Caching and atomic reload

The authorization layer MAY cache parsed policy in memory rather than re-reading
a file per request. If it does, **reload MUST be atomic**: a policy update is
applied by swapping in a fully-parsed, validated replacement in one step, so no
request ever evaluates against a partially-updated policy. A reload that fails to
parse or validate (§7.1) MUST leave the previously-loaded policy in force and log
the error — a broken edit never silently opens or empties access. Per-request
consistency is required: a single request evaluates against one policy snapshot.

# 8. HTTP Binding

## 8.1 Mapping operations to required capabilities

The binding derives the required capability from the request:

* `GET /:repo/info/refs` → `read`
* `POST /:repo/fetch` → `read`
* `POST /:repo/receive` → for each `RefUpdate`, from the update's shape alone
  (no repository state, A5):
  * `new` = zero (deletion, reserved) → `delete-ref`
  * `old` = zero (the ref does not yet exist) → `create-ref`
  * `force` = true → `force-push`
  * otherwise (existing ref, `force` = false) → `push`

Every determination above uses only fields present in the request — `old`, `new`,
and `force` — never the commit graph. Determining whether a `force = false`
update is *actually* a fast-forward is the transport's job (RFC-0016 §7): if it is
not, the transport rejects it (`ok:false, NON_FAST_FORWARD`) — a *rejection*, not
a privilege escalation. Authorization gates on **declared intent** (`force = true`
is the intent to rewrite, and `force-push` gates exactly that), so it never needs
to consult repository state (A5).

## 8.2 Status code and error

RFC-0018 adds one status code:

| Code | When | Notes |
|------|------|-------|
| `403 Forbidden` | The subject is authenticated but lacks a required capability. | **Authorization** failure. Distinct from `401` (authentication). |

The body reuses the RFC-0016 §8.6 structured error schema:

```jsonc
{
  "ok": false,
  "code": "UNAUTHORIZED",
  "message": "subject 'alice' lacks capability 'force-push' on repository 'demo'",
  "details": { "action": "force-push", "resource": "demo" }
}
```

`UNAUTHORIZED` is the single new request-level error code. The `401`/`403` split
is strict: `401 UNAUTHENTICATED` means "I don't know who you are"; `403
UNAUTHORIZED` means "I know who you are and you may not." A server MUST NOT use
`403` for an authentication failure or `401` for an authorization failure.

## 8.3 Capability advertisement

A server MAY advertise, via the RFC-0016 §5.4 capability list, that it enforces
authorization (`authz-v1`), so a client can present a credential proactively
rather than after a `403`. This is advisory; the enforcement is server-side
regardless.

# 9. Client Behavior

* On `403`, the client surfaces the authorization failure with the action and
  resource from the error body. Unlike `401`, retrying with the *same* identity
  is pointless — the subject genuinely lacks the capability — so the client MUST
  NOT auto-retry a `403`; the remedy is different credentials or a policy change.
* A client cannot discover its own capabilities in v1 beyond attempting an
  operation; a capability-introspection endpoint is future work.

# 10. Architectural Constraints (Normative)

These make §1's stance enforceable and are binding on every implementation. They
are the authorization analogues of RFC-0017's C-series.

* **A1 — The engine is authorization-agnostic.** No package at or below
  `internal/transport` may import the policy store, read a policy file, or branch
  on a capability. Code such as `if subject.CanPush()` or `if policy.Allows(...)`
  MUST NOT appear in the engine or in `Local`. Authorization is confined to the
  server binding (`internal/server`), above `internal/transport`.
* **A2 — Authorization precedes the transport.** The decision is made before any
  `Transport` method is invoked for a request. A denied request never reaches
  `Local` and never touches the repository's store, refs, or lock.
* **A3 — Policy lives outside the repository.** The policy store is server-managed
  and is never stored in `.vara`, never part of history, never transferred by
  clone/fetch/push. A push can never alter who may push (§7).
* **A4 — Default deny (fundamental).** If no policy explicitly grants the
  required capability, authorization MUST deny the request. There is no fallback
  to "allow" under any condition — a missing policy file, an unlisted subject, an
  empty capability list, or a policy that failed to load (§7.1) all resolve to
  denial. The server fails **closed**.
* **A5 — Decisions read only identity and policy.** An authorization decision is
  a pure function of the resolved `Identity` and the policy; it MUST NOT read
  repository content (objects, refs, working tree, commit graph). Fast-forward vs.
  non-fast-forward is expressed by the request's declared `force` intent, not by
  consulting the graph (§8.1). This is the A-analogue of RFC-0017 C6 and keeps
  authorization deterministic and independent of repository state.
* **A6 — Capability-based, not role-based.** The vocabulary is a closed capability
  set (§5.1). Roles (`admin`, `maintainer`, `owner`) are neither engine nor
  protocol concepts; they may exist only as a hosting-layer convenience that
  expands into capabilities when policy is written.
* **A7 — Single Implementation Principle preserved (RFC-0016 §9.1).** Adding
  authorization does not let the binding reimplement repository semantics. After
  an `allow`, the handler still delegates the mutation to `Local.ReceivePack`
  unchanged. Authorization wraps the codec; it does not replace it.
* **A8 — Additive to the wire.** RFC-0018 changes no RFC-0016/0017 message shape.
  A conforming client remains conforming; it merely may receive `403` where it
  received `200`.
* **A9 — Only the authorization layer owns policy.** Exactly one component loads
  and parses policy: the authorization layer in the server binding (§7.2). No
  other component — engine, transport, request logger — reads, parses, or caches
  policy. Decisions evaluate against an immutable, already-parsed snapshot, and
  any reload is atomic (§7.3).
* **A10 — Fixed pipeline order.** For every request the order is
  **Authenticate → Authorize → Transport → Engine**. Authentication (RFC-0017)
  always precedes authorization; authorization always precedes any `Transport`
  method; the engine runs last and is unaware of both. No stage may be skipped or
  reordered: a request that fails authentication is never authorized, and one
  that fails authorization never reaches the transport.

# 11. Security Considerations

* **Default deny is the safe posture.** Every gap in policy resolves to denial
  (A4), so a misconfigured or missing policy file fails closed, never open.
* **Policy is server-owned and tamper-resistant by placement.** Because policy is
  outside the repository (A3), a client with `push` cannot escalate by editing
  policy — there is no in-band path to the rules. Protecting the policy store on
  disk is a deployment concern (filesystem permissions), like protecting any
  server secret.
* **Anonymous is governed by the same policy.** `anonymous` is an ordinary
  subject (§7.1), so "may anonymous read?" is a normal policy question with a
  default-deny answer — not a special code path.
* **Audit logging.** Authorization is the right layer to audit. The server SHOULD
  log **every** decision, allow and deny alike:
  * `ALLOW subject=<id> action=<cap> resource=<repo> txn=<id>`
  * `DENY  subject=<id> action=<cap> resource=<repo> reason=<...> txn=<id>`

  Logging allows as well as denies gives a complete access trail, not just a
  failure log. As in RFC-0017 §11, the log MUST NOT contain credentials
  (passwords, tokens, the `Authorization` header); the subject ID and request ID
  are the safe correlation keys.
* **No capability disclosure by error.** A `403` names the action and resource
  the caller attempted; it MUST NOT enumerate the subject's *other* capabilities
  or the policy of other subjects.
* **Authentication precedes authorization.** A request with a bad credential is a
  `401` (RFC-0017) and is never evaluated for authorization, so policy evaluation
  only ever runs on a validated identity.

# 12. Testing Strategy

* **Allow path.** A subject with the required capability proceeds; the transport
  is invoked and the operation succeeds — identical to an unauthenticated server.
* **Deny path.** A subject lacking the capability gets `403 UNAUTHORIZED`, and
  **no `Transport` method is invoked** (assert with a spy transport). One case per
  capability: read-denied, push-denied, force-push-denied, delete-ref-denied.
* **Default deny.** An unlisted subject, and a repository with no policy file, are
  denied every action.
* **401 vs 403.** A bad credential yields `401` (never `403`); a valid credential
  lacking permission yields `403` (never `401`); the two are never swapped.
* **push ≠ force-push.** A subject with `push` but not `force-push` is allowed a
  fast-forward-intent update (`force=false`) and denied a `force=true` update.
* **create-ref ≠ push.** A subject with `push` but not `create-ref` is denied a
  new-ref update (`old=zero`) and allowed a fast-forward of an existing ref, and
  vice versa — the two are distinguished from update shape alone (§8.1).
* **Unknown capability fails closed.** A policy file containing a token outside
  the closed vocabulary is rejected at load (§7.1); the affected repository is
  served with no effective grants (default-deny, A4), never with a partial
  policy, and the load error is logged.
* **Atomic reload.** A broken policy edit leaves the previously-loaded policy in
  force (§7.3); no request observes a half-updated policy, and a valid reload is
  visible to subsequent requests as a single atomic swap.
* **A1/A5 enforcement.** A build/lint or import test confirms no engine package
  imports the policy store; a decision test confirms evaluation reads no
  repository content (a decision resolves identically whether or not the target
  repo exists on disk, mirroring RFC-0017 C6).
* **A3 non-transfer.** A clone/fetch/push carries no policy data; the policy store
  is untouched by any transport operation.
* **Anonymous via policy.** `anonymous` granted `read` may clone; not granted
  `push` is denied a push — governed entirely by the policy file.

# 13. Future Work

* **Ref-pattern resources** — per-branch/per-ref capabilities (§5.2), the basis
  for branch protection.
* **Per-ref authorization in a batch** — allow/deny individual updates in a
  `receive` (§6.1), mirroring RFC-0016 §7.4 per-ref results.
* **Capability introspection** — an endpoint letting a client learn its own
  capabilities without trial requests (§9).
* **Group/role expansion** — a hosting-layer mapping of roles to capabilities
  written into policy (never into the engine, A6).
* **Policy administration** — an API/UI and, in the Hub, a database-backed policy
  store replacing the file store without model changes.
* **VARA Hub** — repository hosting building on identity (RFC-0017) and
  authorization (RFC-0018) as its access-control foundation.
