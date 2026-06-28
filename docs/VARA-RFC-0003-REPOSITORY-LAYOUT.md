VARA RFC: 0003
Title: Repository Layout
Status: Accepted
Version: 1.2.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0001, RFC-0002
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the physical directory structure, repository state machine, and file layout of a VARA repository. It specifies where authoritative data is stored, which files govern repository state, who owns them, and what happens when they are lost.

**Strict Rule:** Commands must never modify files outside their documented ownership without an explicit RFC defining that behavior.

# 2. The `.vara` Directory

```text
.vara/
├── VERSION            
├── HEAD               
├── ORIG_HEAD          
├── MERGE_HEAD         
├── CHERRY_PICK_HEAD   
├── REBASE_HEAD        
├── config             
├── index              
├── locks/             # Hierarchical lock files
├── journal/           # Transaction Write-Ahead Log
├── objects/           
├── refs/              
├── logs/              
├── snapshots/         
└── hooks/             
```

# 3. Repository State Machine
A VARA repository is a state machine governed by the presence of specific files.

**States:**
* **Uninitialized:** No `.vara/` directory.
* **Initialized:** Base `.vara/` structure exists. Required: `VERSION`, `HEAD`.
* **Clean:** `index` matches `HEAD` and working directory. Required: `index`.
* **Dirty:** Working directory or `index` differs from `HEAD`.
* **Merge In Progress:** A merge is active. Required: `HEAD`, `MERGE_HEAD`, `index`, `logs/`.
* **Rebase In Progress:** A rebase is active. Required: `REBASE_HEAD`.
* **Detached HEAD:** `HEAD` points to a raw commit hash instead of a symbolic ref.
* **Recovery Mode:** `vara undo` is active. Required: `logs/`, `snapshots/`.

# 4. Core Files and Ownership Matrix

| File | Created By | Modified By | Read By |
| --- | --- | --- | --- |
| `VERSION` | `init` | `upgrade` (future) | Almost every command |
| `HEAD` | `init` | `switch`, `commit`, `merge`, `undo` | Almost every command |
| `config` | `init`, `config` | `config` | Almost every command |
| `index` | `add`, `switch` | `add`, `commit`, `switch`, `merge`, `undo` | `status`, `commit`, `switch`, `merge` |
| `locks/*` | Modifying cmds | Modifying cmds | Modifying cmds |
| `MERGE_HEAD` | `merge` | `merge` | `commit`, `status`, `undo` |
| `ORIG_HEAD` | `merge`, `switch` | `merge`, `switch` | `undo` |

# 5. Recovery Rules
VARA must react predictably if core files are lost or corrupted.

### Missing `index`
* **Action:** Recreate empty index. Log a warning. Continue. All files appear untracked.

### Missing `HEAD`
* **Action:** Fatal error. Repository corrupted. Suggest running `vara recover` (future command).

### Missing `VERSION`
* **Action:** Fatal error. Cannot determine repository format safely.

### Missing `logs/`
* **Action:** Recreate directory structure. Warn user. Recovery history (`vara undo`) is unavailable for past actions.

### Stale Locks
* **Action:** If older than 5 minutes and PID is dead, delete forcefully. Log a warning.

# 6. Subdirectories

## 6.1. `objects/`
The immutable database. Directories (`00/` to `ff/`) are lazily created. Orphaned loose objects are permitted.

## 6.2. `refs/`
Stores human-readable names mapped to commit hashes.
* **Name Rules:** UTF-8, case-sensitive, max 255 bytes. Cannot contain spaces or `~^:?*[]`.

## 6.3. `logs/` (The Reflog)
Records every reference update. Append-only. If truncated mid-line, partial lines are ignored.

## 6.4. `snapshots/`
Stores full workspace backups (`snap-YYYYMMDD-HHMMSS-HEAD.tar.zst`). Retains last 10 snapshots or 7 days, auto-deleting older ones during `vara switch` or `commit`.

## 6.5. `hooks/`
Executable scripts (`pre-commit`, `post-merge`, etc.). Can be disabled via config.
