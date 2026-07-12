VARA RFC: 0021
Title: Hub Read & Management API
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-12
Depends On: RFC-0016, RFC-0017, RFC-0018, RFC-0019, RFC-0020
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0019 and RFC-0020 gave VARA a control plane for *managing* repositories and
accounts. Neither lets a client *read repository content* over HTTP without
cloning: there is no way to ask "what are this repo's branches?" or "show me the
last 30 commits" short of fetching a pack and walking it locally. A web UI cannot
do that. This document defines the small **read API** the VARA Hub needs — branch
and commit listings and commit detail — plus how the Hub is **served** (same-origin
static files from `vara serve`) and how a **browser** authenticates safely. It is
the contract the Hub frontend is built against.

**The one-sentence scope:**

> RFC-0021 adds read-only content endpoints (`branches`, `commits`, `commit`
> detail) under the existing `/_vara/repositories/{repo}` control plane,
> authorized by the RFC-0018 `read` capability; serves a same-origin static Hub
> UI from `vara serve --hub`; and lets a browser hold its session as an httpOnly
> cookie so the secret never touches JavaScript. It answers "how does a UI read a
> repository and log a user in safely?" — it defines no new mutation (writes stay
> RFC-0019/0020) and no engine behavior.

**Design stance — read the engine, never reimplement it, never change it.**

> The read endpoints are a thin projection over the existing engine primitives —
> `refs` for branches, the commit graph for history order, the object store for
> commit detail. The server reads; it does not re-derive traversal or storage
> logic (Single Implementation Principle, RFC-0016 §9.1), and it adds nothing to
> `pkg/*`. This RFC is proven, like the five before it, by an unchanged engine.

```
        Browser (Hub UI, served same-origin)
             │  fetch /_vara/repositories/demo/commits
             ▼
   ┌───────────────────────────┐
   │ Authenticate (RFC-0017)   │  cookie OR bearer → identity (or 401)
   └───────────────────────────┘
             │
   ┌───────────────────────────┐
   │ Authorize: read (RFC-0018)│  before any content is read (or 403)
   └───────────────────────────┘
             │
   ┌───────────────────────────┐
   │ Read via engine primitives│  refs · graph · object store — no reimpl
   └───────────────────────────┘
             │
        JSON  ← projection of engine reads → rendered by the UI
```

# 2. Motivation

The Hub v0.1 needs six views: login, a repository dashboard, a repository
overview, commit history, branches, and settings. Five of the six already have
endpoints (sessions, repository list/detail, rename/delete, tokens). **History and
branches do not** — and a browser cannot clone-and-walk to synthesize them. These
two read endpoints are exactly the "the frontend genuinely needs it" bar for new
backend work. Alongside them the Hub needs to be *served* and a browser needs to
*log in* without exposing a bearer secret to XSS. RFC-0021 supplies precisely
those three things and nothing more.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **File browser, file contents, diffs, blame, search.** The v0.1 Hub lists
  history and branches; a tree/blob/diff API is future work (§14). This RFC does
  not expose blob contents.
* **New mutations.** All writes remain the RFC-0019 (repository lifecycle) and
  RFC-0020 (accounts/sessions/tokens) control planes. RFC-0021 is read + serving +
  browser-auth only.
* **The frontend itself.** The Hub UI (its framework, components, styling) is an
  application built on this API, not part of the spec.
* **Server-side rendering, GraphQL, websockets/live updates.** v1 is plain JSON
  over the existing request model.
* **Cross-origin as the default.** The Hub is served same-origin; cross-origin
  CORS is an opt-in for development (§9), never the production posture.
* **Engine / transport changes.** RFC-0021 adds no field to `pkg/*` and no message
  to the RFC-0016 wire; it reads existing objects through existing APIs.

# 4. Terminology

* **Read API** — the RFC-0021 JSON endpoints that project repository content
  (branches, commits) read-only, under `/_vara/repositories/{repo}/…`.
* **Hub UI** — the static web application served same-origin by `vara serve
  --hub`; a client of this API, not part of it.
* **Browser session** — a session (RFC-0020 §5.2) whose secret is carried in an
  **httpOnly** cookie rather than an `Authorization` header, so page JavaScript
  can never read it.
* **Same-origin** — the Hub UI and the API share scheme+host+port, so the browser
  sends the session cookie automatically and no CORS is involved.

# 5. The Read API

All read endpoints live under the existing repository path and require the
RFC-0018 `read` capability (§6). They read content through engine primitives and
never reimplement traversal or storage (§5.5).

## 5.1 Repository overview

`GET /_vara/repositories/{repo}` already exists (RFC-0019 §8.1) and returns the
descriptor (id, name, owner, visibility, state, timestamps). The Hub overview page
reuses it unchanged; RFC-0021 adds nothing here.

## 5.2 Branches

```
GET /_vara/repositories/{repo}/branches → 200 {branches:[{name, target, is_head}...]}
```

Lists the repository's branches (each `refs/heads/*`), the commit each points at,
and which is HEAD. Read from `refs` (`FSResolver.List` + the symbolic HEAD), in
stable name order.

## 5.3 Commit history (paginated)

```
GET /_vara/repositories/{repo}/commits?ref=<name>&limit=<n>&before=<commit-id>
    → 200 {commits:[{id, message, author, timestamp, parents}...], next:"<commit-id>"?}
```

Returns commits reachable from `ref` (default HEAD) in graph order (newest first),
at most `limit` (default 50, capped, §10). Pagination is a **cursor**: `before` is
a commit id to start after, and the response's `next` is the cursor for the
following page (absent at the end). Order and reachability come from the commit
graph (RFC-0007/0013); each commit's fields come from the object store. History is
potentially deep (validation reached hundreds of commits per repo), so pagination
is mandatory, not optional.

## 5.4 Commit detail

```
GET /_vara/repositories/{repo}/commits/{id} → 200 {id, message, author, timestamp, parents, tree}
```

Returns one commit's metadata (including its tree id and parents). It does **not**
return file contents or a diff — those are future work (§14). A malformed or
unknown id is `404`.

## 5.5 Repository summary

```
GET /_vara/repositories/{repo}/summary
    → 200 {id, name, default_branch, head, commit_count, branch_count, last_commit}
```

One call a dashboard or repository landing page can render without a fan-out of
five requests: the repository's identity, its default branch and HEAD commit, its
commit and branch counts, and a summary of the latest commit. Every field is
projected from the same primitives as the individual endpoints (§5.7); the summary
is a convenience *composition*, never a new source of truth. `commit_count` MAY be
served from the commit-graph index (RFC-0013) so it is O(1) rather than a full
walk.

## 5.6 Caching (ETag / If-None-Match)

Every read endpoint returns a strong `ETag` derived from the content it projects —
for branches/history, the resolved commit ids; for a commit, its (immutable) id.
A client MAY send `If-None-Match`; when the tag is unchanged the server answers
`304 Not Modified` with no body. Because a commit id already content-addresses its
whole history, a branch tip's id is a perfect cache key: a page whose branch has
not advanced re-validates for free. ETags are advisory and additive — a client
that ignores them sees identical data.

## 5.7 Reading content, not reimplementing the engine

The read endpoints are a projection, not a second engine. Branches come from
`refs`; history order comes from the commit graph / graph index; commit fields
come from the object store. The server calls these existing packages and shapes
the result as JSON — it does not re-derive reachability, re-parse object bytes, or
cache a private copy of the graph. If a future read needs a capability the engine
does not expose, the engine grows the primitive (its own RFC) and the API calls
it; the API never grows a shadow implementation (H2).

# 6. Authorization

Every read endpoint requires the RFC-0018 **`read`** capability on the repository,
evaluated **before** any content is read — the same capability and the same
"authorize before act" ordering as the data-plane read (`info/refs` / `fetch`). A
caller who may clone a repository may read it in the Hub, and no other; a caller
without `read` gets `403`, having learned nothing about the repository's contents.
`read` is the only capability these endpoints require (they never mutate), so no
new capability is introduced.

# 7. Browser Sessions (httpOnly cookie)

A browser must not hold a bearer secret where XSS can read it. So a browser logs
in the same way (RFC-0020 `POST /_vara/sessions`) but MAY request a **cookie**
session: the server sets the session secret in a cookie marked **httpOnly**,
**Secure**, **SameSite=Strict**, scoped to the API path, and does **not** return
the secret in the body. The browser then authenticates by the cookie the browser
sends automatically; page JavaScript never sees the secret.

**Cookie authentication is merely another credential transport; it introduces no
second authentication system.** A browser cookie and a CLI `Authorization: Bearer`
carry the same RFC-0020 session secret and resolve through the exact same
`IdentitySource` to the same `Identity`.

* The cookie carries exactly the RFC-0020 session secret and resolves through the
  exact same credential lookup — a cookie is just a second *carrier* for a bearer
  credential, changing nothing about sessions themselves (RFC-0020 unchanged, and
  `Identity` stays mechanism-agnostic, S12).
* `SameSite=Strict` plus the same-origin serving model means a cross-site page
  cannot ride the cookie (CSRF mitigation); state-changing routes remain the
  RFC-0019/0020 control plane, reachable only same-origin with the cookie.
* Logout (`DELETE /_vara/sessions/current`) revokes the session (RFC-0020 §5.4)
  **and** clears the cookie. `Authorization: Bearer` still works unchanged for the
  CLI and programmatic clients — the cookie is additive, for browsers only.

# 8. Static UI Serving

`vara serve --hub <dir>` serves the Hub UI from `<dir>` on the **same origin** as
the API. Routing precedence is explicit and unambiguous:

1. `/_vara/…` — control plane (RFC-0019/0020/0021).
2. `/{repo}/info/refs`, `/{repo}/fetch`, `/{repo}/receive` — data plane (RFC-0016).
3. everything else — static files from `<dir>` (with `index.html` for `/` and for
   unknown paths, so a client-side-routed SPA works).

The static handler is a strict fallback: it is consulted only for paths that match
no API or data-plane route (H3), so the UI can never shadow an endpoint. Serving
the UI is optional — omit `--hub` and the server is exactly today's API-only host.
Because UI and API share an origin, **no CORS is involved** in the normal
deployment.

# 9. CORS (cross-origin development, reserved)

For local frontend development against a separately-served UI (e.g. a dev server
on another port), the server MAY be configured with an explicit **allowlist** of
origins (`vara serve --cors-origin <origin>`, repeatable). It then emits
`Access-Control-Allow-Origin` for a listed origin only, with
`Access-Control-Allow-Credentials: true`, and never the wildcard `*` with
credentials. CORS is **off by default** and is a development convenience, not the
production posture (which is same-origin, §8). Cookie sessions do not cross origins
regardless; a cross-origin dev client uses a bearer token.

# 10. HTTP Binding

New routes (all `GET`, all require `read`):

```
GET /_vara/repositories/{repo}/summary
GET /_vara/repositories/{repo}/branches
GET /_vara/repositories/{repo}/commits
GET /_vara/repositories/{repo}/commits/{id}
```

DTOs (added to `internal/protocol`): `RepositorySummary`, `BranchInfo`,
`BranchesResponse`, `CommitSummary`, `CommitsResponse` (with `next` cursor),
`CommitDetail`.

Status codes:

| Situation | Status | Code |
|-----------|--------|------|
| OK | 200 | — |
| Not modified (ETag matched) | 304 | — |
| Unknown repository or commit | 404 | `UNKNOWN_REPOSITORY` / `NOT_FOUND` |
| `limit` out of range / bad cursor | 400 | `MALFORMED_REQUEST` |
| Unauthenticated | 401 | `UNAUTHENTICATED` |
| Lacks `read` | 403 | `UNAUTHORIZED` |
| Rate limited | 429 | `RATE_LIMITED` |

`limit` is clamped to a maximum (e.g. 100) so a single request can never ask the
server to walk unbounded history.

## 10.1 API versioning (independent of the transport protocol)

The Hub API carries its **own** version, decoupled from the RFC-0016 transport
protocol version (`X-VARA-Protocol`). The server advertises it in an `X-VARA-API`
response header (`1` for this RFC); a client MAY send `X-VARA-API` to assert the
version it expects. Path prefixes such as `/api/v1` were considered and rejected:
the control-plane routes are already established under `/_vara/` without a version
segment (RFC-0019/0020), and a header keeps those paths stable while still letting
the Hub API evolve on its own cadence. Engine, transport, and Hub API each version
independently.

## 10.2 JSON compatibility — additive only

Within API v1, JSON contracts are **additive only**: a field is never renamed,
removed, or re-typed; new information arrives as new optional fields. A client
written against v1 keeps working as the API grows; a breaking change requires a new
`X-VARA-API` version (§10.1), never an in-place edit. (Normative: H9.)

## 10.3 Errors, request IDs, rate limiting

* **Errors** reuse the transport error schema (RFC-0016 §8.6:
  `{ok:false, code, message, details?}`). The Hub invents no error format — one
  schema spans the whole platform.
* **`X-Request-ID`**: every response carries a request id (echoed from the request
  if supplied, else server-generated) so a UI can quote it and an operator can grep
  for it — the debugging analogue of the RFC-0016 transaction id.
* **Rate limiting**: the server SHOULD apply a simple per-identity token bucket to
  the read API — not a security control but a guard against a runaway client (a UI
  bug that loops); an over-limit request is `429` with the standard error body. The
  exact limits are deployment configuration, not protocol.

# 11. Architectural Constraints (Normative)

* **H1 — Read is authorized before content is read.** Every read endpoint checks
  `read` (RFC-0018) before touching refs/graph/objects; a denied caller learns
  nothing (extends A2/A10).
* **H2 — Project the engine, never reimplement it.** Branches/history/commits are
  read through `refs`, the commit graph, and the object store; the API adds no
  traversal or storage logic and no `pkg/*` change (Single Implementation
  Principle).
* **H3 — Static serving never shadows an API or data-plane route.** The `--hub`
  handler is a strict fallback consulted only for otherwise-unmatched paths.
* **H4 — Browser secrets are httpOnly.** A browser session secret lives only in an
  httpOnly, Secure, SameSite=Strict cookie — never in a response body reachable by
  JavaScript, never logged (RFC-0020 S4/S10 preserved).
* **H5 — Same-origin by default; no wildcard CORS with credentials.** Cross-origin
  access is an explicit opt-in allowlist; `*` with credentials is never emitted.
* **H6 — No new mutation.** RFC-0021 is read + serving + browser-auth carrier only;
  all writes remain the RFC-0019/0020 control planes.
* **H7 — Engine unchanged.** RFC-0021 adds nothing to `pkg/*` and no RFC-0016 wire
  message.
* **H8 — Projection, never reinterpretation.** Hub read endpoints MUST project
  existing repository state — refs, commits, objects — as it already is. They MUST
  NOT create an alternate representation of a commit, a reference, or an object, nor
  a private index or cache that could diverge from the engine's truth. The API is a
  view; the engine remains the single source of truth.
* **H9 — Additive-only JSON, independent API version.** Within an API version, JSON
  contracts change only by adding optional fields (§10.2); a breaking change bumps
  the `X-VARA-API` version (§10.1), which is independent of the transport-protocol
  and engine versions.

# 12. Security Considerations

* **Token exposure.** The httpOnly cookie (§7) keeps the session secret out of
  JavaScript, defeating token theft via XSS — the primary reason not to keep
  bearer tokens in `localStorage`. Bearer-header auth remains for non-browser
  clients that manage their own secret storage.
* **CSRF.** `SameSite=Strict` + same-origin serving means a third-party page
  cannot cause an authenticated state-changing request; the control-plane
  mutations are same-origin only.
* **Read authorization is not weaker than clone.** The read API gates on the same
  `read` capability as `fetch`, so exposing history/branches grants no access a
  clone would not (H1).
* **History walks are bounded.** `limit` is capped and history is paginated, so a
  read request cannot force an unbounded traversal (DoS bound).
* **Static serving is read-only and confined.** `--hub` serves a fixed directory;
  it resolves and confines paths within that directory (no traversal), serves no
  dotfiles, and never executes.
* **No content leakage without authorization.** Commit messages/authors are
  repository content; they are returned only to a `read`-authorized caller.

# 13. Testing Strategy

* **Branches / commits / detail** return correct data for a repo with a known
  history (including a merge commit's two parents), newest-first, HEAD flagged.
* **Pagination** — `limit` caps the page, `next` cursors through the whole history
  exactly once with no gaps or repeats; an out-of-range `limit` is 400.
* **Authorization (H1)** — a caller without `read` gets 403 from every read
  endpoint and no body content; with `read`, 200. 401-vs-403 never swapped.
* **Browser session (H4)** — a cookie login sets an httpOnly+Secure+SameSite
  cookie and returns no secret in the body; the cookie authenticates subsequent
  reads; logout clears it and the next request is 401.
* **Static serving (H3)** — `/_vara/...` and `/{repo}/info/refs` still route to the
  API/data plane with `--hub` set; an unknown path serves `index.html`; a path
  traversal attempt is refused.
* **CORS (H5)** — a listed origin gets `Access-Control-Allow-Origin`; an unlisted
  one does not; `*`-with-credentials never appears.
* **Architecture (H2/H7)** — the engine diff is empty; the read API imports engine
  read packages but adds no traversal/storage logic; `tests/architecture` passes.

# 14. Future Work

* **Reserved successor RFCs** — each a focused subsystem layered on this read API:
  **RFC-0022 Repository Browser** (tree / blob / file history), **RFC-0023 Diff
  Viewer** (commit diffs, branch comparison), **RFC-0024 Search** (repository and
  commit search); then AI surfaces (RFC-0011).
* **Tree & blob browsing** — list a commit's tree, read a file's contents, a file
  history — the "repository browser / file explorer" views (RFC-0022).
* **Diffs** — commit diffs and branch comparison (the diff/commit/merge-conflict
  viewers).
* **Search** — repository and commit search.
* **Richer overview** — commit/branch counts, last-activity, contributors on the
  repository descriptor.
* **Live updates** — websockets/SSE for push notifications to an open UI.
* **AI surfaces** (RFC-0011) — "explain this commit", "summarize", "find when this
  bug started", layered on the read API once it exists.
