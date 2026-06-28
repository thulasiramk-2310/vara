VARA RFC: 0004
Title: References and Resolution
Status: Accepted
Version: 1.1.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0002, RFC-0003
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines how VARA names, stores, validates, and updates references (refs). Refs map human-readable names (branches, tags) or states (`HEAD`) to immutable commit hashes. It also establishes the precise algorithms for reference resolution and atomic updates.

# 2. Reference Type Taxonomy
VARA handles several distinct classes of references, each with specific behaviors.

1. **Symbolic Reference**: A pointer to another reference (e.g., `HEAD -> refs/heads/main`).
2. **Direct Reference**: A pointer to a commit hash (e.g., `refs/heads/main -> a1b2c3...`).
3. **Detached HEAD**: `HEAD` pointing directly to a commit hash instead of a branch.
4. **Tag Reference**: An immutable pointer to a specific commit (`refs/tags/v1.0.0`).
5. **Remote Tracking Reference**: A cached state of a remote branch (`refs/remotes/origin/main`).
6. **Pseudo References**: Ephemeral runtime state pointers (e.g., `ORIG_HEAD`, `MERGE_HEAD`, `REBASE_HEAD`, `CHERRY_PICK_HEAD`). These never participate in push/pull operations and live directly in `.vara/`.

# 3. Reference Validation Rules
To ensure cross-platform compatibility and prevent filesystem exploits, branch and tag names must pass strict validation before creation.

* **Encoding:** Must be valid UTF-8, normalized (NFC).
* **Length:** Maximum length of 255 bytes.
* **Case Sensitivity:** Preserved and case-sensitive (implementations must account for case-insensitive filesystems during collision checks).
* **Restricted Characters:** Cannot contain spaces, ASCII control characters, or any of the following: `~`, `^`, `:`, `?`, `*`, `[`, `\`.
* **Path Restrictions:** Cannot contain `..` or begin/end with `/` or `.`. (e.g., `refs/heads/.hidden` is rejected).

# 4. Atomic Reference Updates
A reference update must **never** be performed via a simple open-overwrite-close operation, as this risks corruption during a crash.

Updates MUST follow this atomic protocol:
1. Acquire global `VARA_LOCK`.
2. Write the new commit hash to a temporary file: `.vara/refs/heads/<branch>.lock`.
3. Flush to disk (`fsync()`).
4. Perform an atomic rename (`rename()`) of the `.lock` file over the target reference file.
5. Append an entry to the specific reflog (`.vara/logs/refs/heads/<branch>`).
6. Append an entry to the global reflog (`.vara/logs/HEAD`).
7. Release `VARA_LOCK`.

# 5. Reference Transactions
Internally, VARA defines a Transaction Model for updating multiple references simultaneously (e.g., updating `HEAD`, a branch, and `ORIG_HEAD` during a merge).
Even if not exposed to the CLI immediately, the implementation must structure reference updates as transactions to guarantee that either all refs update successfully or none do.

# 6. Reference Resolution Algorithm
When a command receives a ref name (e.g., `main`, `v1.0`, or `HEAD`), it deterministically resolves to a commit hash using this sequence:

1. **Hex Prefix:** If the input is a unique hexadecimal prefix (≥ 7 chars), resolve directly via the object store.
2. **Pseudo Refs:** Check for runtime state files (`.vara/HEAD`, `.vara/MERGE_HEAD`, etc.). If `HEAD` is symbolic (`ref: <path>`), recursively resolve `<path>`.
3. **Local Branches:** Check `.vara/refs/heads/<input>`.
4. **Tags:** Check `.vara/refs/tags/<input>`.
5. **Remotes:** Check `.vara/refs/remotes/<input>`.
6. **Packed Refs:** If none exist in the loose `refs/` directory, perform the same search order against `.vara/packed-refs`.
7. If still not found, return `ErrRefNotFound`.

# 7. Packed References
**Status: Concept defined, implementation deferred.**
To avoid inode exhaustion when a repository has thousands of branches, references can be packed into a single `.vara/packed-refs` file formatted as `<64-char-hex> <ref-path>\n`. If a loose ref exists alongside a packed ref for the same path, the loose ref takes precedence.
