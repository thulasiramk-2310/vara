VARA RFC: 0024
Title: Search
Status: Accepted
Version: 0.2.0
Authors: Thulasiram K
Created: 2026-07-26
Last Updated: 2026-07-26
Depends On: RFC-0016, RFC-0017, RFC-0018, RFC-0019, RFC-0021, RFC-0022
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0021 exposed a repository's history, RFC-0022 its content, RFC-0023 its
changes. Each answers a question about a *known* location — this ref, this path,
this commit. This document adds the inverse: **finding** the location from what
you remember of it. It answers "where is the commit / file / line that mentions
*this*?" — and does so, like every Hub read before it, as a thin projection over
the engine, adding no stored state of its own.

**The one-sentence scope:**

> RFC-0024 adds read-only search endpoints — `search/commits` (match a query
> against commit messages and authors across history), `search/paths` (match file
> names in the tree at a ref), and `search/content` (grep matching lines of text
> files at a ref) — under the existing repository path, authorized by the same
> RFC-0018 `read` capability. Each is a **live, bounded scan** of engine objects at
> request time: no index is built, no derived state is stored, and no engine
> behavior changes.

**Design stance — search is a scan, not an index.**

> A search result is not repository state; it is a *view* computed from state the
> `read` capability already exposes. RFC-0024 therefore does exactly what a clone
> holder could do locally — walk the commit graph, walk the trees, read the blobs,
> and match a literal query — but does it server-side and returns the hits. It
> builds **no persistent inverted index**: there is nothing to keep consistent,
> nothing to invalidate on push, nothing that can drift from the objects it
> describes. That keeps search on the right side of the engine freeze (a future
> index would be *derived* state like `graph.idx`, never authoritative — §14, S2).
> Because a scan of a large repository is unbounded work, the normative core (§7)
> is the set of budgets that make every request terminate: a traversal budget, a
> bytes-scanned budget, and a result cap, degrading to `truncated` rather than to a
> hang. And because matched bytes are repository content, they are served inert —
> JSON string data the UI renders, never a document on the Hub origin (§7, exactly
> as RFC-0022 §7 and RFC-0023 §7).

```
   GET /_vara/repositories/demo/search/content?q=TODO&ref=main
             │  authenticate → authorize `read` (RFC-0018, before any read)
             ▼
   resolve ref → tree ; walk tree objects  (entry budget)
             ▼   per text blob, within the bytes budget
   scan lines for the literal query → matches  →  JSON (inert, escaped line data)
             ▼   over any budget
                          truncated:true, partial results, 200
```

# 2. Motivation

Every question the Hub can answer so far starts from a location you already know.
But the most common real question is the opposite: you remember a phrase from a
commit message, a half-remembered file name, a string in the code — and you need
to find *where*. Without search, the only recourse is to clone and grep locally,
which defeats the point of a browsable Hub. The engine already stores everything
needed to answer these questions; RFC-0024 scans it on demand and returns the
matches. It needs no new capability, no new stored state, no engine primitive, and
no wire change — it is the natural fourth read after history, content, and diff.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **A persistent search index.** v1 scans live on every request. An inverted index
  (trigram / full-text) for sub-linear queries is future work (§14); it would be
  *derived* state, rebuilt from objects, never authoritative — the `graph.idx`
  model (RFC-0013), not a new source of truth.
* **Regular-expression queries.** v1 matches a **literal substring** (optionally
  case-insensitive). Go's `regexp` is RE2 (linear, no catastrophic backtracking),
  so a bounded regex mode is safe to add later (§14), but it is not in v1.
* **Cross-repository / global search.** v1 searches **one** repository (S6). A
  Hub-wide search across every repository a caller may read is future work.
* **Ranking / relevance scoring.** Results are returned in a deterministic order
  (§5), not sorted by relevance. Scoring is future work.
* **Fuzzy / stemmed / semantic matching.** v1 is exact substring only. Typo
  tolerance, stemming, and embedding-based semantic search (the latter an AI-layer
  concern, RFC-0011) are out of scope.
* **Any write.** RFC-0024 is strictly read-only; it stores nothing and mutates
  nothing.
* **Binary content.** `search/content` scans only text files (NUL-free and valid
  UTF-8, §7); binary blobs are skipped, never scanned, never emitted.
* **Engine / transport changes.** RFC-0024 adds no field to `pkg/*` and no RFC-0016
  wire message; the scan lives in `internal/hub`, above the engine.

# 4. Terminology

* **Query (`q`)** — the literal string to match. Matching is substring, and
  case-insensitive by default (§6); an empty query is `400`.
* **Scope** — what a given endpoint scans: commit metadata across *history*
  (`search/commits`), file names in the *tree* at a ref (`search/paths`), or the
  *text content* of files at a ref (`search/content`).
* **Match** — one hit: a commit (commit search), a path (path search), or a
  `(path, line)` pair (content search). A match's textual payload is inert line
  data, never a rendered document (§7).
* **Budget** — the per-request ceiling on work: commits walked, tree entries
  visited, and bytes scanned (§7). Exhausting a budget ends the scan with
  `truncated:true` and the results gathered so far.
* **Live scan** — the v1 strategy (S2): match by reading engine objects at request
  time, with no persistent index.

# 5. The Search API

All endpoints live under the repository path, require `read` (§8), take a required
`q`, and an optional `ref` (a branch name or commit id, default HEAD; RFC-0022 §6
resolution). Every response echoes the resolved `ref_commit` so the caller sees
exactly what was scanned even when a branch name moved, and a `truncated` flag that
is `true` when a budget (§7) ended the scan early. Results are deterministically
ordered so paging and re-fetching are stable.

## 5.1 Commit search (message & author, across history)

```
GET /_vara/repositories/{repo}/search/commits?q=<query>&ref=<ref>&in=message,author&limit=<n>&before=<id>&case=<sensitivity>
    → 200 {query, ref, ref_commit, matches:[{id, message, author, timestamp, parents}...], next, truncated}
```

Walks the commit graph from `ref` (default HEAD), newest first — the **same walk**
as RFC-0021 history (§5.3) — and returns commits whose message and/or author
contains `q`. `in` selects the fields to match (`message`, `author`, or both;
default both). Matches are the RFC-0021 `CommitSummary` shape verbatim (reuse, not
a new type). Paging follows the RFC-0022 file-history model (§5.4 there): `before`
and `next` are **underlying-walk** commit ids and the query filter is re-applied
after resuming, so pages never skip or repeat even though a page may traverse many
commits to gather `limit` matches. The walk is bounded by a **traversal budget**
(§7): if it is exhausted before `limit` matches are found, `next` is the id at which
to resume and `truncated` is `true` — the client continues rather than the server
scanning unbounded history in one call. As with every cursor here, `next` names the
first **un-examined** commit at the point the page ended (a full page or an
exhausted budget), so resuming re-walks up to it and neither skips nor repeats a
match — the RFC-0021/0022 cursor invariant, extended to the budget case.

## 5.2 Path search (file names in the tree)

```
GET /_vara/repositories/{repo}/search/paths?q=<query>&ref=<ref>&limit=<n>&case=<sensitivity>
    → 200 {query, ref, ref_commit, matches:[{path, blob, mode, is_dir}...], truncated}
```

Recursively walks the tree objects at `ref` (root downward) and returns entries
whose **full path** contains `q` — a file-name / path finder. Like RFC-0022's tree
listing (§5.1 there) this costs **tree reads only, never a blob read**: it matches
names, not content, so it is cheap. Each match carries the entry's `path`, object
`blob` id (content address), raw `mode`, and `is_dir`. The walk is bounded by an
**entry budget** and the result list by `limit` (§7); over either, `truncated` is
`true`. Matches are returned **sorted by path** (ascending byte order), independent
of how a tree object happens to store its entries, so the result order is stable
across requests.

## 5.3 Content search (grep over text files)

```
GET /_vara/repositories/{repo}/search/content?q=<query>&ref=<ref>&limit=<n>&case=<sensitivity>
    → 200 {query, ref, ref_commit, matches:[{path, blob, lines:[{line, content}...]}...], truncated}
```

Recursively walks the tree at `ref`, reads each **text** blob, and returns files
containing a line that matches `q`, with the matching lines. This is the one
endpoint that reads blob content, so it is the most heavily bounded (§7): a **file
is skipped** if it is binary (a NUL byte or invalid UTF-8, §7) or larger than the
**per-file size cap**; the scan stops when the **bytes-scanned budget**, the
**result cap** (`limit` files), or the **per-file line cap** is reached, setting
`truncated:true` with the partial results. Each match lists the file `path`, its
`blob` id, and up to the per-file line cap of matching `lines`, each a `line`
number (1-based) and the inert `content` of that line (no surrounding context in
v1, §14). Files are returned **sorted by path**; a binary or oversize file is
simply absent (it is not an error and not reported as a match).

# 6. Query Semantics & Resolution

* **Literal substring.** `q` matches as a plain substring, not a pattern: a match
  is any occurrence of `q` within the target text. There are no metacharacters; a
  query of `a.b` matches the three literal characters (D-analogue: no regex engine
  runs, §7). Regex is reserved (§3, §14).
* **Case.** Matching is **case-insensitive by default** (both query and target
  lower-cased before comparison — ASCII and the common Unicode cases, not full
  locale-aware collation). `?case=sensitive` requests an exact byte match;
  `?case=insensitive` is the explicit default. An unknown `case` value is `400`.
* **Empty / whitespace query.** An empty `q` (or, for the substring semantics,
  a `q` the server cannot use) is `400 MALFORMED_REQUEST`; there is no "match
  everything" query — use RFC-0021/0022 listings for enumeration.
* **Ref resolution.** `ref` resolves as a branch name or commit id exactly as
  RFC-0022 §6 (an unresolvable ref is `404`); the response echoes `ref_commit`.
  Path and content search scan that commit's tree; commit search walks history from
  it. Path resolution within the tree walks tree **objects**, never the host
  filesystem (RFC-0022 B4 holds — the walk enumerates entries, so there is no path
  to escape).

# 7. Serving Search Safely (Normative Core)

Search reads repository bytes and returns fragments of them, so it carries the
**same same-origin risk as RFC-0022 §7 / RFC-0023 §7**, plus a **resource risk**
unique to it: a naive scan of a large repository is unbounded work. Both are
mitigated normatively.

* **Results are data, never a document.** Every textual payload — a commit message,
  a path, a matching code line — is returned inside a JSON string (`content`,
  `message`, `path`), so the UI renders it into a result view it controls; the
  server never hands matched bytes to the browser as a page. There is no "raw
  search result" endpoint. This is RFC-0022 B5 / RFC-0023 D4 applied to search.
* **Text vs binary reuses the shared rule.** `search/content` scans a blob only if
  it is NUL-free **and** valid UTF-8; a binary blob is skipped before any byte is
  matched or emitted, so non-text bytes never enter a JSON string. Path and commit
  search never touch blob content at all.
* **Bounded on every axis** — the defining property. Each endpoint carries an
  explicit budget and degrades to `truncated:true` (a `200` with partial results),
  never a hang and never an unbounded response:
  * `search/commits` — a **traversal budget** (max commits walked per request);
    over it, return `next` to resume.
  * `search/paths` — an **entry budget** (max tree entries visited) and a **result
    cap** (`limit`).
  * `search/content` — a **per-file size cap** (larger files skipped), a
    **bytes-scanned budget** across the whole request, a **per-file line cap**, and
    a **result cap** (`limit` files).

  Matching is literal substring (linear scan), never a backtracking pattern, so the
  match itself cannot blow up; the budgets bound how much text is scanned, so no
  request can be made to run unbounded work.
* **Search is a scan with no stored state** (S2). It reads engine objects live and
  builds nothing persistent: no index file, no cache of results, nothing to
  invalidate on push or keep consistent with the objects. A search response is a
  pure function of the objects it scanned and the query, which is exactly why it can
  be cached by content address (§9) without a coherence problem.

These rules are why a search result, like a blob or a diff, is served only as
inert, escaped data the UI renders — never as executable content on the Hub origin
— and why a search over an arbitrarily large repository still terminates.

# 8. Authorization

Every search endpoint requires the RFC-0018 **`read`** capability on the
repository, evaluated **before** any object is read — the same capability and
ordering as RFC-0021, RFC-0022, RFC-0023, and `clone`. Search reveals only what a
clone the same `read` grants would: it returns fragments of commits, trees, and
blobs the caller may already read in full. No new capability is introduced (S1).
Because v1 is repo-scoped (S6), authorization is the ordinary per-repository `read`
check; a future global search (§14) would additionally have to filter results to
the repositories each caller may read, evaluated the same way per repository.

# 9. Caching

A search response is a pure function of the objects scanned and the query, so it
has a natural strong `ETag`:

* **commit search** by `(ref_commit, in, case, q, limit, before)` — history from a
  commit is immutable, so a repeated query over an unchanged head re-validates for
  free;
* **path search** by `(ref tree id, case, q, limit)` — the tree at a commit is
  immutable and content-addressed;
* **content search** by `(ref tree id, case, q, limit)` — likewise a pure function
  of the tree's blobs.

`If-None-Match` → `304` exactly as RFC-0021/0022/0023. Because there is no index
(S2), there is no staleness to reason about: the ETag changes precisely when the
scanned objects or the query change.

# 10. HTTP Binding

New routes (all `GET`, all require `read`):

```
GET /_vara/repositories/{repo}/search/commits    ?q=<query>&ref=<ref>&in=<fields>&limit=<n>&before=<id>&case=<sensitivity>
GET /_vara/repositories/{repo}/search/paths       ?q=<query>&ref=<ref>&limit=<n>&case=<sensitivity>
GET /_vara/repositories/{repo}/search/content     ?q=<query>&ref=<ref>&limit=<n>&case=<sensitivity>
```

All three are scoped under the literal `search/` prefix, so they do not collide
with the RFC-0022 `tree`/`blob`/`raw`, the RFC-0023 `diff`/`commits/{id}/diff`, or
the RFC-0021 `commits/{id}` routes (ServeMux most-specific-wins, as RFC-0022 §10).

DTOs (added to `internal/protocol`): `SearchCommitsResponse` (reusing the RFC-0021
`CommitSummary` for `matches`), `SearchPathsResponse` + `PathMatch`
(path, blob, mode, is_dir), `SearchContentResponse` + `ContentMatch`
(path, blob, lines) + `ContentLine` (line, content).

Status codes:

| Situation | Status | Code |
|-----------|--------|------|
| OK (possibly `truncated`) | 200 | — |
| Not modified (ETag matched) | 304 | — |
| Unknown repo / unresolvable ref | 404 | `UNKNOWN_REPOSITORY` / `NOT_FOUND` |
| Empty query, unknown `in`/`case` value, or malformed ref | 400 | `MALFORMED_REQUEST` |
| Unauthenticated | 401 | `UNAUTHENTICATED` |
| Lacks `read` | 403 | `UNAUTHORIZED` |

There is **no `413`**: search never rejects on size — it truncates. The Hub API
version stays `1` (X-VARA-API): these routes are additive to the v1 surface.

# 11. Architectural Constraints (Normative)

Inherits RFC-0021 H1–H9, RFC-0022 B1–B7, and RFC-0023 D1–D7 (project the engine,
read before act, additive JSON, same-origin, error-schema reuse, object-graph
paths, inert bytes, bounded reads, engine unchanged). RFC-0024 adds:

* **S1 — Read is the only capability.** Every search endpoint requires `read`
  (RFC-0018), checked before any object is read; no search-specific capability.
* **S2 — Search is a live scan, not an index.** v1 answers every query by reading
  engine objects at request time and building **no persistent state** — no index,
  no result cache, nothing to invalidate or keep coherent with the objects. A
  future inverted index (§14) is permitted only as *derived* state, rebuilt from
  objects and never authoritative (the RFC-0013 `graph.idx` model); it may never
  become a second source of truth about repository content.
* **S3 — Read-only.** RFC-0024 defines no write and stores nothing; repository
  content changes only by pushing commits (RFC-0016).
* **S4 — Results are inert on the app origin.** Every matched fragment (message,
  path, code line) is JSON string data the UI renders, never a document; there is
  no raw-result endpoint, and a binary blob is never scanned or emitted (extends
  RFC-0022 B5 / RFC-0023 D4).
* **S5 — Bounded on every axis.** Each endpoint has an explicit traversal / bytes /
  result budget and degrades to `truncated`, never a hang or unbounded response;
  matching is literal substring (linear), never a backtracking pattern (§7).
* **S6 — Repo-scoped.** v1 searches one repository (its history for commit search,
  its tree at a ref for path/content search); cross-repository global search is
  future work, and would filter per-repository by `read` (§8, §14).
* **S7 — Engine unchanged.** RFC-0024 adds nothing to `pkg/*` and no RFC-0016 wire
  message. The scan reuses the existing read primitives (`graph.Walker`, the object
  store, tree objects) from `internal/hub`; a bounded recursive tree walk is new
  **read-only** projection code above the engine, importing no binding layer.

# 12. Security Considerations

* **Same-origin result content is a primary risk** and §7/S4 are its mitigation:
  matched lines and paths are inert JSON data, there is no raw-result document, and
  a binary blob is excluded from content search. A repository's `.html`/`.js` can
  never run on the Hub origin through a search result, just as it cannot through a
  blob (RFC-0022 §12) or a diff (RFC-0023 §12).
* **Resource exhaustion is the risk unique to search** (S5): a scan of a large
  repository is real work. The budgets in §7 bound it on every axis — commits
  walked, tree entries visited, bytes scanned — so a query against a huge history
  or a huge tree degrades to `truncated` with partial results, never a hang. Literal
  substring matching (not backtracking regex) means the match step itself is linear
  and cannot be made pathological by the query.
* **No index means no coherence surface** (S2): because search stores nothing, there
  is no index that can drift from the objects, leak deleted content, or serve a
  subject stale results after a permission change — every response is computed fresh
  from the objects the `read` check just authorized.
* **No information beyond clone** (S1/S6): a search returns fragments of commits,
  trees, and blobs the caller may already read in full; it exposes nothing a clone
  of the same repository would not, and it is scoped to one repository the caller is
  authorized to read.

# 13. Testing Strategy

* **Commit search (§5.1)** — a query matches commits by message and by author;
  `in=message` and `in=author` scope the fields; results are the reachable matching
  commits newest-first; a query matching nothing is a `200` with empty `matches`;
  paging over the underlying walk yields every match once across pages with no skip
  or repeat; exhausting the traversal budget sets `truncated` and returns a usable
  `next`.
* **Path search (§5.2)** — a query matches file paths anywhere in the tree
  (nested included), returns `blob`/`mode`/`is_dir`, costs **no blob read**, orders
  by path; over the entry budget or `limit`, `truncated` is set.
* **Content search (§5.3)** — a query matches lines of text files with correct
  1-based `line` numbers; a binary file (NUL side and NUL-free-but-invalid-UTF-8
  side) is skipped, never scanned, never emitted; an oversize file is skipped;
  exhausting the bytes budget, the per-file line cap, or `limit` sets `truncated`
  with partial results.
* **Query semantics (§6)** — matching is literal substring (a query with regex
  metacharacters matches them literally); case-insensitive by default,
  `case=sensitive` exact; an empty `q` and an unknown `in`/`case` value are `400`.
* **Authorization (S1)** — a caller without `read` gets 403 from every search
  endpoint; with `read`, 200; 401-vs-403 never swapped.
* **Caching (§9)** — each endpoint carries the specified content-addressed ETag;
  `If-None-Match` → 304; changing `q`/`case`/`limit` (or the scanned commit/tree)
  changes the ETag.
* **Architecture (S2/S7)** — the engine diff is empty; `internal/hub` gains a
  bounded recursive tree walk and the scan routines but imports no binding layer and
  builds no persistent index; the import test still passes.

# 14. Future Work

* **Persistent inverted index** — a trigram / full-text index (rebuilt from
  objects, derived like `graph.idx`, never authoritative — S2) for sub-linear
  queries over large repositories, retiring the live scan for indexed refs while
  keeping it as the fallback.
* **Regular-expression queries** — a bounded RE2 (`regexp`) mode; safe because Go's
  engine is linear, gated behind the same budgets (§7) as substring search.
* **Cross-repository / global search** — search every repository a caller may read
  (S6), filtering results per repository by the `read` capability (§8).
* **Ranking & relevance** — score and order matches by relevance rather than the
  deterministic history/path order of v1.
* **Line context** — surrounding lines around a content match (like a diff's
  context), served inert (§7), for a richer result view.
* **Path-scoped search** — a `path=` prefix to restrict path/content search to a
  subtree, and a date / author range to restrict commit search.
* **Semantic search** — embedding-based matching (the AI layer, RFC-0011),
  distinct from the exact matching here.
