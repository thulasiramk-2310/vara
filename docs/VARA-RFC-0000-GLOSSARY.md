VARA RFC: 0000
Title: Glossary
Status: Draft
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: None
Supersedes: None
Superseded By: None

# 1. Introduction
This document defines the core terminology used across all VARA RFCs. 

# 2. Definitions

**Blob**
An object storing raw file content. It contains no metadata about the file (like name or permissions).

**Tree**
An object representing a directory. It contains a list of entries, mapping filenames to their corresponding object hashes and permissions.

**Commit**
An object representing a snapshot of the repository at a fixed point in time. It references a single tree and zero or more parent commits.

**Object**
A generic term for any data structure (Blob, Tree, Commit) stored in the `.vara/objects/` database. All objects are immutable and content-addressed.

**Ref (Reference)**
A named pointer to a commit hash (e.g., a branch or a tag). Stored in `.vara/refs/`.

**HEAD**
A special reference pointing to the currently active branch or commit in the working directory.

**Index**
The binary staging area (cache) that bridges the working tree and the object store.

**Snapshot**
A full backup of the workspace state (tracked, untracked, ignored files) taken automatically before destructive operations. Powered by the reflog, it enables `vara undo`.

**Reflog**
An append-only log of every update made to a reference (including HEAD). Essential for undo and recovery.

**Pack**
An archive containing multiple objects, potentially delta-compressed to save space (future optimization).

**Loose Object**
A single object stored in its own file within `.vara/objects/`, compressed using zstd.

**Merge Base**
The best common ancestor commit between two branches, used as the starting point for a 3-way merge.

**Fast Forward**
A merge operation where the target branch is a direct descendant of the current branch, requiring no new merge commit.

**Detached HEAD**
A state where HEAD points directly to a commit hash rather than a symbolic branch reference.
