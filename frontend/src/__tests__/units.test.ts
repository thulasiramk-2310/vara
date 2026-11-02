import { describe, it, expect, vi, afterEach } from 'vitest';
import { safeNext } from '../components/common/RequireAuth';
import { filterAndSort, parseSort } from '../utils/repoList';
import { mapLimit } from '../utils/pool';
import { languageFor, flatten, highlightLines, MAX_HIGHLIGHT_CHARS } from '../utils/syntax';
import { shouldRetry } from '../hooks/useAsync';
import { treeResponse, commitsResponse, repoSummary, blobResponse } from '../api/schemas';
import { copyText } from '../utils/clipboard';
import { contribNote } from '../hooks/useContributions';
import { routes } from '../routes';

describe('safeNext (post sign-in redirect)', () => {
  it('keeps in-app paths', () => {
    expect(safeNext('/r/demo/blob/src/main.go?ref=feature')).toBe('/r/demo/blob/src/main.go?ref=feature');
    expect(safeNext('/new')).toBe('/new');
  });
  it.each([null, '', '//evil.example', 'https://evil.example', '/\\evil.example', 'javascript:alert(1)', 'evil.example'])(
    'refuses %j',
    (raw) => expect(safeNext(raw as string | null)).toBe('/dashboard'),
  );
});

describe('repository list filter and sort', () => {
  const repos = ['proj-10', 'proj-2', 'Alpha', 'beta'].map((name) => ({ name, visibility: 'public', state: 'active' }));
  it('sorts names naturally and ignores case', () => {
    expect(filterAndSort(repos, '', 'name-asc').map((r) => r.name)).toEqual(['Alpha', 'beta', 'proj-2', 'proj-10']);
    expect(filterAndSort(repos, '', 'name-desc').map((r) => r.name)).toEqual(['proj-10', 'proj-2', 'beta', 'Alpha']);
  });
  it('filters by substring, trimming spaces, without touching the input', () => {
    const before = repos.map((r) => r.name);
    expect(filterAndSort(repos, '  PROJ ', 'name-asc').map((r) => r.name)).toEqual(['proj-2', 'proj-10']);
    expect(filterAndSort(repos, 'zzz', 'name-asc')).toEqual([]);
    expect(repos.map((r) => r.name)).toEqual(before);
  });
  it('falls back to a known sort', () => {
    expect(parseSort('nonsense')).toBe('name-asc');
    expect(parseSort('name-desc')).toBe('name-desc');
  });
});

describe('mapLimit', () => {
  it('never runs more than the limit at once and keeps the order', async () => {
    let running = 0;
    let peak = 0;
    const out = await mapLimit(Array.from({ length: 40 }, (_, i) => i), 6, async (n) => {
      running++;
      peak = Math.max(peak, running);
      await new Promise((r) => setTimeout(r, 2));
      running--;
      return n * 2;
    });
    expect(peak).toBeLessThanOrEqual(6);
    expect(out).toEqual(Array.from({ length: 40 }, (_, i) => i * 2));
  });
  it('handles an empty list', async () => {
    expect(await mapLimit([], 6, async () => 1)).toEqual([]);
  });
});

describe('syntax highlighting', () => {
  it('picks languages by file name', () => {
    expect(languageFor('src/main.go')).toBe('go');
    expect(languageFor('a/b/Makefile')).toBe('makefile');
    expect(languageFor('README')).toBeUndefined();
    expect(languageFor('.gitignore')).toBeUndefined();
    expect(languageFor('x.unknownext')).toBeUndefined();
  });
  it('splits a token that spans lines so every line keeps its colour', () => {
    const lines = flatten({
      type: 'root',
      children: [
        { type: 'element', properties: { className: ['hljs-comment'] }, children: [{ type: 'text', value: '/* a\nb */' }] },
        { type: 'text', value: '\nx' },
      ],
    });
    expect(lines).toEqual([[{ text: '/* a', cls: 'hljs-comment' }], [{ text: 'b */', cls: 'hljs-comment' }], [{ text: 'x', cls: undefined }]]);
  });
  it('colours Go, and shows unknown or oversized files plain', async () => {
    const go = await highlightLines('package main\nfunc f() {}\n', 'go');
    expect(go).not.toBeNull();
    expect(go!.flat().some((t) => t.cls === 'hljs-keyword')).toBe(true);
    expect(await highlightLines('x', undefined)).toBeNull();
    expect(await highlightLines('x', 'nosuchlanguage')).toBeNull();
    expect(await highlightLines('a'.repeat(MAX_HIGHLIGHT_CHARS + 1), 'go')).toBeNull();
  });
  it('never turns file content into markup', async () => {
    const lines = await highlightLines('var s = "<img src=x onerror=alert(1)>";', 'javascript');
    expect(lines!.flat().map((t) => t.text).join('')).toContain('<img src=x onerror=alert(1)>');
    expect(lines!.flat().every((t) => typeof t.text === 'string')).toBe(true);
  });
});

describe('retry policy', () => {
  it('retries network and server failures twice, never client errors', () => {
    expect(shouldRetry(0, { code: 'NETWORK', status: 0 })).toBe(true);
    expect(shouldRetry(1, { status: 503 })).toBe(true);
    expect(shouldRetry(2, { status: 503 })).toBe(false);
    for (const status of [400, 401, 403, 404, 413]) expect(shouldRetry(0, { status })).toBe(false);
    expect(shouldRetry(0, undefined)).toBe(false);
  });
});

describe('response schemas', () => {
  it('turns the Go server\'s null lists into empty ones', () => {
    expect(treeResponse.parse({ entries: null }).entries).toEqual([]);
    expect(commitsResponse.parse({ commits: null }).commits).toEqual([]);
  });
  it('tolerates extra fields and missing optional text', () => {
    const t = treeResponse.parse({ entries: [{ name: 'a', type: 'file', extra: 1 }], future: true });
    expect(t.entries[0].mode).toBe('');
  });
  it('rejects a response the pages cannot use', () => {
    expect(treeResponse.safeParse({ entries: 5 }).success).toBe(false);
    expect(treeResponse.safeParse({ entries: [{ type: 'file' }] }).success).toBe(false);
    expect(repoSummary.safeParse({ id: 'x' }).success).toBe(false);
    expect(blobResponse.safeParse({ content: 7, size: 1 }).success).toBe(false);
  });
});

describe('routes', () => {
  it('carries the branch and encodes awkward names', () => {
    expect(routes.tree('demo', '', '')).toBe('/r/demo/tree');
    expect(routes.tree('demo', 'src', 'feature/x')).toBe('/r/demo/tree/src?ref=feature%2Fx');
    expect(routes.blob('my repo', 'a b/ü.txt', '')).toBe('/r/my%20repo/blob/a%20b/%C3%BC.txt');
    expect(routes.commits('demo', 'src/main.go')).toBe('/r/demo/commits?path=src%2Fmain.go');
  });
});

describe('copyText', () => {
  afterEach(() => vi.restoreAllMocks());
  it('reports failure instead of throwing when nothing can copy', async () => {
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true });
    document.execCommand = vi.fn(() => false) as unknown as typeof document.execCommand;
    await expect(copyText('hello')).resolves.toBe(false);
  });
  it('uses the fallback on a page that is not secure', async () => {
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true });
    document.execCommand = vi.fn(() => true) as unknown as typeof document.execCommand;
    await expect(copyText('hello')).resolves.toBe(true);
    expect(document.querySelector('textarea')).toBeNull(); // the helper cleans up after itself
  });
});

describe('contribution graph note', () => {
  it('says nothing when the counts are complete', () => {
    expect(contribNote(3, false)).toBeUndefined();
    expect(contribNote(50, false)).toBeUndefined();
  });
  it('names each limit that was reached', () => {
    expect(contribNote(51, false)).toBe('Counted from the first 50 of 51 repositories.');
    expect(contribNote(3, true)).toBe('Counted from the latest 100 commits of each repository.');
    expect(contribNote(300, true)).toBe('Counted from the latest 100 commits of each of the first 50 of 300 repositories.');
  });
});
