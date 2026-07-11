VARA RFC: 0016
Title: Remote Transport Protocol (HTTP Binding v1)
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-11
Last Updated: 2026-07-11
Depends On: RFC-0004, RFC-0006, RFC-0014
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0014 defined *what* two VARA repositories exchange (references, object
closures, the VPCK stream) and the semantics that make an exchange safe
(compare-and-swap ref updates, fast-forward rules, all-or-nothing transfer). It
did so against an in-process `Transport` interface, implemented only by the
**local filesystem transport**.

This document defines the **wire protocol** that carries those same operations
between two processes across a network. It specifies the request/response
messages, their encoding, their guarantees, and their failure behavior
**independently of any single transport mechanism**, then binds them to HTTP as
the first concrete binding.

**Design stance — protocol first, HTTP second.**

> The protocol is the set of messages and guarantees in §4–§7. HTTP (§8) is
> *binding v1*: one way to move those messages over a wire. A future gRPC or raw
> `vara://` socket binding MUST carry the identical messages and honor the
> identical guarantees, so a repository is indifferent to which binding a client
> speaks.

```
                 Remote Transport Protocol   (this RFC, §4–§7)
                           messages + guarantees
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
              HTTP Binding v1              Future gRPC / vara://
                  (§8)                        (out of scope)
                    │                           │
              internal/server            (future server)
                    │
              internal/transport (Local)      ← the engine, unchanged
```

**Key invariant (inherited from RFC-0014, restated for the network):**

> A transfer is complete only if, for every reference reported as updated, the
> receiving repository can resolve that reference's full object closure from its
> own store. An interrupted or rejected request MUST NOT advance any reference
> and MUST NOT corrupt the repository.

# 2. Motivation

RFC-0014 makes VARA distributed in principle but only over a shared filesystem:
the local transport opens the peer's `.vara` directory directly. To collaborate
across machines, VARA needs a transport that:

1. Advertises a repository's references to a remote client.
2. Streams object closures in both directions over a byte channel.
3. Applies reference updates on the server under the *same* atomic CAS
   guarantee the local transport already provides — the guarantee MUST NOT
   weaken merely because a network now sits between the CAS and the caller.
4. Fails safely: a dropped connection, a timeout, or a retried request never
   leaves either side pointing at an object it does not have.

The `Transport` interface (RFC-0014 §6) already draws the seam. This RFC turns
that seam into a wire protocol and provides two implementations of it: an HTTP
**client** transport (`HTTPTransport`, satisfying the same interface) and an
HTTP **server** (`internal/server`) that dispatches each request to a `Local`
transport bound to the requested repository.

# 3. Non-Goals

Explicitly **out of scope** for RFC-0016, each deferred to a named successor:

* **Authentication** (who is the caller) — RFC-0017 (Identity).
* **Authorization** (may the caller read/write this repository) — RFC-0018
  (Repository Permissions).
* **Repository hosting / lifecycle** (creating, listing, deleting repositories
  over the network; storage layout on the server) — VARA Hub, a later RFC.
* **Web UI, pull requests, issues, AI features.**
* **Alternate bindings** (gRPC, `vara://` sockets) — this RFC defines only the
  protocol and its HTTP binding.
* **Pack optimization** (delta packs, thin packs, streaming ingestion) —
  RFC-0015. The wire uses the VPCK format of RFC-0014 §8 verbatim.

RFC-0016 targets **anonymous, unauthenticated** clone/fetch/pull/push against a
`vara serve` process. This is intentional: it proves the protocol-over-network
thesis end to end before identity is layered on top. §12 reserves the hooks
(headers, status codes) that RFC-0017/0018 will populate so adding auth is
additive, not a breaking change.

# 4. Protocol Guarantees

This section is the network analogue of RFC-0006 (Locking) and RFC-0014 §10. A
conforming server and client MUST provide every guarantee below, regardless of
binding.

* **G1 — Object integrity.** Every transferred object's identity is verified by
  recomputing its SHA-256 (RFC-0002); a peer cannot inject an object under a
  hash it does not hash to. Every VPCK stream carries a SHA-256 trailer over all
  preceding bytes (RFC-0014 §8); a stream whose trailer fails verification is
  rejected in full.
* **G2 — Atomic reference updates.** A reference moves all-or-nothing. The
  server advances a ref only after the entire object stream is ingested and its
  trailer verified. A partial or aborted `receive` advances no reference.
* **G3 — Compare-and-swap.** Every ref update carries the `Old` value the client
  believed current. The server applies the update only if the ref still equals
  `Old`, under the Refs lock (§7). A concurrent update makes exactly one pusher
  win; the loser is told its CAS is stale.
* **G4 — Idempotent fetch.** `list-refs` and `fetch` are side-effect-free reads
  on the server. A client MAY safely retry them; identical inputs produce
  equivalent object sets (enumeration is deterministic, RFC-0014 §7).
* **G5 — Interrupted-request safety.** A connection dropped mid-request leaves,
  at worst, unreferenced objects in the receiver's store (inert, reclaimable by
  `vara gc`). It never advances a ref and never corrupts the store. On the
  client side, an interrupted clone rolls back its partial destination
  (RFC-0014 clone rollback).
* **G6 — No partial application of a ref-update batch's *effects*.** Within a
  single `receive`, each ref update is independently CAS-checked and reported;
  the server MUST NOT report an update as `OK` unless the ref was actually moved.
  (Batch atomicity across *multiple* refs is not required in v1 — see §7.4 —
  but per-ref honesty is.)

These guarantees are stated so that a future binding author has a fixed contract
to satisfy, and so that a server implementation that weakens G3 (the easy
mistake when moving the CAS behind an HTTP handler) is non-conforming by
definition.

# 5. Protocol Messages

The protocol is three request/response pairs, mapping one-to-one onto the
`Transport` interface (RFC-0014 §6). Messages are described here as abstract
structures; §8 gives their HTTP encoding. Field types reuse RFC-0014: `CommitID`
and `ObjectID` are 32-byte SHA-256 values, rendered on the wire as lowercase hex.

## 5.1 ListRefs

```
ListRefsRequest {
    repo   string        // repository identifier (path component)
}

ListRefsResponse {
    head   string                 // symbolic HEAD target, e.g. "refs/heads/main"
    refs   []RefAdvertisement     // { name string; target CommitID }
    caps   []string               // server capability set (§5.4)
}
```

Read-only (G4). Advertises every `refs/heads/*` on the server, its HEAD target,
and its capability set, exactly as `Local.ListRefs` / `Local.HeadTarget` do
today (the capability set is added by the binding, not the transport).

## 5.2 Fetch

```
FetchRequest {
    repo   string
    wants  []CommitID     // commits the client wants
    haves  []CommitID     // commits the client already has (may be empty)
}

FetchResponse {
    pack   VPCK stream     // closure(wants) − closure(haves), RFC-0014 §7/§8
}
```

Read-only (G4). The server enumerates and streams; it makes no mutation. An
empty `wants` is valid and yields an empty pack (header + zero records +
trailer).

## 5.3 Receive

```
ReceiveRequest {
    repo     string
    pack     VPCK stream          // objects the server may lack
    updates  []RefUpdate          // { name; old CommitID; new CommitID; force bool }
}

ReceiveResponse {
    results  []RefUpdateResult     // { name; ok bool; reason string }
}
```

Mutating. The server ingests `pack` (G1), then applies each `RefUpdate` under
CAS + fast-forward rules within the Refs lock (§7), and reports per-ref results
(G3, G6). The response reports rejections in-band — a rejected fast-forward is a
**successful request with `ok:false` results**, not a transport error (this
mirrors `Local.ReceivePack`, which returns results, not an error, for a
non-fast-forward).

## 5.4 Capability negotiation

`list-refs` is also the negotiation point. Beyond the protocol version
(`X-VARA-Protocol`) and wire version (`X-VARA-Wire`, §8.3), the server advertises
a **capability set** — a list of opt-in features the client MAY then use. This
lets the protocol grow without new endpoints or version bumps: a client that
does not understand a capability simply does not use it, and a server that lacks
one omits it. Capabilities are additive and MUST NOT change the meaning of a
request that does not invoke them.

v1 reserves the mechanism and defines an initial vocabulary; a v1 server MAY
advertise the empty set and remain conforming.

| Capability | Meaning (when advertised) |
|------------|---------------------------|
| `pack-zstd` | Server accepts/produces zstd-framed VPCK (the v1 default framing). |
| `pack-none` | Server accepts/produces uncompressed VPCK records. |
| `receive-idempotent` | Server deduplicates a retried `receive` by `X-VARA-Transaction` (§7.6). |
| `report-status-v1` | Server returns the structured per-ref result schema (§8.6). |

Compression framing (`pack-zstd` / `pack-none` / a future `pack-lz4`) is
**reserved here, not negotiated in v1**: v1 always uses the zstd framing of
RFC-0014 §8. Advertising the tokens now means a future client and server can
agree on an alternative framing purely through the capability set, with no new
wire version required for the *negotiation* itself (only the pack format's own
`X-VARA-Wire` bump).

## 5.5 Message ordering within an operation

The high-level operations (RFC-0014 §9) compose these messages unchanged:

* **clone** = `ListRefs` → `Fetch(wants=all tips, haves=[])` → local ref setup +
  checkout, with rollback on any failure.
* **fetch** = `ListRefs` → `Fetch(wants=tips, haves=local commits)` → update
  tracking refs.
* **pull** = fetch → `MergeIntoHEAD` (local, RFC-0008).
* **push** = `ListRefs` → local fast-forward pre-check → `Receive(pack, updates)`.

Only `Receive` mutates the server. Everything else is a read the client may
retry freely.

# 6. Failure Semantics

Every message defines its behavior under failure so retries are safe (G4, G5).

| Condition | Protocol behavior |
|-----------|-------------------|
| **Interrupted upload** (client → server, during `receive`) | Server ingests whatever objects fully arrived (inert), then the trailer check fails → **no ref update**, error returned. Client MAY retry the whole `receive`; re-sent objects are no-op writes. |
| **Interrupted download** (server → client, during `fetch`) | Client's trailer check fails → client discards/ignores the partial pack (objects it wrote are inert) → **no ref update**, retriable. On clone, the partial destination is rolled back. |
| **Timeout** | Treated as an interrupted request of whichever phase was in flight. Same guarantees; retriable. |
| **Duplicate / retried request** | `list-refs` and `fetch` are idempotent (G4). A retried `receive` is safe: object writes are idempotent, and the CAS makes a second application either a no-op (ref already at `New`, reported per §7.4) or a stale-CAS rejection. |
| **Stale client** (`Old` no longer current) | `receive` returns `ok:false, reason:"stale: …"` for that ref (G3). Not a transport error. |
| **Missing objects** (a `RefUpdate.New` absent from pack and store) | That ref is rejected `ok:false, reason:"new commit … missing from pack"`. Other refs in the batch are unaffected. |
| **Invalid pack** (bad magic, version, or trailer mismatch) | Whole `receive`/`fetch` fails; **no ref update**. Distinct from a per-ref rejection — this is a request-level error. |
| **Malformed request** (unparseable body, unknown repo, bad hex) | Request-level error before any mutation. |

The load-bearing distinction: **request-level errors** (invalid pack, malformed
request, unknown repo) fail the whole call and touch nothing; **per-ref
rejections** (stale CAS, non-fast-forward, missing object) are a *successful*
`receive` whose response reports which refs did and did not move. Clients MUST
distinguish the two and MUST NOT blindly retry a per-ref rejection (G3) — the
correct response is to re-fetch, re-integrate, and re-push.

## 6.1 Timeout ownership

To keep implementations from drifting, the protocol assigns each timeout to
exactly one owner:

* **Transport/connection timeout — owned by the client.** The client sets the
  deadline on the whole request (connect + send + receive). A client that gives
  up MUST treat the operation as an interrupted request of whichever phase was in
  flight (§6) — safe to retry per G4/G5. The server never depends on the client's
  deadline for correctness.
* **Repository lock timeout — owned by the server, fixed by RFC-0006.** The Refs
  lock uses the existing `AcquireWithTimeout(…, 30s)` (§7); if the server cannot
  acquire it, it fails the `receive` with a `500` and advances no ref (G2). This
  timeout is a property of the repository, not the request, and is not
  client-tunable in v1.
* **Request-processing / read limits — owned by the server.** Body-size caps and
  enumeration bounds (§10) are the server's defense and are independent of the
  client's connection deadline.

The rule of thumb: **the client owns "how long am I willing to wait," the server
owns "how long am I willing to hold a lock or buffer a body."** Neither borrows
the other's deadline, so a slow client cannot extend a server lock and a slow
server cannot silently outlive a client's patience without the client treating it
as an interruption.

# 7. Concurrency Contract

This section is normative and exists to prevent the single most likely
regression in moving from the local transport to a server: **weakening the CAS
when it moves behind a request handler.**

## 7.1 The requirement

> The server MUST preserve the atomic compare-and-swap guarantee of RFC-0006 for
> every reference update, exactly as `Local.ReceivePack` does today. The presence
> of an HTTP (or any other) layer between the caller and the CAS MUST NOT widen
> the window between the ref read and the ref write.

## 7.2 The mandated sequence

For each `receive` request the server MUST perform:

```
             Receive request
                    │
                    ▼
        Ingest objects into store        ← content-addressed, lock-free,
                    │                       safe to interleave across requests
                    ▼
          Verify VPCK trailer (G1)        ← reject whole request on mismatch,
                    │                       BEFORE acquiring the lock
                    ▼
          Acquire the Refs lock           ← locks/refs.lock, RFC-0006 §2, O_EXCL
                    │
                    ▼
     For each update:  read → CAS-check → fast-forward-check → write
                    │
                    ▼
           Release the Refs lock
                    │
                    ▼
             Respond with results
```

This is precisely the sequence `Local.ReceivePack` implements (`internal/
transport/local.go`). The server does not reimplement it — it **calls it**. See
§9.

## 7.3 Why object ingestion precedes the lock

Objects are immutable and content-addressed (RFC-0002), so writing an object
that already exists is a no-op and two requests writing the same object cannot
conflict. Ingesting before the lock keeps the lock held only for the short
ref-update phase, so concurrent pushes serialize on refs — not on the (large,
slow) object transfer. This is the correct throughput/safety balance and MUST be
preserved by every binding.

## 7.4 Batch semantics

Within one `receive`, updates are applied sequentially under the single held
lock. Each is independently CAS-checked and reported (G6). v1 does **not**
provide all-or-nothing atomicity across *multiple* refs in one batch: if a batch
updates two refs and the second's CAS fails, the first remains applied and is
reported `ok:true`. This matches `Local.ReceivePack`. Multi-ref transactional
push is deferred; a client needing it pushes one ref per request. An update
whose `New` already equals the current ref value (and `Old` matches) is reported
`ok:true` as a no-op, making retried pushes idempotent (§6).

## 7.5 Cross-request isolation on the server

A `vara serve` process handling concurrent requests for the **same** repository
relies entirely on the on-disk Refs lock (`O_EXCL`, RFC-0006) for mutual
exclusion, not on in-process synchronization — because correctness must also hold
against a *second* `vara serve` process, or a local `vara` command, operating on
the same `.vara` directory. The lock is the single source of truth. The server
MUST NOT cache ref values across the lock boundary; it re-reads under the lock.

## 7.6 Receive idempotency

A `receive` may complete on the server and then have its response lost to a
dropped connection, leaving the client unsure whether the push landed. The
protocol makes the natural retry safe **by construction**, and defines an
optional stronger guarantee:

* **Baseline (always).** Re-sending the same `receive` is safe: object writes are
  content-addressed no-ops (§7.3), and each ref update is re-evaluated under CAS.
  The retry has one of two outcomes, both correct:
  * the first attempt had *not* applied → the retry applies normally; or
  * the first attempt *had* applied → the ref now equals `New`. If the client
    re-sends the original `{Old, New}`, the CAS finds the ref at `New` ≠ `Old` and
    the retry is reported `ok:false, reason:"stale"`. This is a **false failure**
    for an operation that in fact succeeded.
* **The client's obligation.** Because of that false-failure case, a client that
  retries a `receive` and receives a stale-CAS rejection MUST re-run `list-refs`
  before concluding the push failed: if the ref already equals its intended
  `New`, the push succeeded and the "rejection" is an artifact of the retry, not
  a real conflict. This keeps the baseline safe without server-side state.
* **`receive-idempotent` capability (optional, §5.4).** A server MAY advertise
  `receive-idempotent` and remember, keyed by `X-VARA-Transaction` (§8.3) for a
  bounded window, the result of each `receive` it completes. A retry carrying a
  transaction ID it has already applied returns the **original stored results**
  (reporting the update as `ok:true`) instead of re-evaluating the now-stale CAS.
  This turns the false-failure into a true success without a `list-refs`
  round-trip. The window is best-effort; a client MUST still fall back to the
  baseline obligation if the server has forgotten the transaction.

In no case does a retry create duplicate history, a duplicate commit, or double
work on the object store — the content-addressed store and the CAS make "apply
twice" indistinguishable from "apply once."

# 8. HTTP Binding v1

HTTP is binding v1. It maps each §5 message onto one HTTP request/response.

## 8.1 Endpoints

```
GET   /:repo/info/refs          → ListRefs      (read, idempotent)
POST  /:repo/fetch              → Fetch         (read, idempotent)
POST  /:repo/receive            → Receive       (mutating)
```

`:repo` is a path segment naming the repository on the server (e.g.
`/myproject/info/refs`). Its resolution to a `.vara` directory is a server
configuration concern (a served root directory in v1); repository *hosting* is
out of scope (§3). The server MUST reject a `:repo` that contains path-traversal
segments (`.`, `..`, absolute paths, or separators) with `400`.

The `info/refs` spelling (rather than bare `/refs`) is chosen deliberately to
leave the `/:repo/*` namespace open for future non-transport endpoints without
collision.

## 8.2 Content types

* `ListRefsResponse` — `application/json` (small, structured; §8.4).
* `FetchRequest`, `ReceiveRequest`'s update list, `ReceiveResponse` — the
  structured parts are `application/json`.
* VPCK streams (the `fetch` response body, the `receive` request pack) —
  `application/x-vara-pack`, an opaque binary stream (RFC-0014 §8).

Because `receive` carries **both** a JSON update list and a binary pack, v1
encodes the request as `multipart/mixed` with two parts: part 1
`application/json` = `{ "updates": [...] }`, part 2 `application/x-vara-pack` =
the VPCK bytes. The `fetch` request is small and carries no pack, so it is a
single `application/json` body; its *response* is the raw pack stream.

> Rationale: keeping the pack as a raw part (not base64 in JSON) preserves G1's
> streaming/trailer semantics and avoids a ~33% base64 inflation on the largest
> payload. A future binding MAY frame these differently provided the two logical
> parts (updates, pack) remain separable and the trailer stays intact.

## 8.3 Reserved headers

Every request and response carries these headers. v1 sets and checks the version
headers; the remainder are **reserved now** so RFC-0017/0018 add semantics
without a breaking change (§12):

| Header | v1 meaning | Reserved for |
|--------|-----------|--------------|
| `X-VARA-Protocol` | Protocol version, `"16.1"` (RFC number . binding version). Server rejects unknown major with `426`. | Version negotiation |
| `X-VARA-Wire` | VPCK wire version, `"1"` (RFC-0014 §8). | Pack format evolution (RFC-0015) |
| `X-VARA-Repository` | Echoes `:repo`; lets proxies route without parsing the path. | Hosting / routing |
| `X-VARA-Transaction` | Optional client-generated opaque request ID; echoed verbatim in the response (§8.3.1). Enables idempotent-retry correlation and logging. | Retry dedup, tracing |
| `Authorization` | Ignored in v1 (anonymous). | RFC-0017 (Identity) |

A v1 server MUST ignore headers it does not understand and MUST NOT fail a
request for a *present but empty* reserved header.

### 8.3.1 X-VARA-Transaction semantics

* **Optional** — a client MAY omit it; the server MUST NOT require it.
* **Client-generated** — the client mints the value (a UUID or any opaque
  unique token). The server never generates it for a request.
* **Echoed** — the server MUST copy the received value verbatim into the response
  headers, unchanged, whether the request succeeds or fails. This is what makes a
  client's log line correlate with the server's.
* **Retry stability** — a client retrying the *same* logical operation SHOULD
  reuse the *same* transaction ID; that is precisely the key the
  `receive-idempotent` capability (§7.6) deduplicates on. A client that mints a
  fresh ID per attempt forgoes server-side dedup but is still safe under the
  baseline (§7.6).
* A server with no use for it (v1 default) simply echoes it and moves on.

## 8.4 JSON shapes

Commit/object IDs are lowercase hex strings.

```jsonc
// GET /:repo/info/refs  → 200
{
  "head": "refs/heads/main",
  "refs": [
    { "name": "refs/heads/main", "target": "9c1999a…(64 hex)" }
  ]
}

// POST /:repo/fetch  request body
{
  "wants": ["…64 hex…"],
  "haves": ["…64 hex…"]        // may be []
}
// → 200, body = application/x-vara-pack (raw VPCK stream)

// POST /:repo/receive  multipart part 1 (application/json)
{
  "updates": [
    { "name": "refs/heads/main",
      "old": "…64 hex or 64 zeros for a new ref…",
      "new": "…64 hex…",
      "force": false }
  ]
}
// part 2 (application/x-vara-pack) = raw VPCK stream

// POST /:repo/receive  → 200
{
  "results": [
    { "name": "refs/heads/main", "ok": true,  "reason": "" },
    { "name": "refs/heads/dev",  "ok": false, "reason": "non-fast-forward: …" }
  ]
}
```

The zero `CommitID` (64 hex zeros) denotes "ref does not exist" in an `old`
field, matching `zeroCommit` in the transport layer.

## 8.5 Status codes

| Code | When | Maps to |
|------|------|---------|
| `200 OK` | Request processed. For `receive`, **this includes per-ref rejections** — inspect `results`. | Normal completion, G6 |
| `400 Bad Request` | Malformed body, bad hex, illegal `:repo`, invalid pack framing. | Request-level error (§6) |
| `404 Not Found` | `:repo` names no served repository. | Unknown repo (§6) |
| `409 Conflict` | *Optional* fast path: server MAY short-circuit a `receive` whose every update is a stale CAS. Clients MUST also handle the equivalent `200` + all-`ok:false`. | Stale CAS (G3) |
| `422 Unprocessable Entity` | VPCK trailer mismatch / corrupt pack after a well-formed request. | Invalid pack (G1, §6) |
| `426 Upgrade Required` | `X-VARA-Protocol` major unsupported. | Version negotiation |
| `500 Internal Server Error` | Unexpected server fault (I/O, lock timeout). **Must leave no ref advanced** (G2, G5). | Server fault |

The critical rule: **a non-fast-forward or stale-CAS push is `200` with
`ok:false` results, not a `4xx`** (except the optional `409` fast path). The
per-ref outcome lives in the body, because a batch can mix accepted and rejected
refs (§7.4). Clients decide success by reading `results`, never by status code
alone for `receive`.

## 8.6 Structured bodies (never status alone)

A client MUST NOT have to reverse-engineer meaning from the HTTP status line
alone. Every non-`2xx` **request-level error** carries a JSON body with a stable
machine-readable code:

```jsonc
// any 4xx / 5xx request-level error
{
  "ok": false,
  "code": "INVALID_PACK",        // stable enum, see below
  "message": "VPCK trailer mismatch: stream corrupt or truncated",
  "details": {}                   // optional, code-specific
}
```

Stable `code` values for v1 (extensible — clients MUST treat an unknown code as a
generic failure of its status class):

| `code` | Status | Meaning |
|--------|--------|---------|
| `MALFORMED_REQUEST` | 400 | Unparseable body, bad hex, illegal `:repo`. |
| `UNKNOWN_REPOSITORY` | 404 | `:repo` names no served repository. |
| `INVALID_PACK` | 422 | VPCK magic/version/trailer check failed. |
| `UPGRADE_REQUIRED` | 426 | Unsupported `X-VARA-Protocol` major. |
| `LOCK_TIMEOUT` | 500 | Server could not acquire the Refs lock (§6.1). |
| `INTERNAL` | 500 | Unexpected server fault; no ref advanced. |

**Per-ref rejections are the deliberate exception.** A non-fast-forward or
stale-CAS `receive` is *not* a request-level error — it is a `200` whose
`results[]` entries each carry their own outcome:

```jsonc
// POST /:repo/receive → 200
{
  "ok": false,                    // false ⇔ at least one ref did not move
  "results": [
    { "name": "refs/heads/main", "ok": true,  "code": "OK",               "reason": "" },
    { "name": "refs/heads/dev",  "ok": false, "code": "NON_FAST_FORWARD", "reason": "…" }
  ]
}
```

Per-ref `code` values: `OK`, `NON_FAST_FORWARD`, `STALE`, `MISSING_OBJECT`,
`CREATE_FAILED`, `UPDATE_FAILED` — a machine-readable encoding of the
`RefUpdateResult.Reason` strings the transport already returns. The top-level
`ok` is the AND of every per-ref `ok`, so a client can branch on one boolean and
drill into `results` only when it is false. This structured shape is gated behind
the `report-status-v1` capability (§5.4) so a future richer schema is additive.

# 9. Server Architecture

The server is a new package **above** the transport in the import hierarchy; it
consumes the engine, never the reverse.

```
internal/server        (this RFC — HTTP handlers)
        │ imports
        ▼
internal/transport     (RFC-0014 — Local, and HTTPTransport client)
        │ imports
        ▼
internal/repository, pkg/refs, pkg/object, pkg/graph, pkg/transfer, internal/locking
```

* **`HTTPTransport`** (client) lives in `internal/transport` beside `Local` and
  satisfies the same `Transport` interface (RFC-0014 §6). `RunClone`/`RunFetch`/
  `RunPull`/`RunPush` (`internal/commands`) select `Local` vs `HTTPTransport` by
  URL scheme — a `http(s)://` or `vara://` URL yields `HTTPTransport`, a path or
  `file://` yields `Local` — and are otherwise **unchanged**. This is the payoff
  of the interface: the command layer does not learn that the network exists.
* **`internal/server`** exposes an `http.Handler` (mux over the §8.1 routes).
  Each handler:
  1. resolves `:repo` to a `.vara` directory (served-root + validated segment),
  2. opens a `Local` transport on it (`transport.Open`),
  3. decodes the §5 message, calls the matching `Local` method
     (`ListRefs` / `FetchPack` / `ReceivePack`), and
  4. encodes the result per §8.
* **`vara serve [--addr :port] [--root <dir>]`** (`internal/commands` +
  `cmd/vara`) starts the server over a directory of repositories.

**The server contains no version-control logic.** It is a codec between HTTP and
the `Local` transport. In particular the concurrency contract (§7) is satisfied
*by calling `Local.ReceivePack`*, which already holds the Refs lock across the
CAS — the handler MUST NOT reimplement ref updating and MUST NOT hold results
across a second, wider lock of its own.

## 9.1 Single Implementation Principle

The rule that makes the previous paragraph load-bearing, stated so it applies to
every present and future binding:

> **Single Implementation Principle.** A transport binding (HTTP, gRPC, `vara://`,
> …) MUST delegate every repository mutation to the transport implementation
> (`Local`) and MUST NOT reimplement repository semantics — ref updates, CAS,
> fast-forward checks, locking, object writes. A binding is a codec: it decodes a
> request into a `Transport` call and encodes the result. There is exactly **one**
> implementation of the concurrency-critical logic (RFC-0006 + RFC-0014 §10),
> and every binding routes through it.

This is why adding a second binding later cannot reintroduce the concurrent-push
race: the race lives in code no binding is permitted to duplicate.

## 9.2 A server MUST NOT touch a working tree

A repository server **stores** repositories; it is not a desktop client. It
therefore MUST NOT perform any working-tree mutation as part of serving a
request:

> The server MUST NOT `checkout`, `switch`, materialize files, or merge into a
> working tree. It operates only on the object store and ref store. Working-tree
> operations (RFC-0008 merge into HEAD, checkout on clone) belong to the **client**
> that owns a working tree; the server's served `.vara` directories are bare in
> spirit — no working copy is read or written on their behalf.

Concretely: `Local.ReceivePack` / `FetchPack` / `ListRefs` never touch a working
tree, and the server calls nothing else. `pull`'s `MergeIntoHEAD` and `clone`'s
checkout run entirely on the *client*, never the server. This keeps the server
stateless with respect to any working copy and prevents a class of bugs where a
served repository's checkout races a push.

> **Abstraction check.** If implementing the server requires changing anything
> in `internal/transport` or below, that is a signal the `Transport` interface is
> wrong for the network case — stop and revise the interface deliberately rather
> than reaching down through it. The expected change surface of this RFC is
> exactly two new files' worth: `HTTPTransport` (client) and `internal/server`
> (handler), plus `vara serve` wiring.

# 10. Security Considerations

This section covers **protocol** security — properties the wire format and server
must hold regardless of authentication. Caller identity and per-repository
authorization are separate concerns, deferred to RFC-0017/0018.

v1 is **anonymous and unauthenticated by design** (§3); it is intended for
trusted networks, local development, and as the substrate RFC-0017 builds on. It
nonetheless preserves VARA's content-level integrity:

* **Object integrity (G1).** Every received object's SHA-256 is recomputed; a
  malicious peer cannot forge an object under a hash it does not hash to. The
  VPCK trailer bounds tampering to detectable corruption. This holds with or
  without authentication — it is the same guarantee the local transport provides.
* **Malformed-pack handling.** A corrupt, truncated, or hostile VPCK stream MUST
  be rejected as a request-level error (`422 INVALID_PACK`) *before* any ref
  update (G1, G2). The receiver validates magic, version, the object count, each
  record's declared length against remaining bytes, and the whole-stream trailer;
  a record length that overruns the buffer is a rejection, not a read past the
  end. Objects written from a stream later found corrupt are inert and swept by
  `vara gc` — they can never be referenced because the trailer failed.
* **No privilege escalation / traversal.** `:repo` is validated against
  path-traversal (`.`, `..`, separators, absolute paths — §8.1); the server
  serves only within its configured `--root`. It performs no privilege
  escalation and runs with the serving user's filesystem permissions.
* **Denial-of-service bounds.** A server SHOULD:
  * cap request body size (a `receive` pack and a `fetch`'s want/have list);
  * bound enumeration work per `fetch` (a client requesting an enormous closure);
  * bound the Refs-lock hold time (already fixed at 30s, §6.1) so a slow or
    malicious pusher cannot hold the lock indefinitely — object ingestion happens
    *before* the lock (§7.3), so a slow upload never blocks other pushers' CAS.
  Specific limits are a deployment concern; the protocol only requires that they
  exist and that exceeding one is a clean request-level error, never a crash.
* **Replay.** Because `receive` is a CAS on `Old` (G3), a replayed `receive` is
  harmless: once the ref has moved past `Old`, the replay is a stale-CAS
  rejection; the `receive-idempotent` capability (§7.6) additionally collapses a
  replay carrying a known `X-VARA-Transaction` into the original result. No
  replayed request can double-apply history or move a ref twice. (Replay as an
  *authentication* concern — a captured credential — is RFC-0017's domain.)
* **What v1 does *not* provide, and defers:** caller identity (RFC-0017),
  per-repository read/write authorization (RFC-0018), transport encryption, and
  rate limiting. **TLS is assumed to be terminated by front-end infrastructure**
  (reverse proxy) until a native transport-encryption story exists; the protocol
  is TLS-agnostic and carries no secrets in v1 (it is anonymous). Operators MUST
  NOT expose a v1 `vara serve` to an untrusted network as a *write* endpoint,
  because any client can push.

# 11. Testing Strategy

The protocol's guarantees (§4) are the test checklist. Because `HTTPTransport`
and `Local` satisfy the same interface, most transport tests can run against
*both* to prove wire-parity.

* **Interface parity.** Run the RFC-0014 transport suite
  (`TestReceivePackFastForward`, `…RejectsNonFastForward`, `…StaleCAS`,
  `…ConcurrentPushOneWins`) against an `HTTPTransport` pointed at an in-process
  `httptest.Server`, asserting identical outcomes to `Local`.
* **G3 over the wire.** The concurrent-push test is the load-bearing one: two
  `HTTPTransport` clients pushing divergent commits at one server ref
  concurrently — exactly one `ok:true`, the ref ends at a real winner. This
  proves §7 survived the HTTP boundary.
* **G5 interruption.** Truncate a `receive` body mid-stream (close the
  connection) → server advances no ref, store has only inert objects, a
  subsequent `vara gc` reclaims them; and truncate a `fetch` response → client
  clone rolls back.
* **§6 failure table.** One test per row: stale CAS → `200`+`ok:false`; corrupt
  pack → `422`; unknown repo → `404`; traversal `:repo` → `400`; unknown
  protocol major → `426`.
* **Round-trip.** Full `clone` over HTTP against a populated server materializes
  the working tree and both `refs/heads/*` and `refs/remotes/origin/*`, matching
  the local-transport clone test byte for byte.
* **Stress.** The RFC-0014 stress harness (`TestStressRemote`, env-gated)
  extended with an HTTP mode, to confirm the network binding does not change the
  linear-clone / bounded-memory findings beyond expected serialization overhead.

# 12. Compatibility & Extension

* **Protocol versioning.** `X-VARA-Protocol: "<rfc>.<binding>"`. v1 is `"16.1"`.
  A server rejects an unknown *major* (RFC number) with `426`; a client and
  server disagreeing only on *minor* proceed on the lower common feature set.
* **Wire versioning.** `X-VARA-Wire` tracks the VPCK format (RFC-0014 §8,
  currently `1`); RFC-0015's delta packs will bump it, negotiated separately from
  the protocol version so pack evolution and protocol evolution are independent.
* **Additive auth (RFC-0017).** Identity slots into the reserved `Authorization`
  header and a new `401`/`403` status range; an authenticated server rejecting an
  anonymous `receive` is a *new* behavior gated behind RFC-0017, not a change to
  any v1 message shape. A v1 client talking to an RFC-0017 server simply receives
  `401` where it used to receive `200`.
* **Additive authorization (RFC-0018).** Per-repo permission checks occur after
  identity, before the §7 sequence; a denied write is `403` and, like a
  request-level error, advances no ref.
* **New bindings.** A gRPC or `vara://` binding is a new §8-equivalent section
  under this same RFC number (or a thin successor) reusing §4–§7 unchanged. The
  message set (§5) and guarantees (§4) are the stable contract; bindings are
  replaceable.
* **Reserved headers.** `X-VARA-Repository` is reserved and unused in v1;
  `X-VARA-Transaction` is already echoed in v1 (§8.3.1) and gains dedup semantics
  only when a server advertises `receive-idempotent`. Both are safe to give
  richer meaning later; v1 servers ignore values they do not act on (§8.3).

# 13. Future Work

* **RFC-0017 Identity** — caller authentication (tokens/keys) over the reserved
  `Authorization` header; `401`/`403`.
* **RFC-0018 Repository Permissions** — per-repository read/write authorization
  layered after identity.
* **VARA Hub** — repository hosting (create/list/delete over the network),
  storage management, and a web UI, consuming this transport rather than
  extending it.
* **Streaming ingestion (RFC-0015)** — verify-and-write VPCK records as they
  arrive rather than buffering, relevant once transfers outgrow the bounded-heap
  regime measured in RFC-0014 §13.
* **Alternate bindings** — gRPC and/or a raw `vara://` socket binding for the
  same protocol.
* **New capabilities** — the negotiation *mechanism* is defined in v1 (§5.4);
  future features (alternate pack framing `pack-lz4`, streaming ingestion,
  multi-ref-atomic push) slot in as new capability tokens without a protocol
  version bump.
