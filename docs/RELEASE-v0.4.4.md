# VARA v0.4.4 Release Notes

**Released**: 2026-10-20
**Tag**: `v0.4.4`

---

## What v0.4.4 is

A fix release for `vara undo`. Before it, undo did not reverse a commit, and
after a branch switch it could point the wrong branch at another commit.

## Fixes

- **Undo reverses commits.** Commits were not written to the reflog, so undo
  found nothing to reverse, fell back to the snapshot it had just taken and
  changed nothing while reporting a restore. Commits and merges are now
  recorded in the reflog of HEAD and of the current branch.
- **Each undo steps further back.** Undo records itself in the reflog, so a
  second undo reverses the change before the first, back to where the branch
  began. Then it says there is nothing earlier to undo.
- **Undo works on the branch you are on.** It reads that branch's own reflog,
  so switches never get in the way, and it never moves another branch. Before,
  undo after `vara switch` pointed the current branch at the commit of the
  branch you came from.
- **Files the undone commit added are removed.** The snapshot taken before
  every undo still holds them.
- **Undo stops instead of guessing.** If the branch moved without a record, it
  changes nothing and says why. It no longer restores an unrelated snapshot in
  a repository that has commits, and it never restores a snapshot that undo
  itself took; `vara snapshot restore` still does.

The reflog walk is a change in `pkg/recovery`, made under the v1 freeze rule
for proven correctness bugs.

## Upgrading

Commits made with v0.4.3 or earlier are not in the reflog. In an existing
repository, undo reaches back only to the first commit made with v0.4.4.
