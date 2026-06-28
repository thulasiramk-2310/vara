# VARA

**VARA** is an RFC-driven, transactional, content-addressed distributed version
control engine written in Go.

It is designed from first principles with a focus on correctness guarantees,
structured recovery, and an architecture that supports future AI-assisted
development workflows.

> **Status**: v0.1.0-alpha. Core engine is feature-complete for local operations.
> Remote protocol and AI layer are not yet implemented.

---

## Features

- **Content-addressed object store** — SHA-256, zstd compression, immutable blobs
- **Transactional mutations** — write-ahead journal, atomic rename, crash recovery
- **Three-way merge** — Myers O(ND) diff, diff3 line-level merge, conflict markers
- **Commit Graph Index** — RFC-0013; 16 ms history on 10,000 commits (was 75.8 s)
- **Three-layer undo** — journal rollback → reflog restore → snapshot archive
- **Repository verification** — `vara verify` checks objects, trees, DAG, refs, index
- **Hierarchical locking** — deadlock-free by construction (RFC-0006)
- **Parser fuzzing** — ref names, journal, commit objects, tree blobs, garbage inputs

---

## Quick Start

```sh
# Initialize a repository
vara init

# Stage files
vara add .

# Commit
vara commit -m "initial commit"

# Create and switch branches
vara branch feature
vara switch feature

# Merge
vara switch main
vara merge feature

# Undo the last change (three-layer: journal → reflog → snapshot)
vara undo

# Verify repository integrity
vara verify
```

---

## Installation

```sh
git clone https://github.com/thulasiramk-2310/vara
cd vara
go build -o vara ./cmd/vara
```

Requires Go 1.21 or later.

---

## Architecture

VARA is built around a strict layered architecture where lower layers never
import higher layers:

```
cmd/vara          — argument parsing, command dispatch only
    ↓
internal/commands — command implementations (RFC-0012)
    ↓
internal/transaction, internal/locking — RFC-0006
    ↓
internal/repository — RFC-0003
    ↓
pkg/refs, pkg/reflog — RFC-0004
    ↓
pkg/graph, pkg/graphindex — RFC-0007, RFC-0013
    ↓
pkg/index, pkg/scanner — RFC-0005
    ↓
pkg/object — RFC-0002
    ↓
pkg/hash, pkg/types — cryptographic primitives
```

Every package maps to one or more RFCs. See [`docs/`](docs/) for the full
specification.

---

## RFC Index

| RFC | Title | Status |
|-----|-------|--------|
| [0002](docs/VARA-RFC-0002-OBJECT-FORMAT.md) | Object Format | Accepted |
| [0003](docs/VARA-RFC-0003-REPOSITORY-LAYOUT.md) | Repository Layout | Accepted |
| [0004](docs/VARA-RFC-0004-REFERENCES.md) | References | Accepted |
| [0005](docs/VARA-RFC-0005-INDEX.md) | Index | Accepted |
| [0006](docs/VARA-RFC-0006-LOCKING.md) | Locking & Transactions | Accepted |
| [0007](docs/VARA-RFC-0007-COMMIT-GRAPH.md) | Commit Graph | Accepted |
| [0008](docs/VARA-RFC-0008-MERGE-ALGORITHM.md) | Merge Algorithm | Accepted |
| [0009](docs/VARA-RFC-0009-UNDO-RECOVERY.md) | Undo & Recovery | Accepted |
| [0012](docs/VARA-RFC-0012-COMMAND-SPECIFICATION.md) | Command Specification | Accepted |
| [0013](docs/VARA-RFC-0013-COMMIT-GRAPH-INDEX.md) | Commit Graph Index | Accepted |

---

## Performance

Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS:

| Command | Result | Budget |
|---------|--------|--------|
| `vara init` | 2.8 ms | 20 ms ✅ |
| `vara commit` | 15.4 ms | 200 ms ✅ |
| `vara switch` | 220 ms | 500 ms ✅ |
| `vara history` (10k commits, warm) | **16.3 ms** | 100 ms ✅ |

---

## Testing

```sh
# Unit and integration tests
go test ./...

# With race detector
go test -race ./...

# Benchmarks
go test ./benchmarks/commands/ -bench=. -benchtime=1x

# Fuzz (30 seconds)
go test ./tests/fuzz/ -fuzz=FuzzRefName -fuzztime=30s
```

---

## Documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — start here: layer map, data flows, key invariants
- [`docs/`](docs/) — RFC specifications
- [`docs/ADR/`](docs/ADR/) — Architecture Decision Records
- [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md) — stable vs internal components
- [`docs/IMPLEMENTATION-STATUS.md`](docs/IMPLEMENTATION-STATUS.md) — what's built

---

## License

MIT — see [LICENSE](LICENSE).
