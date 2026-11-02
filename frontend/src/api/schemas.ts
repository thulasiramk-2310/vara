import { z } from 'zod';

// Runtime checks for every response the Hub API returns. TypeScript types are
// erased at build time, so without these a server that sends an unexpected shape
// would crash a page with "cannot read properties of undefined". Each schema
// below does two jobs:
//
//   1. rejects a response that is missing something the page needs, so the user
//      sees "the server sent something this page does not understand" and a
//      retry button instead of a blank screen;
//   2. repairs the one harmless difference the Go server produces: an empty list
//      is written as `null`, which becomes `[]` here.
//
// Unknown extra fields are ignored, so a newer server does not break an older UI.

// list turns a missing or null array into an empty one.
const list = <T extends z.ZodTypeAny>(item: T) =>
  z
    .array(item)
    .nullish()
    .transform((v) => v ?? []);

const str = z.string();
const int = z.number();

export const whoami = z.object({ id: str, anonymous: z.boolean().nullish().transform((v) => v ?? undefined) });

export const loginResponse = z.object({ expires_at: str.nullish().transform((v) => v ?? undefined) });

export const commitRef = z.object({
  id: str,
  message: str.nullish().transform((v) => v ?? ''),
  author: str.nullish().transform((v) => v ?? ''),
  timestamp: str.nullish().transform((v) => v ?? ''),
});

export const repoListItem = z.object({
  id: str,
  name: str,
  owner: str.nullish().transform((v) => v ?? ''),
  visibility: str.nullish().transform((v) => v ?? ''),
  state: str.nullish().transform((v) => v ?? ''),
});

export const repoList = z.object({ repositories: list(repoListItem) });

export const repoSummary = z.object({
  id: str,
  default_branch: str.nullish().transform((v) => v ?? ''),
  head: str.nullish().transform((v) => v ?? ''),
  commit_count: int,
  branch_count: int,
  last_commit: commitRef.nullish(),
});

export const treeEntry = z.object({ name: str, type: str, mode: str.nullish().transform((v) => v ?? '') });
export const treeResponse = z.object({ entries: list(treeEntry) });

export const blobResponse = z.object({
  content: str.nullish().transform((v) => v ?? undefined),
  size: int,
  binary: z.boolean().nullish().transform((v) => v ?? undefined),
  truncated: z.boolean().nullish().transform((v) => v ?? undefined),
});

export const commitsResponse = z.object({
  commits: list(commitRef),
  next: str.nullish().transform((v) => v ?? undefined),
});

export const branch = z.object({
  name: str,
  target: str,
  is_head: z.boolean().nullish().transform((v) => v ?? undefined),
});
export const branchesResponse = z.object({ branches: list(branch) });

export const diffFileInfo = z.object({
  path: str,
  old_path: str.nullish().transform((v) => v ?? undefined),
  status: str,
});
export const commitDiffSummary = z.object({
  base_commit: str.nullish().transform((v) => v ?? ''),
  files: list(diffFileInfo),
});

export const diffLine = z.object({
  type: str.nullish().transform((v) => v ?? undefined),
  content: str.nullish().transform((v) => v ?? ''),
});
export const diffHunk = z.object({
  header: str.nullish().transform((v) => v ?? ''),
  old_start: int,
  new_start: int,
  lines: list(diffLine),
});
export const fileDiffResponse = z.object({
  binary: z.boolean().nullish().transform((v) => v ?? undefined),
  truncated: z.boolean().nullish().transform((v) => v ?? undefined),
  hunks: list(diffHunk),
});

const flag = z.boolean().nullish().transform((v) => v ?? undefined);

export const contentSearchResponse = z.object({
  matches: list(
    z.object({
      path: str,
      lines: list(z.object({ line: int, content: str.nullish().transform((v) => v ?? '') })),
    }),
  ),
  truncated: flag,
});

export const pathSearchResponse = z.object({
  matches: list(z.object({ path: str, is_dir: flag, mode: str.nullish().transform((v) => v ?? '') })),
  truncated: flag,
});

export const commitSearchResponse = z.object({
  matches: list(commitRef),
  truncated: flag,
  next: str.nullish().transform((v) => v ?? undefined),
});
