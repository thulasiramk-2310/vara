# VARA v0.4.0 Release Notes

**Released**: 2026-07-26
**Tag**: `v0.4.0`
**Codename**: Reading the Repository

---

## What v0.4.0 Is

v0.3.0 made VARA installable and self-hostable. **v0.4.0 makes the Hub something
you can actually *read* a repository with.** Until now the web UI could list
branches, commits, and a summary — but it could not open a directory, read a file,
show what a commit changed, or find anything. This release adds all three of the
reads a code host is expected to have:

- **Repository Browser** (RFC-0022) — browse the file tree, read a file, download
  raw bytes, and view a file's history.
- **Diff Viewer** (RFC-0023) — a commit's changed-file summary and per-file unified
  diff, and comparison between two refs.
- **Search** (RFC-0024) — find commits by message or author, files by name, and
  matching lines of content — with the match highlighted, counted, and shareable
  by URL.

Every one of these is a **thin, read-only projection over the unchanged engine**:
the same layered discipline as the rest of the platform. The `pkg/*` engine and the
`internal/transport` interface still have an **empty diff** — mechanically enforced
by `tests/architecture`. No new capability, no wire change, no stored state.

---

## Highlights

| Area | What shipped |
|------|--------------|
| **Browser** | `tree` (list a directory), `blob` (read a file), `raw` (inert byte download), and a `path` filter on commit history — all content served **inert** on the Hub origin (neutral type, `nosniff`, attachment for non-text) so a repository's `.html`/`.js` can never execute there |
| **Diff** | `diff` summary (changed-file set), per-file unified hunks (a presentational line diff kept separate from the engine's merge diff), and a `commits/{id}/diff` convenience against the first parent |
| **Search** | `search/commits`, `search/paths`, `search/content` — a **live, bounded scan** (no index to build or invalidate), literal substring, case-insensitive by default, results inert and highlighted |
| **Web UI** | A refreshed design-system look plus new Files, commit-diff, and Search views; search highlights matches, shows result counts, and encodes its query in the URL for sharing |
| **Safety** | Every read is authorized by the RFC-0018 `read` capability **before** any object is read; every scan is bounded on every axis (traversal / bytes / results → `truncated`, never a hang) |

---

## Reading a repository

Point the Hub at your data directory and open it in a browser:

```sh
vara serve --hub ./web --root ./repos --policy ./policy --meta ./meta --accounts ./accounts
# open http://localhost:8080 → pick a repository → Files / History / Search
```

Or drive the read API directly (same `read` capability as `clone`):

```sh
curl -s .../_vara/repositories/demo/tree                 # list the root
curl -s .../_vara/repositories/demo/blob/src/main.go     # read a file
curl -s '.../_vara/repositories/demo/diff?head=main'     # what the tip commit did
curl -s '.../_vara/repositories/demo/search/content?q=TODO'   # grep the tree
```

---

## Architecture Note

The headline property of this release is what **didn't** change. Browser, Diff, and
Search are all new code in `internal/hub` (projection) and `internal/server`
(binding), layered strictly above the frozen engine:

- **Project the engine, never reinterpret it.** The changed-file set is
  `diff.DiffTrees` verbatim; the tree walk reads tree objects; the commit scan reads
  the commit graph.
- **The one genuinely new algorithm — the viewer's line diff — is deliberately
  separate** from the engine's merge Myers (RFC-0023 D2): a viewer optimizes for
  readable output, a merge for correct reconstruction, and the two must not be
  conflated.
- **Search builds no index** (RFC-0024 S2): it scans live, so there is nothing to
  keep consistent with the objects and nothing to invalidate on push. A future
  inverted index is reserved as *derived* state, never authoritative.

`git diff pkg/ internal/transport/` is empty across the entire release.

---

## Verification

- `go build ./...` and full `go test ./...` green, including `tests/architecture`
  (the import guards proving the projection layer touches no binding layer and the
  engine imports nothing above it).
- Engine freeze confirmed: empty `pkg/` + `internal/transport/` diff.
- All three read features verified live in a browser against a real repository
  (tree → file → commit diff; content / file / commit search with highlighting),
  and via `curl` (case-insensitive matching, `400` on empty query, `304`
  revalidation).

---

## Compatibility

Pre-1.0 still holds, and this release is purely additive: no on-disk format change
(RFC-0002/0005), no transport wire change (RFC-0016, `docs/COMPATIBILITY.md`), no
new capability. Repositories from every prior release read unchanged; the new routes
are additive to the v1 Hub API surface (`X-VARA-API: 1`).

---

## What's next

- **v0.5** — Organizations, teams, pull requests, and issues (collaboration, keyed
  by immutable repository ID).
- **Deferred, demand-driven** — a persistent search index (RFC-0024 §14), pack-file
  delta compression (RFC-0015), and the AI workflow layer (RFC-0011).
