# VARA v0.4.2 Release Notes

**Released**: 2026-10-20
**Tag**: `v0.4.2`

---

## What v0.4.2 is

A small release with one Hub fix and one safer way to create the first account.

## Fixes

- **The Hub opens on its sign-in page.** A visitor who was not signed in saw
  "You don't have permission to view this (UNAUTHORIZED)" with no way to sign
  in, because the server answers 403 to anonymous visitors and the web app only
  redirected on 401. It now sends signed-out visitors to the sign-in page.

## New

- **`vara account create --password-stdin`** reads the first account's password
  from standard input, so it never appears in the process list:

  ```sh
  printf '%s\n' "$PASSWORD" | vara account create --accounts ./accounts \
      --policy ./policy --username admin --password-stdin
  ```

  `--password` still works. Giving both is an error. Other commands that take
  `--password` (such as `vara login`) are unchanged.
