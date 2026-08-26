# VARA

An RFC-driven, transactional, content-addressed distributed VCS in Go, plus a
web Hub (read UI) in `frontend/`. Go module: `github.com/thulasiramk-2310/vara`.

## Repo map
- **Engine** (`pkg/*`) — object/hash/index/graph/refs/diff/merge/... The v1 core.
- **Transport** (`internal/transport`) — Local + HTTP binding (RFC-0016). Interface frozen.
- **Server / platform** (`internal/server`, `internal/identity`, `internal/authz`,
  `internal/repomanager`) — auth (RFC-0017), authz (RFC-0018), repo lifecycle (RFC-0019).
- **CLI** (`cmd/vara`, `internal/commands`) — argument parsing + dispatch only.
- **Hub UI** (`frontend/`) — React+TS+Vite SPA. See `frontend/CLAUDE.md`.
- **Specs** (`docs/VARA-RFC-*.md`, `docs/ADR/`, `docs/ROADMAP.md`).

## Architecture invariants (do not break)
- **Lower layers never import higher layers.** Import order:
  commands → transaction → locking → repository → refs → graph → index → object → hash.
- **v1 freeze:** treat `pkg/*` and `internal/transport` (Local + Transport interface)
  as STABLE. Layer new capability *above* them; only touch them for a proven correctness bug.
  The HTTP client (`internal/transport/http.go`) may take additive client-only changes.
- **Access control lives above transport.** Pipeline: Authenticate → Authorize →
  Transport → Engine. Engine is identity- and authz-agnostic (no `if user.IsAdmin()`).
- `cmd/vara` contains ONLY arg parsing + dispatch. Every package maps to ≥1 RFC.
- Objects live under the `.vara/` root (NOT `.vara/objects/`) — use `object.NewStore(VaraDir)`.

## Frontend (Hub UI) — see `frontend/CLAUDE.md` for the full rules
Highlights that bite: BrowserRouter (server SPA fallback already in
`internal/server/static.go`); repo bytes are inert — **never `dangerouslySetInnerHTML`
for repository content**; repo addressed by name only (no owner namespaces);
self-hosted fonts (no CDN); **no violet/purple** (accent = teal, heatmap = git-green).

## Working agreements
- **No `Co-Authored-By` trailer on commits.**
- Branch before committing on `main`; commit/push only when asked.
- Detailed engine status + RFC ledger: auto-memory `project-vara.md`
  (`C:\Users\kthul\.claude\projects\D--version\memory\`). Frontend status: `project-vara-hub-ui.md`.
