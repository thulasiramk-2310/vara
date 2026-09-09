// A thin, typed client over the VARA Hub read API (RFC-0021/0022/0023/0024).
// Every call is same-origin (the httpOnly session cookie rides automatically),
// returns parsed JSON, and throws an ApiError on a non-2xx response. It talks
// ONLY to endpoints the v0.4 backend actually serves; unsupported features
// (pull requests, issues, orgs) have no client here and are UI stubs.

export interface ApiError extends Error {
  status: number;
  code?: string;
}

function apiError(message: string, status: number, code?: string): ApiError {
  const e = new Error(message) as ApiError;
  e.status = status;
  e.code = code;
  return e;
}

// encPath percent-encodes each segment of a browse path while preserving "/" as
// the separator. "" → "" (the repository root).
export const encPath = (p: string): string =>
  String(p || '')
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');

const enc = encodeURIComponent;

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = { method, headers: {} };
  if (body !== undefined) {
    (opts.headers as Record<string, string>)['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(path, opts);
  if (resp.status === 204) return null as unknown as T;

  const text = await resp.text();
  const ct = resp.headers.get('content-type') || '';
  // Parse JSON only when the body really is JSON. A route that isn't enabled on
  // this server falls through to the static handler, which returns index.html —
  // parsing that as JSON would throw a cryptic "Unexpected token '<'". Treat a
  // non-JSON 200 as "endpoint unavailable" instead.
  let data: unknown = null;
  if (text && (ct.includes('json') || text.trimStart()[0] === '{')) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!resp.ok) {
    const d = data as { message?: string; code?: string } | null;
    throw apiError((d && d.message) || resp.statusText || `HTTP ${resp.status}`, resp.status, d?.code);
  }
  if (data === null && text && !ct.includes('json')) {
    throw apiError("This feature isn't enabled on this server.", 0, 'UNAVAILABLE');
  }
  return data as T;
}

// ---- response shapes (only what the read API returns) -----------------------

export interface Whoami {
  id: string;
  anonymous?: boolean;
}

export interface LoginResponse {
  expires_at?: string;
}

export interface RepoListItem {
  name: string;
  owner: string;
  visibility: string;
  state: string;
}

export interface CommitRef {
  id: string;
  message: string;
  author: string;
  timestamp: string;
}

export interface RepoSummary {
  id: string;
  default_branch: string;
  head: string;
  commit_count: number;
  branch_count: number;
  last_commit?: CommitRef | null;
}

export interface TreeEntry {
  name: string;
  type: string; // "dir" for directories, otherwise a file
  mode: string;
}

export interface TreeResponse {
  entries: TreeEntry[];
}

export interface BlobResponse {
  content?: string;
  size: number;
  binary?: boolean;
  truncated?: boolean;
}

export interface CommitsResponse {
  commits: CommitRef[];
  next?: string;
}

export interface Branch {
  name: string;
  target: string;
  is_head?: boolean;
}

export interface BranchesResponse {
  branches: Branch[];
}

export interface DiffFileInfo {
  path: string;
  old_path?: string;
  status: string;
}

export interface CommitDiffSummary {
  base_commit: string;
  files: DiffFileInfo[];
}

export interface DiffLine {
  type?: string; // "add" | "del" | undefined (context)
  content: string;
}

export interface DiffHunk {
  header: string;
  old_start: number;
  new_start: number;
  lines: DiffLine[];
}

export interface FileDiffResponse {
  binary?: boolean;
  truncated?: boolean;
  hunks?: DiffHunk[];
}

export interface ContentMatchLine {
  line: number;
  content: string;
}

export interface ContentMatch {
  path: string;
  lines: ContentMatchLine[];
}

export interface ContentSearchResponse {
  matches: ContentMatch[];
  truncated?: boolean;
}

export interface PathMatch {
  path: string;
  is_dir?: boolean;
  mode: string;
}

export interface PathSearchResponse {
  matches: PathMatch[];
  truncated?: boolean;
}

export interface CommitSearchResponse {
  matches: CommitRef[];
  truncated?: boolean;
  next?: string;
}

// ---- endpoints --------------------------------------------------------------

const base = (repo: string) => `/_vara/repositories/${enc(repo)}`;

export const api = {
  whoami: () => request<Whoami>('GET', '/_vara/whoami'),

  // Cookie mode (RFC-0021 §7): the server sets an httpOnly session cookie and
  // withholds the secret from the body, so page JS never touches it. The body is
  // just { expires_at }; call whoami() afterward to learn the resolved identity.
  login: (username: string, password: string) =>
    request<LoginResponse>('POST', '/_vara/sessions?cookie=1', { username, password }),

  logout: () => request<null>('DELETE', '/_vara/sessions/current'),

  listRepos: () => request<{ repositories: RepoListItem[] }>('GET', '/_vara/repositories'),

  createRepo: (name: string) => request<unknown>('POST', '/_vara/repositories', { name }),

  summary: (repo: string) => request<RepoSummary>('GET', `${base(repo)}/summary`),

  branches: (repo: string) => request<BranchesResponse>('GET', `${base(repo)}/branches`),

  tree: (repo: string, path: string) =>
    request<TreeResponse>('GET', `${base(repo)}/tree/${encPath(path)}`),

  blob: (repo: string, path: string) =>
    request<BlobResponse>('GET', `${base(repo)}/blob/${encPath(path)}`),

  rawUrl: (repo: string, path: string) => `${base(repo)}/raw/${encPath(path)}`,

  commits: (repo: string, limit = 30, before = '') =>
    request<CommitsResponse>(
      'GET',
      `${base(repo)}/commits?limit=${limit}${before ? `&before=${enc(before)}` : ''}`,
    ),

  commit: (repo: string, id: string) =>
    request<CommitRef>('GET', `${base(repo)}/commits/${enc(id)}`),

  commitDiff: (repo: string, id: string) =>
    request<CommitDiffSummary>('GET', `${base(repo)}/commits/${enc(id)}/diff`),

  fileDiff: (repo: string, path: string, head: string, base_: string) =>
    request<FileDiffResponse>(
      'GET',
      `${base(repo)}/diff/${encPath(path)}?head=${enc(head)}&base=${enc(base_)}`,
    ),

  searchContent: (repo: string, q: string) =>
    request<ContentSearchResponse>('GET', `${base(repo)}/search/content?q=${enc(q)}`),

  searchPaths: (repo: string, q: string) =>
    request<PathSearchResponse>('GET', `${base(repo)}/search/paths?q=${enc(q)}`),

  searchCommits: (repo: string, q: string) =>
    request<CommitSearchResponse>('GET', `${base(repo)}/search/commits?q=${enc(q)}`),
};

// short truncates an object id for display; the full id stays available to copy.
export const short = (id?: string): string => (id ? id.slice(0, 10) : '');

// when formats an ISO/RFC timestamp for display, tolerating an empty value.
export const when = (ts?: string): string => {
  if (!ts) return '';
  const d = new Date(ts);
  return isNaN(d.getTime()) ? ts : d.toLocaleString();
};
