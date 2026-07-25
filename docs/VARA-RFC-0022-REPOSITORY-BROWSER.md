VARA RFC: 0022
Title: Repository Browser
Status: Accepted
Version: 0.2.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-25
Depends On: RFC-0016, RFC-0017, RFC-0018, RFC-0019, RFC-0021
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0021 gave the Hub a read API for a repository's *history* — branches,
commits, a summary. It deliberately stopped short of *content*: it exposes no way
to list a directory or read a file. This document adds exactly that — the file
explorer — as another thin projection over the engine: **tree listings, file
contents, and a file's history**. It answers "what is *in* this repository, at
this ref, at this path?"

**The one-sentence scope:**

> RFC-0022 adds read-only content-browsing endpoints — `tree` (list a directory),
> `blob` (read a file), `raw` (download a file's bytes), and a `path` filter on
> the RFC-0021 commit history — under the existing repository path, authorized by
> the same RFC-0018 `read` capability, projecting the object store and tree
> objects without reinterpreting them. It defines no mutation (files are pushed,
> never edited via the API) and no engine behavior.

**Design stance — the same projection invariant as RFC-0021.**

> A tree listing is the entries of a `Tree` object; a file is the bytes of a
> `Blob`; a file's history is the RFC-0021 walk filtered to commits that changed a
> path. The server resolves a path *within a tree* (never on the host
> filesystem), reads the object store, and shapes JSON. It adds no traversal or
> storage logic and nothing to `pkg/*`. RFC-0021's H1–H9 hold here unchanged, and
> this RFC adds only browse-specific constraints (§11).

```
   GET /_vara/repositories/demo/tree/src?ref=main
             │  authenticate → authorize `read` (RFC-0018, before any read)
             ▼
   resolve ref → commit → tree ; walk "src" within the tree (not the filesystem)
             ▼
   list the subtree's entries  →  JSON  (a projection of the Tree object)
```

# 2. Motivation

A user looking at a repository in the Hub can currently see its history and
branches but cannot open a directory or read a file — the single most basic thing
a code host does. The file explorer needs three reads the engine already
supports: list a tree, read a blob, and (for a file page) show the commits that
touched it. Each is a projection of existing objects; none needs a new capability,
a new engine primitive, or a wire change. RFC-0022 is the smallest layer that
turns "I can see the history" into "I can browse the code."

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **Diffs and comparison.** Showing what changed between two commits or branches
  is RFC-0023. RFC-0022 reads content at a single ref, not deltas.
* **Search.** Finding a file or a string is RFC-0024.
* **Any write.** Files are created by pushing commits (RFC-0016), never edited,
  uploaded, or deleted through this API. RFC-0022 is strictly read-only.
* **Syntax highlighting, markdown/notebook rendering, image thumbnails.** Those
  are the UI's concern; the API returns bytes and a type, not rendered HTML.
* **Symlink following, submodules, LFS pointers.** v1 lists what the tree object
  contains, as it contains it; richer semantics are future work.
* **Blame (line-level authorship).** A file's *commit* history is in scope; per-
  line blame is future work (§14).
* **Engine / transport changes.** RFC-0022 adds no field to `pkg/*` and no
  RFC-0016 wire message; it reads existing tree and blob objects.

# 4. Terminology

* **Tree** — an `object.Tree`: a directory, a list of entries each naming a child
  (a sub-tree or a blob) with a mode. Listing a tree is projecting these entries.
* **Blob** — an `object.Blob`: a file's content bytes, content-addressed by id.
* **Ref** — a branch name or a commit id, supplied as the `ref` query parameter
  (default HEAD), naming the commit whose tree a path is resolved in.
* **Path** — a slash-separated path resolved *within* the ref's tree, segment by
  segment. It is never a host filesystem path (§6).
* **Browse endpoints** — the RFC-0022 `tree` / `blob` / `raw` endpoints and the
  `path` filter on RFC-0021 `commits`.

# 5. The Browse API

All endpoints live under the repository path, require `read` (§8), and take the
target commit as a `ref` query parameter (default HEAD) so a path — which may
contain slashes — is never ambiguous with a branch name that also contains slashes
(§6).

## 5.1 Tree listing

```
GET /_vara/repositories/{repo}/tree/{path...}?ref=<name>
    → 200 {ref, commit, path, entries:[{name, type, mode, id}...]}
```

Lists the entries of the tree at `path` in `ref` (empty path = the root tree).
Each entry carries its `name`, the octal `mode`, the child object `id`, and a
`type` **derived from the mode**: the subtree sentinel `0o040000` is the only
`dir`; every other mode is a `file`. The raw `mode` still distinguishes a regular
file (`0o100644`) from an executable (`0o100755`), and the client reads that there.
The engine defines no symlink or gitlink mode today, so the split is exactly
two-way (§3 defers richer entry semantics). Entries are returned in name order —
the tree's own stored order, which `object.NewTree` sorts lexicographically. A path
that is not a directory is `404`.

**A tree listing costs exactly one object read** — the tree itself. It
deliberately omits per-entry file `size`: size is `len(Blob.Data)`, knowable only
by loading each child blob, so including it would turn one listing into an
O(entries) read and violate B6. A client that needs a file's size gets it from the
`blob` or `raw` response when it opens that file (§5.2). Per-entry size in a
listing is deferred to future work (§14).

## 5.2 File content

```
GET /_vara/repositories/{repo}/blob/{path...}?ref=<name>
    → 200 {ref, commit, path, id, size, binary, encoding, content, truncated}
```

Returns the file at `path`. It is served **inline as text** only when it is both
NUL-free **and** valid UTF-8, and within the inline cap (§7); then `encoding` is
`utf-8` and `content` is the text. Otherwise — a NUL byte, an invalid-UTF-8
sequence, or over the cap — `content` is omitted, `binary` and/or `truncated` is
set, `encoding` is `binary`, and the client fetches the bytes from `raw` (§5.3).
The UTF-8 requirement matters because `content` is a JSON string: NUL-free but
non-UTF-8 bytes would corrupt or break the encoding, so such a file is treated as
binary. `id` is the blob id (its content address); `size` is the byte length. A
path that is a directory, or does not exist, is `404`.

## 5.3 Raw file bytes

```
GET /_vara/repositories/{repo}/raw/{path...}?ref=<name>
    → 200 <raw bytes>
```

Streams a file's exact bytes — for downloading, or for the UI to render an image.
**Raw content is always served defensively** (§7): a neutral `Content-Type`
(`text/plain` for text, `application/octet-stream` otherwise), `X-Content-Type-
Options: nosniff`, and `Content-Disposition: attachment` for non-text — so
attacker-controlled repository content can never execute as HTML/JS on the Hub's
origin. This is the security crux of RFC-0022 (§7, §12).

## 5.4 File history (a `path` filter on RFC-0021 commits)

```
GET /_vara/repositories/{repo}/commits?ref=<name>&path=<path>&limit=&before=
    → 200 {commits:[...], next?}
```

RFC-0022 adds an optional `path` parameter to the RFC-0021 `commits` endpoint:
when present, only commits that **changed** `path` are returned — a file's history.
A commit changed `path` when the blob id at `path` differs from the blob id at the
same path in the commit's **first parent** (an added or deleted path is a change:
present-then-absent or absent-then-present). Two boundary rules make this
deterministic:

* **Root commit** (no parents): a path present in it counts as its introduction, so
  history terminates at the commit that first added the file.
* **Merge commit**: only the first parent is compared — the standard first-parent
  simplification (as `git log -- <path>` uses). A change that entered solely
  through a non-first parent is attributed to the merge commit, not hidden.

This is additive to RFC-0021 (a new optional query field, H9); without `path` the
endpoint behaves exactly as before. **Cursoring is over the underlying walk, not
the filtered result**: `next` is a commit id in the raw RFC-0021 walk at which the
following page resumes, and the server re-applies the `path` filter after resuming
— so pages never skip or repeat. Because a rarely-touched path may require scanning
many commits to fill one page of `limit` matches, an implementation MAY bound the
scan per request and return the walk cursor it reached; the natural end of history
is the zero cursor, as in RFC-0021.

# 6. Path & Ref Resolution

A path is resolved **within the ref's tree**, never against the host filesystem:
starting at the commit's root tree, each segment selects the child entry of that
name; a non-final segment must be a sub-tree, the final segment is the target
(tree for `tree`, blob for `blob`/`raw`). A segment that does not exist, or a
non-final segment that is not a directory, is `404`. Because resolution walks tree
*objects*, a path like `../../etc/passwd` cannot escape anything — there is no
filesystem to escape to; `..` is simply an entry name that does not exist (B4).

`ref` is a query parameter so it may contain slashes (`feature/x`) without
colliding with the path in the URL. It resolves as a branch name or a commit id
(RFC-0021 resolution); an unresolvable ref is `404`. The default ref is HEAD; on an
unborn or empty repository HEAD does not resolve, so a default-ref browse returns
`404` — a deliberate choice (RFC-0021 projects an empty repository as a
mostly-zero *summary*, but a browse of content that does not exist is a not-found,
not an empty listing).

# 7. Serving Content Safely (Normative Core)

Serving repository bytes from the Hub's **same origin** (RFC-0021 §8) is the
sharp edge of RFC-0022: a repository may contain a hand-crafted `.html` or `.js`
file, and if the server returned it with an executable content type the browser
would run it *on the Hub's origin*. Therefore:

* **Blobs are never served with an executable or renderable content type on the
  app origin.** `raw` uses `text/plain` (text) or `application/octet-stream`
  (otherwise), always with `X-Content-Type-Options: nosniff`, and
  `Content-Disposition: attachment` for non-text so it downloads rather than
  renders (B5).
* **The `blob` JSON endpoint returns content as data, not as a document** — text
  in a JSON string, binary omitted — so the UI renders it into a text view it
  controls, never by handing raw bytes to the browser as a page.
* **Inline text requires NUL-free *and* valid UTF-8.** Binary-ness is decided
  conservatively: a NUL byte, or any invalid-UTF-8 sequence, marks the file binary
  and suppresses inline `content` — non-UTF-8 bytes would otherwise corrupt or
  break the JSON string. Only a file that is both NUL-free and valid UTF-8 is
  returned inline as text.
* **Two size thresholds, on purpose.** The `blob` JSON endpoint caps *inline*
  content (e.g. 1 MiB): over it, the response is still `200` with `truncated:true`
  and no `content`. The `raw` endpoint enforces a larger *hard streaming ceiling*
  (e.g. 50 MiB): over it, `413 TOO_LARGE`. Below that ceiling `raw` always streams
  the exact bytes regardless of the inline cap — it is the escape hatch a
  `truncated` client follows. So a single request can never force the server to
  buffer an unbounded object (B6).
* **Binary detection and truncation are a presentation decision in the server read
  layer, not the engine.** They never alter the blob id or its ETag (both remain
  the raw content address), so this neither reinterprets nor re-indexes the object
  (B2); the engine still stores and hashes exact bytes.

These rules are why a code host like this either serves user content from a
*separate* origin or, as here, serves it only as inert, non-executable,
attachment-dispositioned bytes. RFC-0022 takes the latter path and makes it
normative.

# 8. Authorization

Every browse endpoint requires the RFC-0018 **`read`** capability on the
repository, evaluated **before** any object is read — the same capability, and the
same ordering, as RFC-0021 and as `clone`. Browsing a repository grants exactly
what cloning it would; a caller without `read` gets `403` and learns nothing about
the contents. No new capability is introduced (B1).

# 9. Caching

Every browse response is content-addressed and so has a natural strong `ETag`:
a tree listing by the tree id, a blob by the blob id, a raw response by the blob
id. Because ids are content hashes, an unchanged file or directory re-validates
for free with `If-None-Match` → `304` (RFC-0021 §5.6). File history reuses the
RFC-0021 commit ETag.

# 10. HTTP Binding

New routes (all `GET`, all require `read`):

```
GET /_vara/repositories/{repo}/tree/{path...}     ?ref=<name>
GET /_vara/repositories/{repo}/blob/{path...}     ?ref=<name>
GET /_vara/repositories/{repo}/raw/{path...}      ?ref=<name>
GET /_vara/repositories/{repo}/commits            ?ref=&path=&limit=&before=   (path added)
```

DTOs (added to `internal/protocol`): `TreeEntryInfo`, `TreeResponse`,
`BlobResponse`. The commit DTOs are unchanged (the `path` filter changes which
commits are returned, not their shape).

The three `{path...}` wildcards are scoped under the literal `tree` / `blob` /
`raw` prefixes, so Go's `http.ServeMux` most-specific-wins routing leaves the
RFC-0021 fixed segments (`summary`, `branches`, `commits`, `commits/{id}`) intact —
the catch-all never shadows them. The two JSON endpoints (`tree`, `blob`) carry the
same `X-VARA-API` / `X-Request-ID` / `ETag` triple as RFC-0021 (the shared write
path). `raw`, a byte stream rather than JSON, sets `X-VARA-API`, its
content-addressed `ETag`, and the §7 safety headers (`Content-Type`,
`X-Content-Type-Options: nosniff`, and `Content-Disposition` for non-text), with no
JSON body.

Status codes:

| Situation | Status | Code |
|-----------|--------|------|
| OK | 200 | — |
| Not modified (ETag matched) | 304 | — |
| Unknown repo / ref / path / not a dir-or-file as required | 404 | `UNKNOWN_REPOSITORY` / `NOT_FOUND` |
| Blob too large to stream (over the hard ceiling) | 413 | `TOO_LARGE` |
| Bad limit / cursor | 400 | `MALFORMED_REQUEST` |
| Unauthenticated | 401 | `UNAUTHENTICATED` |
| Lacks `read` | 403 | `UNAUTHORIZED` |

`TOO_LARGE` is the one new code. The Hub API version stays `1` (X-VARA-API): these
routes are purely additive to the v1 surface (H9).

# 11. Architectural Constraints (Normative)

Inherits RFC-0021 H1–H9 (project the engine, read before act, additive JSON,
same-origin, error-schema reuse). RFC-0022 adds:

* **B1 — Read is the only capability.** Every browse endpoint requires `read`
  (RFC-0018), checked before any object is read; no browse-specific capability.
* **B2 — Projection, never reinterpretation.** A tree listing is the `Tree`
  object's entries; a blob is the `Blob` object's bytes; file history is the
  RFC-0021 walk filtered by path. The server builds no alternate index of tree
  contents and no private cache that could diverge (extends H8).
* **B3 — Read-only.** RFC-0022 defines no write. Repository content changes only
  by pushing commits (RFC-0016).
* **B4 — Paths resolve within a tree, never the filesystem.** Resolution walks
  tree objects segment by segment; `..` and absolute paths are ordinary
  non-existent entry names, not traversal — there is no host path to escape to.
* **B5 — Repository bytes are inert on the app origin.** `raw` serves a neutral
  content type with `nosniff` and (for non-text) an attachment disposition; the
  `blob` endpoint returns content as JSON data, never as a rendered document.
  Repository content can never execute on the Hub's origin (§7).
* **B6 — Bounded reads.** Inline blob content is size-capped; `raw` enforces a
  hard streaming ceiling (413 above it), so one request cannot buffer an unbounded
  object.
* **B7 — Engine unchanged.** RFC-0022 adds nothing to `pkg/*` and no RFC-0016
  wire message.

# 12. Security Considerations

* **Serving user content same-origin is the primary risk** and §7/B5 are its
  mitigation: inert content type, `nosniff`, attachment disposition. A repository
  `.html`/`.js`/`.svg` can never run on the Hub origin. (Even so, the session
  cookie is httpOnly and SameSite=Strict, so it could not be read even if script
  ran — defense in depth.)
* **Path resolution is object-graph, not filesystem** (B4), so there is no path
  traversal surface: a browse request can only reach objects reachable from the
  ref's tree, which is exactly what `read` already authorizes.
* **Resource bounds** (B6): inline caps and a raw ceiling prevent a single request
  from forcing an unbounded read; tree listings are naturally bounded by directory
  size.
* **No information beyond clone.** Everything browse exposes is present in a clone
  the same `read` capability grants; RFC-0022 reveals nothing new (B1).
* **Binary sniffing is conservative**: a NUL byte marks content binary, so text
  rendering is never fed non-text.

# 13. Testing Strategy

* **Tree listing** returns a directory's entries with correct `type`/`mode`/`size`
  for nested trees; the root and a sub-path both resolve; a file path to `tree` is
  404.
* **Blob** returns text content within the cap (`utf-8`), flags a binary file —
  both a NUL-containing file **and** a NUL-free but invalid-UTF-8 file — without
  inline content, and flags an over-cap file `truncated`; a directory path to
  `blob` is 404.
* **Raw** streams exact bytes with `text/plain`/`octet-stream`,
  `X-Content-Type-Options: nosniff`, and an attachment disposition for binary; an
  HTML file is served as `text/plain`+nosniff (never `text/html`) — the load-
  bearing B5 test.
* **Path safety (B4)** — `../../x`, absolute paths, and a non-directory mid-path
  all resolve to 404, reaching no object outside the tree.
* **File history (§5.4)** — with `path`, only commits that changed the path are
  returned, in order, cursored across the underlying walk; history terminates at
  the root commit that introduced the path, and a merge that changed the path only
  via a non-first parent is attributed to the merge (first-parent rule); without
  `path`, identical to RFC-0021.
* **Authorization (B1)** — a caller without `read` gets 403 from every browse
  endpoint; with `read`, 200. 401-vs-403 never swapped.
* **Caching** — tree/blob/raw carry a content-addressed ETag; `If-None-Match` →
  304.
* **Bounds (B6)** — an over-ceiling raw request is 413; inline content is capped.
* **Architecture (B2/B7)** — the engine diff is empty; `internal/hub` gains tree/
  blob reads but imports no binding layer (the H2/H8/B2 import test still passes).

# 14. Future Work

* **Blame** — per-line last-changed commit for a file.
* **Per-entry size in tree listings** — deferred from v1 because it costs one blob
  read per entry (§5.1); a future revision could add a bounded or cached size.
* **Symlinks, submodules, LFS** — richer entry semantics beyond raw tree contents.
* **Directory READMEs / rendered previews** — the UI may render markdown; a future
  RFC could offer a rendered-HTML projection from a *separate* origin (never the
  app origin, §7).
* **Archive download** — a tree as a tarball/zip at a ref.
* **Raw on a separate content origin** — should the Hub ever want to serve
  renderable content, it must come from a distinct origin (the RFC-0022 same-
  origin rule is why it does not today).
* **Diff Viewer (RFC-0023)** and **Search (RFC-0024)** build on these reads.
