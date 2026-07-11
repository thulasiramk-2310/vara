VARA RFC: 0017
Title: Identity & Authentication
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-12
Depends On: RFC-0016
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0016 defined an anonymous remote transport: any client can clone, fetch, and
push. This document adds the ability for a server to learn **who** is making a
request and to reject requests it cannot identify. It defines nothing about what
an identified caller is *allowed* to do — that is RFC-0018 (Authorization).

**The one-sentence scope:**

> RFC-0017 answers "who is calling, how did they present that identity, and was
> it valid?" and assigns HTTP status codes to authentication failure. It does
> not answer "may this caller do this operation?"

**Design stance — identity terminates above the transport.**

> Authentication happens in the HTTP binding layer, before any `Transport`
> method is invoked. By the time `Local.ReceivePack` (or any engine code) runs,
> the caller's identity has already been established or the request has already
> been rejected. The engine never learns *how* a caller authenticated, and never
> makes a decision based on identity. Identity is strictly a property of the
> binding, not of the repository.

```
        HTTP Request
             │
             ▼
    ┌─────────────────┐
    │ Authentication  │  ← RFC-0017: extract + validate credential
    └─────────────────┘
             │
             ▼
        Identity (or 401)
             │
             ▼
    ┌─────────────────┐
    │  Authorization  │  ← RFC-0018 (future): may this identity do this?
    └─────────────────┘
             │
             ▼
        HTTPTransport handler
             │
             ▼
        Local.ReceivePack()   ← engine: identity-agnostic, unchanged
```

# 2. Motivation

An anonymous write endpoint (RFC-0016 §10) is safe only on a trusted network.
To operate on a shared or public network, a server must at minimum be able to:

1. Require that a caller present a credential.
2. Extract that credential from the request.
3. Validate it against a configured identity source.
4. Reject an unidentifiable caller with a standard, machine-readable failure.

That is the whole of RFC-0017. It is deliberately the smallest layer that makes
a server non-anonymous, so that authorization (RFC-0018) and hosting (Hub) build
on a stable identity primitive rather than reinventing one.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **Authorization** — whether an identity may clone/push/force-push/administer a
  repository. Entirely RFC-0018. RFC-0017 produces an identity; it never
  branches on one.
* **User accounts, registration, org membership, teams** — Hub concerns.
* **Credential issuance / token minting / password storage policy** — an
  identity *source* is pluggable (§7); this RFC defines how a credential is
  *presented and validated at the edge*, not how it is created or stored.
* **Transport encryption** — TLS remains terminated by front-end infrastructure
  (RFC-0016 §10). Credentials are bearer secrets and therefore MUST travel over
  TLS in any real deployment, but TLS itself is not specified here.
* **New wire messages or engine changes** — RFC-0017 adds an authentication step
  to the existing HTTP binding. It does not alter the §5 messages of RFC-0016,
  and it changes no code below `internal/transport`.

# 4. Terminology

* **Credential** — the secret a client presents to prove identity (a password, a
  token). Opaque to the transport; meaningful only to the identity source.
* **Identity** — the validated principal a credential resolves to: a stable
  **ID** string (e.g. a username) plus the **method** used (§7.1). It carries no
  permissions, roles, or groups — those are RFC-0018.
* **Anonymous** — the absence of a credential, resolving to the reserved
  identity `anonymous`.
* **Identity source** — the pluggable component that validates a credential and
  returns an identity (or rejects it). Examples: a static credential file, an
  in-memory map, a future OIDC verifier.
* **Authentication failure** — a credential was presented but is invalid or
  unparseable. Distinct from *authorization* failure (a valid identity lacking
  permission), which is RFC-0018.

# 5. Identity Methods

Identity methods are a **closed enumeration** (`IdentityMethod`, §7.2), not free
strings — an implementation cannot introduce an ad-hoc variant. v1 defines three
active methods and reserves four.

## 5.1 v1 methods

A v1 server MUST support at least `anonymous` and SHOULD support both credential
methods below. Which are *accepted* for a given server is configuration
(RFC-0017 does not mandate that any particular method be enabled).

* **Anonymous** — no `Authorization` header. Resolves to the identity
  `anonymous`. A server MAY accept anonymous for reads and reject it for writes;
  that read/write distinction is authorization (RFC-0018), so at the RFC-0017
  layer anonymous is simply *an identity*, not *a permission*.
* **Basic** — `Authorization: Basic base64(user:secret)` (RFC 7617). The
  identity source validates `user`/`secret`; the resolved subject is `user`.
* **Bearer** — `Authorization: Bearer <token>` (RFC 6750). The identity source
  validates the opaque token and resolves it to a subject.

## 5.2 Reserved methods

Reserved so their introduction is additive (a new accepted `Authorization`
scheme or a capability token), never a breaking change:

* **OAuth2** / **OpenID Connect (OIDC)** — federated bearer tokens with
  server-side verification of an issuer's signature.
* **Mutual TLS (mTLS)** — client-certificate identity established at the TLS
  layer and surfaced to the binding.
* **SSH** — identity over an SSH transport binding (a different §8-equivalent
  binding, not HTTP).

A server advertises which methods it accepts via the capability mechanism
(§8.3); a client that does not understand a method simply does not attempt it.

# 6. The Authentication Step (HTTP Binding)

Authentication is a step the HTTP binding performs **before** dispatching to a
`Transport` method. In server terms (RFC-0016 §9) it happens inside the request
handler's preamble — the same place `open()` validates the protocol version and
repository name — and strictly before `transport.OpenLocal` is used to serve the
request.

For each request the server:

1. **Extracts** the credential from the `Authorization` header (absent → the
   anonymous credential).
2. **Validates** it against the configured identity source (§7).
3. On success, **attaches** the resolved identity to the request context and
   proceeds to the operation (and then, in a future RFC-0018 server, to an
   authorization check).
4. On failure (a credential was presented but is invalid/unparseable), **rejects**
   the request with `401 Unauthorized` and a structured error (§8.2), before any
   `Transport` method runs.

The identity, once resolved, is **not passed to the engine**. It lives in the
binding's request context, available to an RFC-0018 authorization check, and is
discarded when the request completes. `Local.ListRefs/FetchPack/ReceivePack`
receive exactly the arguments they receive today.

## 6.1 Anonymous vs. rejected

These are different and MUST NOT be conflated:

* **No credential** → identity `anonymous`, request proceeds to the operation.
  Whether `anonymous` may perform the operation is RFC-0018's call; a pure
  RFC-0017 server that requires identity for *all* access returns `401` for a
  missing credential, while one that permits anonymous access proceeds.
* **Bad credential** → `401`, always. Presenting a broken credential is never
  silently downgraded to anonymous — that would let an attacker probe as
  anonymous after a failed auth. A malformed `Authorization` header is a `401`,
  not a fallthrough.

## 6.2 Parsing precedes authentication

Header **parsing** and credential **authentication** are two distinct steps, and
a malformed header MUST NOT reach the identity source:

1. **Parse** the `Authorization` header into a `Credential{Scheme, Value}`
   (absent header → the anonymous credential). A syntactically invalid header —
   unknown scheme, non-base64 Basic payload, missing token — fails here.
2. **Authenticate** the parsed credential via the identity source (§7.3).

A parse failure is a `401` (`UNAUTHENTICATED`) that never calls
`Authenticate` — the identity source only ever sees well-formed credentials.
This keeps the source simple (it validates meaning, not syntax) and avoids
feeding attacker-controlled malformed input into credential validation.

## 6.3 Identity is request-scoped

A resolved identity lives for exactly **one request** and is discarded when that
request completes. It is not cached across requests, not bound to a connection,
and not tied to a session. Every request re-authenticates from its own
`Authorization` header. This keeps the server stateless — it works unchanged
behind a load balancer or across multiple `vara serve` processes — and means a
revoked credential stops working on the very next request, with no session to
invalidate.

# 7. Identity Types & Sources

## 7.1 The Identity object

An identity is intentionally tiny in v1 — a stable ID and the method that
produced it, and **nothing permission-bearing**:

```
type Identity struct {
    ID     string          // stable subject, e.g. a username; "anonymous" for no credential
    Method IdentityMethod  // how the ID was established
}
```

It carries no permissions, roles, groups, or repository access. Those are
RFC-0018's data, and putting them here would leak authorization into the
identity layer. If v1's `Identity` is ever extended, it may gain only
identity-descriptive fields (e.g. a display name), never permission fields.

## 7.2 IdentityMethod enum

Methods are a **closed enumeration**, not free strings, so no implementation can
invent ad-hoc variants:

```
type IdentityMethod int

const (
    MethodAnonymous IdentityMethod = iota  // no credential presented
    MethodBasic                            // HTTP Basic (RFC 7617)
    MethodBearer                           // Bearer token (RFC 6750)

    // Reserved — not accepted in v1 (§5.2):
    MethodOAuth2
    MethodOIDC
    MethodMutualTLS
    MethodSSH
)
```

## 7.3 The IdentitySource interface

An **identity source** is the pluggable boundary between "a credential arrived"
and "here is who it is." It has a single responsibility:

```
IdentitySource interface {
    // Authenticate validates an already-parsed credential and returns the
    // resolved identity. A nil credential means "anonymous". It returns an
    // error ONLY for a presented-but-invalid credential (→ 401); it MUST NOT
    // return an error for a valid credential that merely lacks permissions
    // (that is RFC-0018), and it MUST NOT read repository state (C6).
    Authenticate(cred *Credential) (Identity, error)
}
```

* `Credential` carries the parsed scheme + value (§6.2); it is opaque to the
  transport and is produced by header parsing, not by the source itself.
* v1 ships at least one concrete source (e.g. a static credential file mapping
  users/tokens to IDs). OIDC/mTLS sources are future implementations of the same
  interface.

The identity source lives **in the server binding**, not in the engine and not
in `internal/transport`'s `Local`. This placement is what keeps the architectural
rule of §1 enforceable: there is no code path from the engine to an identity
source, because the engine cannot import the binding. Symmetrically, the source
does not reach *into* the engine — authentication never depends on repository
contents (C6), so it is well-defined even for a request naming a repository that
does not exist.

# 8. HTTP Binding

## 8.1 The Authorization header

Identity is presented in the reserved `Authorization` header (RFC-0016 §8.3
reserved it for exactly this). The value follows standard HTTP auth syntax:
`Basic <base64>` or `Bearer <token>`. Its absence is the anonymous credential.

On a `401`, the server SHOULD return a `WWW-Authenticate` header advertising the
accepted schemes, so standard HTTP clients (browsers, `curl`, libraries) can
respond without out-of-band knowledge:

```
WWW-Authenticate: Basic realm="VARA"
WWW-Authenticate: Bearer
```

Multiple accepted schemes MAY be combined in a single header
(`Basic realm="VARA", Bearer`). The advertised schemes MUST match the server's
accepted-method capability set (§8.3).

## 8.2 Status codes

RFC-0017 adds exactly one status code to the RFC-0016 set, plus its structured
body:

| Code | When | Notes |
|------|------|-------|
| `401 Unauthorized` | A presented credential is invalid/unparseable, or the server requires identity and none was presented. | Carries `WWW-Authenticate`. **Authentication** failure. |

`403 Forbidden` is **reserved for RFC-0018** (a valid identity lacking
permission) and MUST NOT be used by an RFC-0017 server for authentication
failure — keeping the "who are you" failure (`401`) crisply distinct from the
"you may not" failure (`403`).

The `401` body reuses the RFC-0016 §8.6 structured error schema:

```jsonc
{
  "ok": false,
  "code": "UNAUTHENTICATED",
  "message": "invalid bearer token",
  "details": {}
}
```

`UNAUTHENTICATED` is the single new request-level error code. As with all codes
(RFC-0016 §8.6), a client MUST treat an unknown code as a generic failure of its
status class.

## 8.3 Capability advertisement

A server advertises the identity methods it accepts through the RFC-0016 §5.4
capability list on `info/refs`, so a client can choose a method it supports:

| Capability | Meaning |
|------------|---------|
| `auth-anonymous` | Anonymous access is accepted (for at least some operations). |
| `auth-basic` | HTTP Basic is accepted. |
| `auth-bearer` | Bearer tokens are accepted. |

A server advertising none of the `auth-*` capabilities is an anonymous v1 server
(RFC-0016 behavior). These tokens are additive: an RFC-0016 client that ignores
them still functions against an anonymous server, and a `401` teaches it that
credentials are required where it previously expected `200`.

## 8.4 Reserved headers

RFC-0017 populates the previously-reserved `Authorization` header (RFC-0016
§8.3) and introduces no new headers. `X-VARA-Transaction` continues to correlate
requests, now including failed-auth attempts, which is useful for audit.

# 9. Client Behavior

The command layer gains the ability to *present* a credential, not to make
identity decisions:

* A client MAY attach a credential to its `HTTPTransport` (from configuration or
  a prompt); the transport sets the `Authorization` header on every request.
* On a `401`, the client surfaces the authentication failure to the user (and
  MAY consult `WWW-Authenticate` to choose a method), then retries with a
  credential. It MUST NOT silently retry the same failed credential.
* An anonymous client (no credential) is unchanged from RFC-0016; it simply
  receives `401` instead of `200` from a server that requires identity.

Credential storage/config format is an implementation detail (a later addition
to `pkg/config`, e.g. per-remote credentials), not a protocol concern.

# 10. Architectural Constraints (Normative)

These make §1's stance enforceable and are binding on every implementation:

* **C1 — The engine is identity-agnostic.** No package at or below
  `internal/transport` may import an identity source, inspect an
  `Authorization` header, or branch on a caller's identity. Code such as
  `if identity.IsAdmin()` or `if token.Valid()` MUST NOT appear in the engine or
  in `Local`. Authentication is confined to the server binding
  (`internal/server`, above `internal/transport`).
* **C2 — Authentication precedes the transport.** The auth step runs before any
  `Transport` method is invoked for a request. A request that fails
  authentication never reaches `Local`.
* **C3 — Identity ≠ authorization.** An `IdentitySource` returns an error only
  for an invalid credential. It never decides permissions; a valid identity with
  no rights is a successful authentication (RFC-0018 then denies the operation
  with `403`).
* **C4 — Single Implementation Principle preserved (RFC-0016 §9.1).** Adding
  authentication does not let the binding reimplement repository semantics: after
  the auth step, the handler still delegates the mutation to
  `Local.ReceivePack` unchanged. Auth wraps the codec; it does not replace it.
* **C5 — Additive to the wire.** RFC-0017 changes no RFC-0016 message shape. A
  conforming RFC-0016 client remains a conforming client of an RFC-0017 server;
  it merely may receive `401` where it received `200`.
* **C6 — Authentication is repository-agnostic.** An `IdentitySource` MUST NOT
  read repository state — no objects, refs, config, or working tree. Identity is
  a function of the credential alone, so authentication is well-defined even for
  a request naming a nonexistent repository, and no attacker-influenced
  repository content can affect who a caller is authenticated as.
* **C7 — Identity is request-scoped.** A resolved identity lives for one request
  and is not cached, connection-bound, or sessioned (§6.3). Every request
  re-authenticates, keeping the server stateless and revocation immediate.

# 11. Security Considerations

* **Credentials are bearer secrets.** Basic and Bearer both send a reusable
  secret; they MUST be used over TLS (terminated by front-end infrastructure,
  RFC-0016 §10). RFC-0017 does not add transport encryption.
* **No downgrade on failure.** A bad credential is `401`, never a silent
  fallthrough to anonymous (§6.1), so a failed auth cannot be used to probe
  anonymous access.
* **Constant-time comparison.** An identity source validating a secret (a Basic
  password, a Bearer token) SHOULD compare in constant time (e.g.
  `crypto/subtle.ConstantTimeCompare`) to avoid leaking the secret through a
  comparison-timing oracle. Recommended, not required, for v1.
* **Timing.** Credential validation SHOULD avoid obvious timing leaks where
  practical — a wrong username and a wrong password should not be trivially
  distinguishable by response time. Best-effort in v1.
* **Credential logging is forbidden.** The request logger (RFC-0016 `vara
  serve`) and any audit log MUST NOT log passwords or bearer tokens, nor the raw
  `Authorization` header. It MAY log the identity **method**, the resolved
  identity **ID**, and the **request ID** (`X-VARA-Transaction`) — enough for
  audit and correlation without recording a reusable secret.
* **Anonymous remains a valid identity, not a bypass.** Treating `anonymous` as
  an ordinary identity (subject `anonymous`) means RFC-0018 can write a single
  uniform policy over all identities, including the anonymous one, rather than a
  special case.
* **Replay of a captured credential** is an authentication concern this RFC does
  not fully solve for reusable secrets (a stolen token works until revoked);
  short-lived/rotated tokens (OIDC, reserved §5.2) are the mitigation path.

# 12. Testing Strategy

* **Anonymous unchanged.** A server with no `auth-*` capabilities behaves exactly
  as the RFC-0016 suite expects (regression: run that suite against an
  identity-enabled-but-anonymous-permitted server → identical results).
* **Basic / Bearer round-trip.** A valid credential resolves to the expected
  subject and the request proceeds; the resolved identity is present in the
  handler context and absent from anything the engine sees.
* **Rejection.** A malformed header, an unknown scheme, a wrong password, and an
  invalid token each yield `401` + `UNAUTHENTICATED` + `WWW-Authenticate`, and
  **no `Transport` method is invoked** (assert via a spy/transport that records
  calls).
* **No downgrade.** A bad credential does not fall through to anonymous access.
* **Parse vs authenticate.** A malformed `Authorization` header yields `401`
  without the identity source's `Authenticate` ever being called (assert with a
  spy source that records invocations) — parsing rejects it first (§6.2).
* **Repository-agnostic (C6).** Authentication against a request naming a
  nonexistent repository still resolves identity (or `401`) purely from the
  credential; the identity source is verified never to touch repository state.
* **Request-scoped (C7).** Two sequential requests each re-authenticate; a
  credential accepted on request 1 and revoked before request 2 is rejected on
  request 2 with no session to invalidate.
* **C1 enforcement.** A build/lint check (or a test importing the engine
  packages) confirms no engine package imports the identity source — the
  architectural rule is mechanically verified, not just documented.
* **No credential in logs.** A test scanning captured server logs asserts no
  password/token/`Authorization` value appears, only method/ID/request-ID.
* **Capability negotiation.** `info/refs` advertises the accepted `auth-*`
  methods; a client selects an accepted one.

# 13. Future Work

* **RFC-0018 Authorization** — per-repository read/write/admin decisions over the
  identities RFC-0017 produces; introduces `403`.
* **Token/credential configuration** — per-remote credentials in `pkg/config`.
* **Federated identity** — OAuth2/OIDC identity sources (reserved §5.2).
* **mTLS and SSH bindings** — identity established at the transport layer.
* **Revocation & rotation** — short-lived tokens and a revocation path, reducing
  the reusable-secret replay window (§11).
