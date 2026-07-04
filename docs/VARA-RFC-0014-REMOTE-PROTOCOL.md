VARA RFC: 0014
Title: Remote Protocol
Status: Draft
Version: 0.1.0
Authors: Thulasiram K
Created: 2026-07-03
Last Updated: 2026-07-03
Depends On: RFC-0002, RFC-0003, RFC-0004, RFC-0007, RFC-0010
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

This document defines how two VARA repositories exchange objects and
references. It specifies the remote configuration model, the reference
namespaces used for tracking remote state, the transport abstraction, the
object-enumeration algorithm, the on-the-wire transfer format, and the
semantics of the `clone`, `fetch`, `pull`, and `push` operations.

**Key invariant:**

> A transfer is complete only if, for every reference advertised as updated,
> the receiving repository can resolve that reference's full object closure
> from its own object store. A partial transfer MUST NOT advance any
> reference.

This is the network analogue of RFC-0006's transactional guarantee: the
repository is never left pointing at an object it does not have.

# 2. Motivation

VARA v0.1 is a complete local engine but has no way to share history between
repositories. Collaboration requires a protocol to:

1. Discover which references a remote repository holds and where they point.
2. Compute the minimal set of objects one side lacks.
3. Transfer those objects with integrity verification.
4. Update references atomically, rejecting updates that would lose history.

This RFC defines that protocol for the **local filesystem transport**. A
future RFC (RFC-0016, Network Transport) layers a network transport over the
same negotiation and transfer primitives defined here; nothing in this
document assumes a local peer beyond the `Transport` interface in §6.

# 3. Terminology

* **Remote** — a named reference to another repository, stored in config as
  `[remote "<name>"]` with at least a `url` key.
* **Remote-tracking reference** — a local reference under
  `refs/remotes/<remote>/<branch>` that records where a remote branch pointed
  at the time of the last `fetch`. It is updated only by `fetch`/`pull`/`clone`
  and MUST NOT be moved by local commits.
* **Want** — a commit ID the local side wishes to obtain.
* **Have** — a commit ID the local side already possesses, offered to let the
  peer prune the object set it sends.
* **Object closure** — the transitive set of objects reachable from a commit:
  the commit itself, its tree, all subtrees, all blobs, and the same for every
  ancestor commit.
* **Fast-forward** — a reference update from commit `A` to commit `B` where
  `A` is an ancestor of `B`. Any other update is **non-fast-forward** and is
  rejected unless forced.

# 4. Remote Configuration

Remotes are stored in the repository config file (`.vara/config`, RFC-0010)
using INI sections:

```ini
[remote "origin"]
    url = /home/user/vara-repos/project
    fetch = +refs/heads/*:refs/remotes/origin/*
```

* `url` (required) — the location of the remote repository. For the local
  transport this is a filesystem path or a `file://` URL pointing at the
  **working root** of the remote repository (the directory containing
  `.vara`).
* `fetch` (optional) — a refspec (§5.1). When absent, the default
  `+refs/heads/*:refs/remotes/<name>/*` is assumed.

`vara remote add <name> <url>` writes this section. `vara remote remove <name>`
deletes it. `vara remote` (no args) lists configured remotes.

# 5. Reference Namespaces

VARA reference layout after cloning `origin`:

```
refs/heads/main                     local branch, moved by commit/merge
refs/remotes/origin/main            remote-tracking ref, moved by fetch
HEAD -> refs/heads/main             symbolic
```

`refs/remotes/*` obeys the same atomic-write rules as all references
(RFC-0004 §5). Remote-tracking references are never resolved as commit
parents and never appear in `vara branch` output; they appear only under
`vara branch --remotes` (future) and in fetch/pull reporting.

## 5.1 Refspecs

A refspec has the form `[+]<src>:<dst>`:

* `<src>` — a reference (or glob) on the remote side.
* `<dst>` — the local reference (or glob) to update.
* A leading `+` permits non-fast-forward updates for that mapping.

The default fetch refspec `+refs/heads/*:refs/remotes/origin/*` maps every
remote branch to the corresponding remote-tracking reference and always
allows it to move (remote-tracking refs are a mirror, not history the user
authored).

For v0.2 the implementation MAY support only the default wildcard refspec and
single explicit `refs/heads/<branch>` mappings; full glob refspec algebra is
deferred.

# 6. Transport Abstraction

All network-independent logic operates against a `Transport` interface:

```
Transport interface {
    // ListRefs advertises the peer's references and its HEAD target.
    ListRefs() ([]RefAdvertisement, error)

    // FetchPack returns a VPCK stream (§8) containing the closure of `wants`
    // minus everything reachable from `haves`.
    FetchPack(wants []CommitID, haves []CommitID) (io.ReadCloser, error)

    // ReceivePack ingests a VPCK stream into the peer's object store and then
    // requests the given reference updates, each guarded by an expected old
    // value for compare-and-swap semantics.
    ReceivePack(pack io.Reader, updates []RefUpdate) ([]RefUpdateResult, error)

    Close() error
}
```

* `RefAdvertisement{ Name string; Target CommitID }`
* `RefUpdate{ Name string; Old CommitID; New CommitID; Force bool }`
* `RefUpdateResult{ Name string; OK bool; Reason string }`

The **local filesystem transport** implements this interface by opening the
remote `.vara` directory directly: `ListRefs` reads the remote's ref store,
`FetchPack` enumerates against the remote's object store, and `ReceivePack`
writes into the remote's object store and ref store under the remote's own
locks (RFC-0006). Because objects are immutable and content-addressed,
writing an object that already exists is a no-op and is always safe.

# 7. Object Enumeration

Given a set of `wants` and `haves`, the sender computes the transfer set:

1. **Ancestor frontier.** Compute `boundary` = the set of all commits
   reachable from `haves` (via `graph.collectAncestors`). These are commits
   the receiver already has; the sender stops walking at them.
2. **Commit walk.** Starting from `wants`, walk parents breadth-first. Skip
   any commit in `boundary` and do not traverse through it. Collect the
   remaining commits as `sendCommits`.
3. **Tree/blob closure.** For each commit in `sendCommits`, add its tree.
   Recursively expand each tree: entries with mode `0o040000` are subtrees
   (recurse), all other entries are blobs (leaf). Collect every distinct
   object ID.
4. **Subtract objects the receiver already has.** Seed a `haveObjs` set with
   the tree closure of each `have` **tip** commit. Because a tree ID uniquely
   determines its entire subtree (content addressing), a tree present in
   `haveObjs` implies its whole closure is present, so enumeration prunes at
   the tree level. Any object in `haveObjs` is omitted from the send set. This
   makes the common case — pushing a few commits that touch a few files — send
   only the changed blobs, new trees, and new commits, at a cost of
   O(current tree) per have rather than O(history). Seeding from the tips (not
   every boundary commit) may occasionally re-send an object that existed only
   in older history; such duplicate writes are harmless no-ops. Full boundary
   subtraction and delta packs are deferred to RFC-0015 (Pack Optimization).

   The correctness invariant is one-directional: `haveObjs` MUST contain only
   objects the receiver provably has (closures of commits it advertised), so
   enumeration never omits an object the receiver lacks.

The result is a set of object IDs. Enumeration MUST be deterministic given the
same inputs (parents and tree entries are already ordered), so the produced
stream is reproducible.

# 8. Transfer Format (VPCK)

Objects are transferred as a **framed loose-object stream**. This reuses the
existing per-object zstd format (RFC-0002) rather than introducing delta
compression; delta packs are a performance milestone (RFC-0015), not a
correctness requirement.

```
Offset  Field
0       Magic        "VPCK"                     (4 bytes)
4       Version      uint8 = 1                   (1 byte)
5       ObjectCount  uint32 big-endian           (4 bytes)
9..     Records      ObjectCount × Record
end     Trailer      SHA-256 of bytes[0 .. end) (32 bytes)
```

Each **Record**:

```
[varint Length][Length bytes: zstd-compressed serialized object]
```

The serialized object is exactly the bytes `object.Store` writes to disk
(RFC-0002 header + payload, then zstd). The receiver, for each record:

1. Decompresses the payload.
2. Computes `SHA-256` over the decompressed bytes.
3. Uses that hash as the object ID and writes via `object.Store.Write`, which
   itself re-verifies identity on subsequent reads.

The 32-byte trailer is the SHA-256 of every preceding byte of the stream and
guards against truncation or corruption in transit. A stream whose trailer
does not match MUST be rejected and MUST NOT result in any reference update
(objects already written are harmless — they are unreferenced and immutable —
but no ref advances).

# 9. Operations

## 9.1 clone `<url> [<dir>]`

1. Create `<dir>` (default: last path segment of `<url>`) and `vara init` it.
2. Add remote `origin` with `url = <url>`.
3. `ListRefs` on the remote.
4. `FetchPack(wants = all advertised branch targets, haves = [])`.
5. Ingest the stream.
6. For each advertised branch `b -> C`, create
   `refs/remotes/origin/b -> C`.
7. Determine the remote HEAD target branch; create the matching local
   `refs/heads/<branch> -> C` and point `HEAD` at it.
8. Materialize the working tree from HEAD's commit.

## 9.2 fetch `<remote>`

1. `ListRefs`.
2. `haves` = the union of all local commit references (heads and existing
   remote-tracking refs).
3. `FetchPack(wants = advertised targets, haves)`.
4. Ingest.
5. Update `refs/remotes/<remote>/*` to the advertised targets. Remote-tracking
   refs use force semantics (§5.1); a rewritten remote branch overwrites the
   tracking ref.
6. Report per-branch old→new transitions. Local branches are **not** modified.

## 9.3 pull `<remote> [<branch>]`

`pull` = `fetch` followed by an integration of the fetched remote-tracking ref
into the current branch:

1. `fetch <remote>`.
2. Let `T = refs/remotes/<remote>/<branch>` (default: the current branch's
   name).
3. If the current branch is an ancestor of `T`: fast-forward the current
   branch to `T` and update the working tree.
4. Otherwise perform a three-way merge (RFC-0008) between the current branch
   and `T`, writing a merge commit or leaving conflict markers exactly as
   `vara merge` does.

## 9.4 push `<remote> <branch>`

1. `ListRefs` to learn the remote's current value of `refs/heads/<branch>`
   (call it `Old`; zero if the branch does not exist remotely).
2. Let `New` = local `refs/heads/<branch>`.
3. **Fast-forward check.** Unless `--force`, require that `Old` is an ancestor
   of `New` (or `Old` is zero). If not, abort with a non-fast-forward error
   and change nothing.
4. `haves` = the remote's advertised targets (so the sender omits objects the
   remote already has).
5. Build a VPCK stream for `wants = [New]`, `haves`.
6. `ReceivePack(stream, [{Name: refs/heads/<branch>, Old, New, Force}])`.
7. The remote writes objects, re-verifies the fast-forward condition under its
   own ref lock (compare-and-swap on `Old`), and updates the branch. The
   result is reported back per reference.

# 10. Atomicity & Failure Semantics

* Object ingestion is idempotent: objects are content-addressed and immutable,
  so re-sending or partially sending objects can never corrupt the store.
* Reference updates are the only mutation that can lose information, and they
  are performed **last**, only after the entire object stream has been ingested
  and its trailer verified.
* Each reference update is a compare-and-swap against `Old`. `ReceivePack`
  acquires the peer's **Refs lock** (`locks/refs.lock`, RFC-0006 §2) for the
  duration of the update phase, so the read→check→write CAS is atomic against
  concurrent pushes. Two pushers racing on the same branch are serialized: the
  second acquires the lock only after the first releases it, re-reads the
  now-updated ref, and its stale CAS is rejected. A concurrent update between
  `ListRefs` and `ReceivePack` therefore causes exactly one winner; the loser
  MUST report the rejection and MUST NOT retry blindly.
* Object ingestion runs *before* the lock, since content-addressed writes are
  already safe to interleave; only the ref phase is serialized.
* A failed or interrupted transfer leaves at most a set of unreferenced
  objects, which are inert and reclaimable with `vara gc` (§12).

# 11. Security Considerations

* The local transport reads and writes another repository's `.vara` directory
  directly and therefore runs with the invoking user's filesystem permissions.
  It performs no privilege escalation.
* Every received object's identity is verified by recomputing its SHA-256; a
  malicious or corrupt peer cannot inject an object under a hash it does not
  actually hash to.
* The stream trailer bounds tampering to detectable truncation/corruption.
* `url` values are treated as opaque locations; the transport MUST reject a
  `url` that does not resolve to a directory containing a valid `.vara`
  repository rather than creating one implicitly.

# 12. Garbage Collection

`vara gc` reclaims objects no longer reachable from any root. The reachable set
is the object closure (§7) of every root, where roots are: all references, the
resolved `HEAD`, and every commit recorded in `HEAD`'s reflog. Including the
reflog guarantees gc never deletes an object that `vara undo` could restore. Any
loose object outside that closure is provably unreferenced and removed;
`--dry-run` reports without deleting. gc makes interrupted transfers (§10) fully
recoverable — their inert leftover objects are swept on the next run.

# 13. Performance & Scaling

Measured on AMD Ryzen 9 8945HS, Windows 11, NTFS. Each commit changes one file
(3 objects/commit), a near-worst case for object count.

| Commits | Full clone | Verify | Incremental fetch (+100) | Live heap |
|---------|-----------|--------|--------------------------|-----------|
| 1 000   | 40.1 s    | 17.3 s | 3.7 s                    | ~52 MiB   |
| 2 000   | 61.8 s    | 48.3 s | 4.0 s                    | ~52 MiB   |

Findings:

* **Clone is linear**, not quadratic (~22 ms/commit + ~18 s fixed overhead).
  The dominant cost is NTFS per-object file operations (~5–7 ms each) — the same
  filesystem limit already documented for `add`/`status`, not an algorithmic
  flaw. A destination inherently must write every object it lacks.
* **Memory is bounded.** Live heap is flat (~52 MiB) regardless of history size:
  objects compress small, so `ReadPack`'s whole-stream read is not a bottleneck
  at these sizes. Streaming ingestion is nonetheless listed below for very large
  transfers.
* **Incremental fetch is ~constant** in repository size — its cost tracks the
  number of *new* commits plus a cheap boundary walk, which is the common case.
* A removable constant factor remains: enumeration reads each commit roughly
  twice and `WritePack` re-reads all objects. Caching tree hashes during the
  commit walk would cut server-side reads ~2×. Deferred to RFC-0015.

# 14. Future Work

* **RFC-0015 Pack Optimization** — delta-compressed packs, boundary-tree
  subtraction, thin packs, streaming ingestion (verify+write records as they
  arrive instead of buffering the whole stream), and caching tree hashes during
  the commit walk to halve server-side reads.
* **RFC-0016 Network Transport** — a `vara serve` daemon and a `vara://`
  (and/or HTTPS) transport implementing the §6 interface over sockets, with
  authentication and capability negotiation.
* **Multi-branch push**, tag propagation, and prune-on-fetch
  (`--prune` deletes remote-tracking refs whose remote branch disappeared).
