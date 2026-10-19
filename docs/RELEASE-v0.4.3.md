# VARA v0.4.3 Release Notes

**Released**: 2026-10-20
**Tag**: `v0.4.3`

---

## What v0.4.3 is

v0.4.2 made `vara undo --force` save your uncommitted work in a snapshot. This
release adds the commands to get it back, and fixes a bug that could lose an
older snapshot.

## New

- **`vara snapshot list`** shows the working-tree snapshots VARA saves before
  `switch`, `merge` and `undo`, newest first, with when and before which command.
- **`vara snapshot restore <name>`** writes one back. The start of the name is
  enough when only one snapshot matches. Restore changes files only: HEAD,
  branches and the index are untouched, so `vara status` shows what came back.
  Like undo, it refuses while tracked files have uncommitted changes; `--force`
  goes ahead after saving the current working tree in a new snapshot.
- Restoring refuses an archive entry that would land outside the repository or
  is not a plain file, before anything is written.

## Fixes

- **Snapshots taken in the same second no longer overwrite each other.** Two
  `switch` commands within one second used to produce one snapshot file, and the
  first copy was lost. The second now gets a `.2` suffix. This is a change in
  `pkg/snapshot`, made under the v1 freeze rule for proven correctness bugs.
