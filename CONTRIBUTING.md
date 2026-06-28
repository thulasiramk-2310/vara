# Contributing to VARA

Thank you for your interest in contributing.

VARA is in early alpha. The core engine is stable; the remote protocol and AI
layer are not yet started. This guide explains how to contribute effectively.

---

## Before You Start

1. **Read the RFCs.** VARA is protocol-first. Every package maps to one or more
   RFCs in `docs/`. If a change contradicts an RFC, either the RFC or the change
   is wrong — clarify that before writing code.

2. **Read the ADRs.** `docs/ADR/` explains *why* key decisions were made. If you
   disagree with a decision, open an issue rather than working around it.

3. **Check existing issues.** To avoid duplicate work, search open issues before
   starting.

---

## Development Setup

```sh
git clone https://github.com/thulasiramk-2310/vara
cd vara
go build ./...
go test ./...
```

Requires Go 1.21+. No external build tools required.

---

## Architecture Rules (non-negotiable)

These rules are enforced by the import hierarchy and must not be broken:

```
cmd/vara → commands → transaction → locking → repository
                   → refs → graph → index → object → hash
```

- **Lower layers never import higher layers.** Adding an import that violates
  this hierarchy is a blocking review comment.
- **`cmd/vara` contains only argument parsing and dispatch.** No business logic.
- **Every new package must reference its governing RFC** in the package doc comment.
- **Mutations must use atomic rename.** Never write a file in place; always write
  to `path + ".tmp"` and rename.
- **The object store is append-only.** Never modify or delete a stored object.

---

## Running Tests

```sh
# All tests
go test ./...

# With race detector (required before opening a PR)
go test -race ./...

# Specific test
go test ./tests/integration/ -run TestMerge_Conflict -v

# Fuzz (30 seconds per target)
go test ./tests/fuzz/ -fuzz=FuzzRefName -fuzztime=30s

# Benchmarks
go test ./benchmarks/commands/ -bench=BenchmarkHistory10k -benchtime=1x
```

All tests must pass with `-race` before a PR is opened.

---

## Making Changes

### Bug fixes

1. Write a failing test that reproduces the bug.
2. Fix the bug.
3. Confirm the test passes.
4. Reference the bug in the commit message.

### New features

1. Determine which RFC governs the feature. If no RFC exists, discuss before
   implementing.
2. Implement the feature.
3. Add integration tests.
4. Update `docs/IMPLEMENTATION-STATUS.md`.

### New packages

1. Every package must reference its RFC in the package doc comment.
2. The package must not violate the import hierarchy.
3. The package must not import `internal/commands` or anything above it.

---

## Commit Messages

Use the imperative mood:

```
fix: scanner fingerprint fast path verifies content hash

The scanner returned StatusClean when ModTime matched the stored fingerprint
without checking the blob hash. On NTFS (100 ns resolution), two writes within
the same timestamp bucket produced the same ModTime, causing RunAdd to miss the
change.

Fix: always verify content when fingerprint matches.
```

Format: `<type>: <short summary>` followed by a blank line and a body if needed.

Types: `fix`, `feat`, `refactor`, `test`, `docs`, `bench`, `build`.

---

## RFC Changes

Accepted RFCs are stable. To change an accepted RFC:

1. Open an issue describing the problem.
2. Propose the amendment in the issue.
3. Once consensus is reached, update the RFC and increment its version.
4. Update all code that implements the changed section.

---

## Reviewer Expectations

- All public functions must have doc comments.
- No `//nolint` or `// TODO: fix this` comments in submitted code.
- Imports must be grouped: stdlib, then internal, then external.
- The linter (`go vet`) must pass cleanly.

---

## What We Are NOT Looking For

At this stage, please do not open PRs for:

- Remote protocol implementation (not yet designed)
- AI provider integration (not yet designed)
- Pack file support (deferred, see RFC-0014)
- GUI or IDE integrations
- `git clone` or `git push` compatibility

These are planned but require RFC discussions before implementation.

---

## Questions

Open an issue with the `question` label.
