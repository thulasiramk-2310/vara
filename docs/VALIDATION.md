# VARA Scale Validation

This document records the results of driving the **complete lifecycle** over an
authenticated HTTP server against a **rich** repository — wide/deep directory
trees, a mix of text and binary blobs, and real branch/merge topology — rather
than a synthetic linear history. It is engineering validation, not a benchmark
suite: the goal is to exercise every layer together and surface edge cases.

Reproduce with the env-gated harness:

```sh
VARA_SCALE=1 VARA_SCALE_N=1000 VARA_SCALE_FILES=64 VARA_SCALE_CLONES=4 \
  go test ./internal/commands/ -run TestScaleValidation -v -timeout 30m
```

## Results (N=250 commits, 64-file working set, 4 merges)

Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS. The repository has wide/deep
trees (`src/pkgN/modM/fileK.dat`), ~25% binary blobs, and a merge commit every
50 commits. All operations run over HTTP with **Basic auth + an RFC-0018 policy**
enforced.

| Stage | Result |
|-------|--------|
| generation | 250 commits, 4 merges, 3081 objects, 0.5 MiB `.vara` |
| **clone** (HTTP + auth) | **25.6 s**, +181 MiB alloc, heap ~256 MiB |
| **verify** (deep DAG audit, incl. merges) | **14.3 s** — healthy |
| history (graph-index fast path) | 46 ms |
| status | 496 ms |
| incremental fetch (+50 commits) | 2.8 s |
| **push** (authenticated fast-forward) | **405 ms** |
| gc dry-run | 419 ms |
| **4 parallel clones** (warm cache) | **9.2 s** — all OK |

## What was validated

- **The whole stack composes under load.** An authenticated, policy-gated HTTP
  server serves clone/fetch/push against a repository with merge topology and
  binary content, and `verify` reports the cloned DAG healthy — the commit graph,
  transfer enumeration, pack format, and integrity checker all handle real
  branch/merge history, not just a line.
- **CAS survives the network boundary (positive finding).** During harness
  development, pushing a commit built on a *stale* local head was correctly
  rejected as non-fast-forward — the server's compare-and-swap did exactly its
  job. The harness now pushes a legitimate fast-forward, which succeeds in
  ~0.4 s.
- **Concurrency is safe.** Four parallel authenticated clones all succeed.
- **`history` stays O(1)-ish** at 46 ms via the RFC-0013 graph index, independent
  of history depth.

## Findings worth tracking

1. **Clone is the dominant cost and is cold-I/O-bound.** A single cold clone of
   250 commits took ~25 s (~100 ms/commit), but **four concurrent clones finished
   in 9.2 s total** — i.e. warm-cache reads are ~10× faster. The bottleneck is
   per-object NTFS file reads on a cold cache (the same limit documented for
   `add`/`status`), not CPU or transfer logic.

2. **Clone memory grows with tree richness.** Heap peaked at ~256 MiB here versus
   the ~52 MiB flat profile of the old linear-history stress test. Richer trees
   mean more objects per commit, and the pack is currently read into memory
   (`io.ReadAll`) on both sides. This is a known trade-off and a concrete
   motivation for **RFC-0015 (pack optimization / streaming / delta packs)** — it
   is a scaling characteristic to improve, not a correctness bug.

3. **`verify` is thorough but linear in object count** (~14 s here). Fine for a
   deliberate integrity audit; not on any hot path.

## Not yet exercised at scale (future work)

- True conflicting-merge resolution over the wire (correctness is covered by the
  merge unit tests; this harness validates merge *topology*, not conflict UX).
- Repository rename/delete cycles under concurrent load.
- Very large single blobs and very wide single directories (10k+ entries).
- Genuinely huge histories (10k–100k commits) — bounded here by NTFS temp-dir
  cleanup time; raise `VARA_SCALE_N` on faster storage.
