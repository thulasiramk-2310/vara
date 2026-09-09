# VARA Hub — Frontend (`frontend/`)

React + TypeScript + Vite SPA that renders the VARA Hub read UI (GitHub-style) on
top of the v0.4 read APIs. **This is the front door to a frozen engine — read the
constraints before changing anything.**

## Stack
- React 19 + TypeScript 5.7 + Vite 6
- react-router-dom 7 (**BrowserRouter**, not HashRouter)
- No CSS framework — inline styles + CSS variables in `src/styles/index.css`
- No UI/data libraries beyond the above; keep it dependency-light and self-contained

## HARD CONSTRAINTS (never violate)
1. **Engine is frozen.** Never modify `pkg/*` or `internal/transport` (Local +
   Transport interface). The frontend is a thin adapter over the read API.
2. **Repository bytes are inert.** Blob/tree/diff/search content is untrusted.
   **NEVER use `dangerouslySetInnerHTML` for repository content** — rely on React's
   auto-escaping of text children. Highlight/search UI must build DOM nodes, not HTML strings.
3. **No owner namespaces.** Repositories are addressed by name only (`/r/:repo`).
   Owner/org namespaces are a v0.5 concern (RFC-0019 §14 deferred). No `:owner` segment.
4. **Self-contained — no CDN.** Fonts are self-hosted via `--font-sans` / `--font-mono`
   tokens. No Google Fonts, no external `<link>`/`<script>`. Assets live in `public/`.
5. **No violet / purple anywhere.** Accent is **teal** (`--accent`), secondary is
   **blue** (`--purple` token name is legacy — its *value* is blue). Contribution
   heatmap uses the **Git-green** scale. If you add color, stay out of the 255–330 hue band.
6. **BrowserRouter deep links already work** — the Go server (`internal/server/static.go`,
   `staticHandler`) serves `index.html` for unknown paths. No Go change needed for SPA fallback.

## Routing (`src/routes.ts`)
Repo by name: `repo`, `tree`, `blob`, `commits`, `commit`, `search`. Plus `login`,
`logout`, `dashboard`, `newRepo`, `profile`, `settings`. `useAppNavigation` derives
`{repo, section, inRepo, isLanding, go}` from the URL. **No owner segment anywhere.**

## Real API vs stubs
- **Wired to real v0.4 read APIs** (via `src/api/client.ts`): tree, blob, commits,
  diff, search, listRepos, summary, branches, whoami. All read pages
  (Code/Blob/Commits/Diff/Search) + Dashboard + Profile use live data.
- **Contribution graph + activity feed are REAL** (`src/hooks/useContributions.ts`):
  fetch repos → commits per repo → bucket by UTC day → 52×7 heatmap + activity list.
- **v0.5 stubs** (clearly marked, do NOT fake as real): Pull Requests, Issues,
  Notifications, Organizations, Profile ownership, Settings.

## Key components
- `src/components/common/Logo.tsx` — the VARA mark: teal A-frame "measuring rod"
  (converging strokes + apex node + graduation ticks). Inherits `currentColor`.
- `src/components/common/Identicon.tsx` — deterministic Git-style identicon from a
  seed (username/commit author). 5×5 mirrored grid, hue bands exclude purple.
  Used for header avatar, activity-feed rows, 120px profile avatar.
- `src/components/common/Contributions.tsx` — `ContributionGraph` + `ActivityFeed`.
- `public/vara-demo.svg` — animated terminal demo used in the landing "How it works"
  section (self-contained CSS `@keyframes`; rendered via `<img>`, no CDN).

## Build & verify
```
cd frontend
npx tsc --noEmit        # must be 0 errors
npx vite build          # must be clean → dist/
```
Live demo: `vara serve --hub ./frontend/dist` (server serves the built SPA + SPA fallback).
Keep `tsc` green and never introduce `dangerouslySetInnerHTML` for repo content.

## Watch out
- An external editor (v0/Cursor-like) has previously reverted real-API pages back to
  mock. If a read page suddenly imports mock `data.ts`, it was clobbered — re-wire to `api`.
- `src/types/index.ts` still holds original mock interfaces consumed by `data.ts`;
  don't trim it or the mock stub pages stop compiling.
