# VARA v0.4.1 Release Notes

**Released**: 2026-10-20
**Tag**: `v0.4.1`

---

## What v0.4.1 is

v0.4.1 collects the command-line work done since v0.4.0. It gives commits a
real author, makes merge conflicts something you can work through step by
step, adds an opt-in structural merge for config files, and fixes two bugs,
one of which could lose uncommitted work.

## Highlights

| Area | What shipped |
|------|--------------|
| **Identity** | `vara identity set --default --name ... --email ...` records who you are. Commits carry that author, and a commit with no identity configured is refused instead of being signed with a placeholder (RFC-0017). Identity is local configuration and never contacts a server. |
| **Remotes** | A local credential store for remote sessions, used automatically by `clone`, `fetch` and `push` (RFC-0020). |
| **Conflicts** | `vara resolve` (ours, theirs, union, auto), `vara rm`, `vara merge --abort` and `vara merge --continue`. `status` and `diff` show conflicts, and a commit is held back until every conflict is resolved. |
| **Structural merge** | Opt-in per repository for JSON, YAML and TOML: edits to different keys combine instead of conflicting (RFC-0025). YAML comments are not kept by a structural merge. |
| **Hub UI** | A React read-only web interface for the Hub. |

## Fixes

- **`vara undo` no longer destroys unsaved edits.** It used to overwrite tracked
  files that had uncommitted changes, with no copy kept. It now refuses and
  names the files. `vara undo --force` goes ahead, and the working tree is saved
  in `.vara/snapshots` first.
- **`vara status` after a merge or an undo** listed unchanged files as modified,
  because it trusted file timestamps. It now compares content. This is a change
  in `pkg/scanner`, made under the v1 freeze rule for proven correctness bugs.
- **`ThreeWayMerge` with an empty base** now terminates.
- **Undo error messages** no longer repeat "undo:".

## Known limitations

See `docs/IMPLEMENTATION-STATUS.md`. A second `vara undo` does not step further
back, and snapshots have no restore command yet.
