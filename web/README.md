# VARA Hub (web)

A minimal, framework-free single-page UI for VARA, served same-origin by the
VARA server (RFC-0021 Phase C). Serve it with:

    vara serve --root ./repos --policy ./policy --meta ./meta \
               --accounts ./accounts --hub ./web --addr :8080

Then open http://localhost:8080/ and log in. The UI uses the httpOnly session
cookie (never a token in JavaScript) and calls only the public `/_vara/*` API:

- login / logout          → /_vara/sessions
- dashboard, create       → /_vara/repositories
- overview                → /_vara/repositories/{repo}/summary
- history (paginated)     → /_vara/repositories/{repo}/commits
- branches                → /_vara/repositories/{repo}/branches
- settings (rename/delete)→ /_vara/repositories/{repo}[/rename]

It is intentionally v0.1: no file browser, diff viewer, or search (those are
reserved for RFC-0022/0023/0024).
