VARA RFC: 0009
Title: Undo and Recovery
Status: Accepted
Version: 1.2.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-10-20
Depends On: RFC-0000, RFC-0003, RFC-0006, RFC-0007
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
VARA replaces the dangerous `git reset` / `git reflog` paradigm with a single coherent recovery model: `vara undo`.
This RFC defines the Recovery Protocol, ensuring that any destructive repository action can be safely reversed.

# 2. The Three-Layer Recovery Model
VARA achieves safe undo through a strict hierarchy of defense:

1. **Layer 1: Transaction Journal** (Crash Recovery)
2. **Layer 2: Reflog** (Repository History Recovery)
3. **Layer 3: Workspace Snapshot** (Uncommitted Work Recovery)

# 3. Recovery Decision Tree
Implementations MUST follow this deterministic decision logic when `vara recover` or `vara undo` is invoked:
```text
Did a crash occur?
 ├── YES -> Does a pending Journal entry exist?
 │           ├── YES -> Replay/Rollback transaction. (End)
 │           └── NO -> Check filesystem integrity. (End)
 └── NO -> Is the user requesting to undo a command?
             ├── YES -> Execute Reflog rollback. (End)
             └── NO -> Is the user missing uncommitted workspace files?
                         ├── YES -> Restore from Snapshot. (End)
                         └── NO -> No action needed.
```

# 4. Recovery States
The repository transitions through these explicit states during a recovery action:
* **Healthy:** Standard operations permitted.
* **Recovery Pending:** A crash or failed command was detected; mutating operations are locked.
* **Recovery Running:** The rollback logic is executing.
* **Recovered:** Successfully restored to a Healthy state.
* **Failed Recovery:** Fatal corruption detected; manual intervention required.

# 5. Snapshot Policy
Unlike Git, VARA automatically snapshots the working directory before any mutating command (e.g., `switch`, `merge`).

* **Format:** `snap-YYYYMMDD-HHMMSS-<command>-<commit>.tar.zst`
* **Compression:** Zstandard (`zstd`) for fast, lightweight archives.
* **Location:** `.vara/snapshots/`
* **Retention Policy:** The last 10 snapshots OR snapshots newer than 7 days (whichever is greater).
* **Cleanup Algorithm:** LRU pruning executed safely under the `Objects Lock` during background GC.
* **Commands:**
  * `vara snapshot create`: Manually create a backup.
  * `vara snapshot list`: View available recovery points.
  * `vara snapshot restore`: Force restore a specific point in time.

# 6. `vara undo` Algorithm
1. **Transaction BEGIN**.
2. Read the reflog of the current branch (`.vara/logs/refs/heads/<branch>`), or `.vara/logs/HEAD` when HEAD is detached. Commits and merges append to both.
3. **Pick the entry:** walk newest-first. Each entry whose message starts with `undo: ` is skipped together with the entry it reversed, so consecutive undos step further back. Reaching an entry with no previous commit means there is nothing earlier to undo.
4. **Check:** the entry's new commit MUST equal the current HEAD. If it does not, something moved HEAD without a record, and undo stops without changing anything.
5. **Reflog Rollback:** point the branch (or HEAD) at the entry's old commit, check out its tree, and remove committed files it does not contain.
6. **Record:** append `undo: <message of the reversed entry>` to the branch reflog and to HEAD's.
7. **Transaction COMMIT.**

The working tree is snapshotted before step 1 (§5). Layer 3 never restores a snapshot taken by undo itself; `vara snapshot restore` does.
