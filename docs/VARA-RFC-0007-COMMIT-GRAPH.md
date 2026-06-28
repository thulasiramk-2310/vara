VARA RFC: 0007
Title: Commit Graph and Traversal
Status: Accepted
Version: 1.1.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0002, RFC-0004
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the mathematical foundation of VARA's history: the Commit Graph. It formalizes the representation of commits as a Directed Acyclic Graph (DAG), the strict invariants that govern it, and the algorithms used for deterministic traversal and reachability.

# 2. DAG Representation and Invariants
The repository history is a Directed Acyclic Graph (DAG) where vertices are commit objects and edges are parent pointers pointing backward in time.

To guarantee correctness across all commands, the following invariants are ALWAYS true:
* **Invariant 1:** The commit graph is strictly a Directed Acyclic Graph.
* **Invariant 2:** Every commit has a globally unique object hash.
* **Invariant 3:** Parent references must resolve to existing commits in the object store.
* **Invariant 4:** Cycles are cryptographically impossible (a commit cannot contain its own hash).
* **Invariant 5:** The root commit has exactly zero parents.
* **Invariant 6:** Merge commits have exactly two parents in Phase 1 (octopus merges are deferred).

# 3. Commit Ordering and Topology
In VARA, **topology determines history. Timestamps are metadata only.**
Because clock skew happens across distributed systems, timestamps cannot be trusted for logical ordering.

**Topological Ordering** is the authoritative way to sort commits.
* Commits are ordered such that every child appears before its parents.
* If a tie occurs during traversal, topological depth (Generation Numbers) is evaluated before falling back to timestamps or hash lexicography.

# 4. Commit Generation Numbers
To optimize graph traversal, VARA introduces Generation Numbers.
A Generation Number is defined as:
* Root commit: `Generation = 0`
* Child commit: `Generation = max(Parent Generations) + 1`

Generation numbers allow algorithms to short-circuit. For example, if searching for an ancestor and the current commit's generation is lower than the target's, the search can immediately stop.

# 5. Reachability and Pruning
A commit is defined as **Reachable (Live)** if:
1. It is directly referenced by any ref in `.vara/refs/` or `HEAD`.
2. OR, it is an ancestor of a referenced commit.

Any commit that does not meet these criteria is **Unreachable** and becomes a **Garbage Candidate**. Future Garbage Collection (GC) operations will only prune Unreachable objects.

# 6. Traversal Algorithms

## 6.1. Merge Base (Lowest Common Ancestor)
Used by `vara merge` to find the best common ancestor of two branches.
**Algorithm:**
1. Collect all ancestors of Branch A (using BFS).
2. Collect all ancestors of Branch B (using BFS).
3. Intersect the two sets.
4. Choose the nearest common ancestor (the one with the highest Generation Number).
5. Return the merge base. (If multiple exist at the same generation, apply recursive merge base logic).

## 6.2. Breadth-First (BFS) and Depth-First (DFS)
* **BFS:** Used for shortest-path ancestor discovery.
* **DFS:** Used for rendering linear histories (`vara history`).

# 7. Graph Validation
The command `vara verify --graph` asserts the mathematical integrity of the DAG.
It strictly checks for:
* Cryptographic cycles.
* Dangling parent pointers (Missing objects).
* Duplicate hashes.
* Unreachable roots.
* Broken references.

# 8. Algorithmic Complexity Expectations
To ensure VARA scales, implementations should target these expected time complexities (assuming an indexed object store implementation):

| Operation | Expected Complexity | Description |
| --------- | ------------------- | ----------- |
| **Commit Lookup** | `O(1)` | Direct filesystem/index read. |
| **Parent Lookup** | `O(1)` | Parsing generation/parents from the commit object. |
| **Ancestor Search** | `O(N)` | Where `N` is the number of commits traversed (optimized by Generation Numbers). |
| **Merge Base (LCA)**| `O(N)` | Walking backward from both branches. |
| **Topological Sort**| `O(V + E)` | Standard graph sorting complexity. |
