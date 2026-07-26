<div align="center">

<img src="docs/assets/vara-demo.svg" alt="VARA terminal demo" width="700">

<br />
<br />

# VARA

**RFC-driven, transactional, content-addressed distributed version control platform written in Go.**

[![CI](https://github.com/thulasiramk-2310/vara/actions/workflows/ci.yml/badge.svg)](https://github.com/thulasiramk-2310/vara/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.4.0-blue)](https://github.com/thulasiramk-2310/vara/releases/tag/v0.4.0)
[![RFC Status](https://img.shields.io/badge/RFCs-24%20accepted-blue)](docs/)

</div>

---

## Why VARA?

In archaic usage, a *vara* is a measuring rod — a fixed reference from which all distances are taken. In VARA, every object is identified by its SHA-256 content hash: an immutable measure. Every repository state is a precise point relative to that measure.

VARA is built around a protocol-first architecture: SHA-256 object identity, zstd compression, transactional repository operations, and a commit graph index that makes history traversal O(1). The local engine is feature-complete in v0.1. Remote protocol, replication, and an AI-assisted workflow layer follow in subsequent releases.

---

## Features

| Feature | Details |
|---------|---------|
| **Immutable object store** | SHA-256 addressed blobs, trees, and commits. Content cannot change after storage. |
| **zstd compression** | All objects are compressed at rest. Faster than zlib at equivalent ratios. |
| **Write-ahead journal** | Six-phase transaction lifecycle. Crash at any point leaves the repository consistent. |
| **Hierarchical locking** | O_EXCL file locks acquired in fixed order. Deadlock-free by construction (RFC-0006). |
| **Repository verification** | Seven-phase integrity report: objects → trees → commits → DAG → refs → index → journal. |
| **Three-way merge** | Myers O(ND) diff, diff3 line-level merge, conflict markers for unresolvable regions. |
| **Three-layer undo** | Journal rollback → reflog restore → snapshot archive. Always a path to recovery. |
| **Commit Graph Index** | Binary `graph.idx` (RFC-0013). History on 10k commits: **16 ms** (was 75.8 s). |
| **Fuzz-tested parsers** | Ref names, journal entries, commit objects, tree blobs, and binary inputs. |
| **Cross-platform** | Tested on Linux, macOS, and Windows in CI across Go 1.21, 1.22, 1.23. |

---

## Architecture

Strict layered hierarchy. Lower layers never import higher layers.

```mermaid
graph TD
    CLI["CLI<br/>cmd/vara"] --> CMD["Commands<br/>internal/commands"]
    CMD --> TXN["Transaction<br/>internal/transaction"]
    CMD --> LOCK["Locking<br/>internal/locking"]
    TXN --> REPO["Repository<br/>internal/repository"]
    LOCK --> REPO
    REPO --> REFS["References<br/>pkg/refs · pkg/reflog"]
    REPO --> GRAPH["Commit Graph<br/>pkg/graph · pkg/graphindex"]
    REPO --> IDX["Index · Scanner<br/>pkg/index · pkg/scanner"]
    REFS --> OBJ["Object Store<br/>pkg/object"]
    GRAPH --> OBJ
    IDX --> OBJ
    OBJ --> PRIM["Primitives<br/>pkg/hash · pkg/compression"]
```

Every package maps to one or more RFC specifications. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for data flow diagrams, transaction state machine, and key invariants.

---

## Installation

VARA is a single static binary — no CGO, no runtime dependencies. The same
`vara` executable is both the client (like `git`) and the self-hosted Hub server.

**Prebuilt binary** (Linux · macOS · Windows, `amd64`/`arm64`) — one line:

```sh
curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh
```

Or grab an archive from the [releases page](https://github.com/thulasiramk-2310/vara/releases)
and put `vara` on your PATH.

**With the Go toolchain** (Go 1.21+):

```sh
go install github.com/thulasiramk-2310/vara/cmd/vara@latest
```

**From source:**

```sh
git clone https://github.com/thulasiramk-2310/vara
cd vara && go build -o vara ./cmd/vara
```

Verify with `vara --version`. Full install and self-hosting instructions —
including running your own Hub with Docker — are in
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

---

## Quick Start

```sh
vara init
vara add .
vara commit -m "initial commit"

vara branch feature
vara switch feature
vara commit -m "add feature"

vara switch main
vara merge feature

vara history          # sub-20ms on 10k commits via graph index
vara verify           # full integrity check
vara undo             # three-layer recovery: journal → reflog → snapshot
```

**Working with remotes** (RFC-0014, local transport):

```sh
vara clone ../upstream myrepo   # copy a repository and its history
vara remote add origin ../up    # register a remote
vara fetch origin               # update remote-tracking refs
vara pull origin main           # fetch + fast-forward or three-way merge
vara push origin main           # upload commits (fast-forward checked)
```

**`vara status`:**

```
On branch main

Changes staged for commit:

    modified:   src/engine.go

Untracked files:

    config.yaml
```

**`vara verify`:**

```
Repository Integrity Report
─────────────────────────────
Objects    ✔  4 verified
Trees      ✔  2 verified
Commits    ✔  2 verified
DAG        ✔  No cycles
Refs       ✔  2 valid
Index      ✔  Consistent
Result     Repository Healthy
```

---

## Run your own Hub

VARA includes a self-hosted Hub — a web UI plus read/management API over your
repositories, with accounts, sessions, and capability-based authorization.

```sh
docker compose up -d --build
docker compose exec hub vara account create \
  --accounts /data/accounts --policy /data/policy \
  --username admin --password 'choose-a-strong-password'
# open http://localhost:8080
```

Or run the binary directly against a data directory — see
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) for the data layout, systemd unit, and
TLS / reverse-proxy setup.

---

## Performance

Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS, Go 1.23.

| Command | Measured | Budget |
|---------|----------|--------|
| `vara init` | 2.8 ms | 20 ms ✅ |
| `vara commit` | 15.4 ms | 200 ms ✅ |
| `vara switch` | 220 ms | 500 ms ✅ |
| `vara history` (10k commits, warm) | **16 ms** | 100 ms ✅ |
| `vara add` (1k files) | 787 ms | 500 ms ⚠️ NTFS-bound |

The `vara history` warm path reads `graph.idx` directly — one file read, in-memory BFS, no object store traversal. Cold path rebuilds the index and caches it for all subsequent calls.

---

## Current Status

**v0.4.0** — the Hub can now read a repository the way a code host does: browse the file tree, read files, view a commit's diff, and **search** commits, file names, and content — all as thin, read-only projections over the unchanged engine. Built on the installable, self-hostable v0.3 platform. See [docs/RELEASE-v0.4.0.md](docs/RELEASE-v0.4.0.md).

| Component | Status |
|-----------|--------|
| Local repository engine | ✅ Complete |
| Transactional storage with crash recovery | ✅ Complete |
| Three-way merge and conflict detection | ✅ Complete |
| Commit Graph Index (RFC-0013) | ✅ Complete |
| Repository verification (`vara verify`) | ✅ Complete |
| Three-layer undo (`vara undo`) | ✅ Complete |
| Remote protocol — clone, fetch, pull, push (local transport, RFC-0014) | ✅ Complete |
| HTTP transport + self-hosted Hub (`vara serve --hub`) | ✅ Complete |
| Binary release artifacts (Linux/macOS/Windows) + Docker | ✅ Complete |
| Hub read UI — browser, diff viewer, search (RFC-0022–0024) | ✅ Complete |
| Pack file format and delta compression | 🚧 later |
| AI workflow layer | 🚧 later |

---

## Roadmap

| Version | Milestone |
|---------|-----------|
| **v0.1** | Local engine — all local commands, RFC-0013, `vara verify` ✅ |
| **v0.2** | Backend platform + Hub — HTTP transport, identity, authorization, repo management, accounts, read API + web UI (RFC-0016–0021) ✅ |
| **v0.3** | Distribution & self-hosting — prebuilt binaries, install script, Docker Hub image ✅ |
| **v0.4** (now) | Hub read UI — repository browser, diff viewer, search (RFC-0022–0024) ✅ |
| **v0.5** | Organizations, teams, pull requests & issues |
| **v1.0** | Stable — frozen wire format, frozen CLI, LTS commitment |

---

## Documentation

| Document | Description |
|----------|-------------|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Layer map, data flows, invariants — start here |
| [`docs/VARA-RFC-*.md`](docs/) | Formal RFC specifications (10 accepted) |
| [`docs/ADR/`](docs/ADR/) | Architecture Decision Records |
| [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md) | Stable vs internal API surfaces |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Dev setup, architecture rules, commit format |

---

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). Key constraints:

- Every new package must reference its RFC in the package comment.
- The import hierarchy is a hard constraint. A package that imports above its layer does not compile.
- `object.BlobHash(content)` is the single authoritative blob ID function — never compute manually.
- Use real repository temp directories in tests; do not mock the object store.

---

## License

MIT — see [LICENSE](LICENSE).
