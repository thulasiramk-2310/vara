VARA RFC: 0012
Title: Command Specification
Status: Draft
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the CLI surface area for VARA. It maps user intentions to the underlying transaction and storage protocol.

# 2. Core Commands
* `vara init`: Initializes `.vara/` directory and `VERSION`.
* `vara status`: Reads the Index and Working Directory to determine states.
* `vara add <path>`: Hashes file to Object Store and updates Index.
* `vara commit`: Wraps Index into a Tree, creates a Commit object, updates `HEAD`.
* `vara switch <branch>`: Updates Working Directory and `HEAD` to match target branch.
* `vara branch`: Manages references in `refs/heads/`.
* `vara merge <branch>`: Executes the Three-Way Merge algorithm.
* `vara undo`: Invokes the Layered Recovery Protocol.

# 3. AI Commands
* `vara explain`: Summarizes recent commits or complex diffs.
* `vara auto-commit`: Uses AI to generate a semantic commit message based on the Index.
* `vara review`: Scans the Index for common bugs or anti-patterns before commit.

# 4. Plumbing vs Porcelain
Like Git, VARA distinguishes between high-level user commands (Porcelain) and low-level protocol commands (Plumbing).
* Plumbing commands (e.g., `vara hash-object`, `vara update-ref`) support a `--json` flag for deterministic script consumption.
* Porcelain commands focus on rich, colorized UX.
