VARA RFC: 0006
Title: Locking and Transactions
Status: Accepted
Version: 1.1.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0003, RFC-0004, RFC-0005
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the atomic locking mechanisms and the overarching Repository Transaction Model that protects a VARA repository from data corruption. It establishes VARA as a transactional, content-addressed storage engine.

# 2. Hierarchical Locks
To prevent serialization bottlenecks (e.g., `vara status` waiting for `vara fetch`), VARA uses a hierarchical locking model rather than a single global lock.

## 2.1. Lock Acquisition
* **Mechanism:** OS-level exclusive file locking (`flock` on Unix, `LockFile` on Windows) on the target lock file.
* **Payload:** The file contains the Process ID (PID) of the lock owner in plain text.
* **Timeout Policy:** 
  * Default timeout is 30 seconds.
  * Processes use exponential backoff for retries while waiting.
  * Timeout can be overridden via `VARA_LOCK_TIMEOUT` env var or configuration.
  * If the timeout expires, the command aborts with `ErrRepositoryLocked`.

Locks are stored in `.vara/locks/`:
* `repository.lock`: Global lock (used only for operations that rewrite the whole repository, like `upgrade`).
* `refs.lock`: Locks reference updates (branch movement, tag creation).
* `index.lock`: Locks modifications to the staging area.
* `objects.lock`: Locks garbage collection or reachability analysis (writing loose objects does not require a lock).

**Lock Hierarchy Rule:**
Locks MUST be acquired in this exact order: `Repository -> Refs -> Index -> Objects`.
A command can skip locks it doesn't need, but it can never acquire a lock if it already holds a lower-priority one. This strictly prevents deadlocks.

# 3. The Write-Ahead Journal
Relying solely on OS-level file locks is insufficient for recovering from hard power failures.
VARA implements a Write-Ahead Journal in `.vara/journal/`.

Each transaction creates a file (e.g., `txn-00123.json`) containing its intent:
```json
{
  "transaction": "00123",
  "state": "EXECUTE",
  "command": "commit",
  "objects": ["a1b2c3...", "d4e5f6..."],
  "refs": {"HEAD": "a1b2c3..."}
}
```
During a crash, `vara recover` inspects unfinished transactions in the journal to definitively clean up or rollback.

# 4. Repository Transactions
VARA treats complex commands as atomic database transactions.
**Fundamental Guarantee:** A partially completed transaction must never produce a repository state that appears committed.

## 4.1. Transaction Lifecycle
1. **BEGIN:** Acquire necessary hierarchical locks. Create the journal entry.
2. **VALIDATE:** Check preconditions. Ensure refs exist, HEAD is valid, repository is healthy.
3. **EXECUTE:** Write loose objects to the store. Write temporary reference `.lock` files. Write the new index to `.vara/index.lock`. Update journal state.
4. **VERIFY:** Assert post-conditions before committing. Check that object hashes match, index checksum is valid, and target refs haven't been changed by another process.
5. **COMMIT:** Atomically rename all `.lock` files to their final destinations. Update the reflog.
6. **CLEANUP:** Delete temporary files, mark the journal entry as `COMPLETED`, and release locks.

# 5. Appendix: VARA Repository Guarantees
These guarantees form the immutable contract with the user:
* **Objects:** Immutable (✓)
* **Commits:** Immutable (✓)
* **Refs:** Atomic (✓)
* **Index:** Recoverable (✓)
* **Transactions:** Crash Safe (✓)
* **History:** Never Rewritten silently (✓)
