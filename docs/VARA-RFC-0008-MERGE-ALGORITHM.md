VARA RFC: 0008
Title: Merge Algorithm
Status: Accepted
Version: 1.1.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0005, RFC-0006, RFC-0007
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines how VARA integrates divergent lines of history. By leveraging the Commit Graph (RFC-0007) and the Index (RFC-0005), the merge process is reduced to a deterministic mathematical algorithm, protected by transactions (RFC-0006).

# 2. Merge Strategy Interface
To ensure future extensibility without breaking the protocol, VARA defines an internal abstraction for merges:
```go
type MergeStrategy interface {
    Merge(base, ours, theirs *Tree) (*Tree, error)
}
```
In Phase 1, only the `RecursiveMerge` strategy is implemented. Future extensions may introduce `Ours`, `Theirs`, `Octopus`, or `AI-Assisted` strategies.

# 3. Merge Result States
A merge operation acts as a finite-state machine, producing one of the following terminal or suspended states:
1. **SUCCESS**: The merge completed automatically (Fast-Forward or clean Three-Way Merge).
2. **CONFLICT**: The merge is suspended pending user resolution.
3. **ABORTED**: The user actively cancelled an in-progress merge.
4. **RECOVERED**: A failed merge was rolled back via the Transaction Journal.

# 4. Merge Algorithms

## 4.1. Fast-Forward Merge
When merging Branch B into Branch A, if `MergeBase(A, B) == A`:
1. **BEGIN**: Transaction.
2. Update Index to match Branch B's tree.
3. Update Working Directory to match Branch B.
4. Point Branch A's ref to Branch B's commit hash.
5. **COMMIT**: Transaction. (Result State: SUCCESS)

## 4.2. Three-Way Merge
If branches have diverged, find `O = MergeBase(A, B)`.
1. **BEGIN**: Transaction.
2. Compare Trees: `Diff(O, A)` and `Diff(O, B)`.
3. Resolve per file using the `MergeStrategy`.
4. If conflicts exist: Write markers, write `.vara/MERGE_HEAD`, and **COMMIT** transaction (suspending state into CONFLICT).
5. If no conflicts: Generate new commit object, update refs, **COMMIT** transaction (Result State: SUCCESS).

# 5. Conflict Classification and Markers
VARA explicitly classifies conflicts for superior UX and AI diagnostics:
* `Content Conflict`: Divergent edits to the same lines.
* `Rename Conflict`: Both sides renamed the file differently.
* `Delete vs Modify`: One side modified, the other deleted.
* `Binary Conflict`: Both sides modified an unmergeable binary file.
* `Directory/File Conflict`: A path is a file on one branch and a directory on the other.
* `Permission Conflict`: Mode changes (e.g., `+x`) conflict.

**Markers:** (Used for Content Conflicts)
```text
<<<<<<< HEAD (Branch A)
Line from current branch.
=======
Line from incoming branch.
>>>>>>> feature-branch (Branch B)
```

# 6. Aborting a Merge
If a user runs `vara undo merge` during a `CONFLICT` state:
1. Deletes `.vara/MERGE_HEAD`.
2. Restores the Index and Working Directory via Workspace Snapshots (RFC-0009).
3. Enters `ABORTED` state.
