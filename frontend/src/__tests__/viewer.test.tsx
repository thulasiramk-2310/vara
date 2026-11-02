import React from 'react';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, renderHook, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';

vi.mock('../api/client', async (orig) => {
  const real = await orig<typeof import('../api/client')>();
  return { ...real, api: { ...real.api, tree: vi.fn(), commits: vi.fn() } };
});

import { api } from '../api/client';
import { FileTree, FIRST_ROWS } from '../components/common/FileTree';
import { FileCommitBar } from '../components/common/FileCommitBar';
import { useStoredFlag } from '../hooks/useStoredFlag';
import { sortEntries } from '../utils/tree';

const tree = api.tree as unknown as ReturnType<typeof vi.fn>;
const commits = api.commits as unknown as ReturnType<typeof vi.fn>;

const wrap = (ui: React.ReactElement) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <MemoryRouter>{ui}</MemoryRouter>
  </QueryClientProvider>
);

const entry = (name: string, type = 'file') => ({ name, type, mode: type === 'dir' ? '40000' : '100644' });

beforeEach(() => {
  tree.mockReset();
  commits.mockReset();
});
afterEach(cleanup);

async function violations(el: HTMLElement) {
  const r = await axe.run(el, { rules: { 'color-contrast': { enabled: false }, region: { enabled: false } } });
  return r.violations.map((v) => `${v.id}: ${v.help}`);
}

describe('sortEntries', () => {
  it('lists folders first, then files, in natural order', () => {
    const out = sortEntries([entry('b.txt'), entry('file10'), entry('file2'), entry('src', 'dir'), entry('A.md'), entry('docs', 'dir')]);
    expect(out.map((e) => e.name)).toEqual(['docs', 'src', 'A.md', 'b.txt', 'file2', 'file10']);
  });
});

describe('FileTree', () => {
  const root = [entry('README.md'), entry('src', 'dir'), entry('docs', 'dir')];

  it('lists folders before files and links files with the chosen branch', async () => {
    tree.mockResolvedValue({ entries: root });
    render(wrap(<FileTree repo="demo" reference="feature" active="" />));
    const names = (await screen.findAllByRole('listitem')).map((li) => li.textContent);
    expect(names).toEqual(['docs', 'src', 'README.md']);
    expect(screen.getByRole('link', { name: 'README.md' }).getAttribute('href')).toBe('/r/demo/blob/README.md?ref=feature');
  });

  it('fetches a folder only when it is opened, and says whether it is open', async () => {
    tree.mockImplementation(async (_r: string, dir: string) => ({ entries: dir === 'src' ? [entry('main.go')] : root }));
    render(wrap(<FileTree repo="demo" reference="" active="" />));
    const src = await screen.findByRole('button', { name: /src/ });
    expect(src.getAttribute('aria-expanded')).toBe('false');
    expect(tree).toHaveBeenCalledTimes(1);
    await userEvent.click(src);
    expect(src.getAttribute('aria-expanded')).toBe('true');
    expect(await screen.findByRole('link', { name: 'main.go' })).toBeTruthy();
    expect(tree).toHaveBeenCalledWith('demo', 'src', '');
    await userEvent.click(src);
    expect(screen.queryByRole('link', { name: 'main.go' })).toBeNull();
  });

  it('opens the folders that lead to the current file and marks it', async () => {
    tree.mockImplementation(async (_r: string, dir: string) => ({
      entries: dir === '' ? root : dir === 'src' ? [entry('lib', 'dir'), entry('main.go')] : dir === 'src/lib' ? [entry('util.go')] : [],
    }));
    render(wrap(<FileTree repo="demo" reference="" active="src/lib/util.go" />));
    const current = await screen.findByRole('link', { name: 'util.go' });
    expect(current.getAttribute('aria-current')).toBe('page');
    expect(screen.getByRole('link', { name: 'main.go' }).getAttribute('aria-current')).toBeNull();
  });

  it('pages a folder of 650 entries instead of drawing them all', async () => {
    tree.mockResolvedValue({ entries: Array.from({ length: 650 }, (_, i) => entry(`f${String(i).padStart(3, '0')}.txt`)) });
    render(wrap(<FileTree repo="big" reference="" active="" />));
    await screen.findAllByRole('link');
    expect(screen.getAllByRole('link')).toHaveLength(FIRST_ROWS);
    await userEvent.click(screen.getByRole('button', { name: /Show 450 more/ }));
    expect(screen.getAllByRole('link')).toHaveLength(650);
  });

  it('shows an empty folder and a failed folder honestly', async () => {
    tree.mockImplementation(async (_r: string, dir: string) => {
      if (dir === '') return { entries: [entry('empty', 'dir'), entry('broken', 'dir')] };
      if (dir === 'empty') return { entries: [] };
      throw Object.assign(new Error('x'), { status: 500 });
    });
    render(wrap(<FileTree repo="demo" reference="" active="" />));
    await userEvent.click(await screen.findByRole('button', { name: /empty/ }));
    expect(await screen.findByText('Empty')).toBeTruthy();
    await userEvent.click(screen.getByRole('button', { name: /broken/ }));
    expect(await screen.findByText(/Could not list this folder/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  });

  it('passes the accessibility scan', async () => {
    tree.mockResolvedValue({ entries: root });
    const { container } = render(wrap(<FileTree repo="demo" reference="" active="README.md" />));
    await screen.findAllByRole('listitem');
    expect(await violations(container)).toEqual([]);
  });
});

describe('FileCommitBar', () => {
  const commit = { id: 'a'.repeat(64), message: 'fix the parser\n\nlonger body', author: 'Ada', timestamp: '2026-11-02T10:00:00Z' };

  it('names the latest commit that changed the file', async () => {
    commits.mockResolvedValue({ commits: [commit] });
    const { container } = render(wrap(<FileCommitBar repo="demo" path="src/main.go" reference="feature" />));
    expect(await screen.findByText('fix the parser')).toBeTruthy();
    expect(screen.getByText('Ada')).toBeTruthy();
    expect(commits).toHaveBeenCalledWith('demo', 1, '', 'src/main.go', 'feature');
    expect(screen.getAllByRole('link').every((l) => l.getAttribute('href') === `/r/demo/commit/${'a'.repeat(64)}`)).toBe(true);
    expect(await violations(container)).toEqual([]);
  });
  it('shows nothing when no commit touched the file', async () => {
    commits.mockResolvedValue({ commits: [] });
    const { container } = render(wrap(<FileCommitBar repo="demo" path="x" reference="" />));
    await waitFor(() => expect(commits).toHaveBeenCalled());
    await waitFor(() => expect(container.textContent).toBe(''));
  });
  it('says so when it cannot load', async () => {
    commits.mockRejectedValue(Object.assign(new Error('x'), { status: 500 }));
    render(wrap(<FileCommitBar repo="demo" path="x" reference="" />));
    expect(await screen.findByText(/could not be loaded/)).toBeTruthy();
  });
});

describe('useStoredFlag', () => {
  afterEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });
  it('remembers the choice between visits', () => {
    const first = renderHook(() => useStoredFlag('k', false));
    expect(first.result.current[0]).toBe(false);
    act(() => first.result.current[1](true));
    expect(first.result.current[0]).toBe(true);
    const second = renderHook(() => useStoredFlag('k', false));
    expect(second.result.current[0]).toBe(true);
  });
  it('keeps working when storage is blocked', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    const r = renderHook(() => useStoredFlag('k', true));
    expect(r.result.current[0]).toBe(true);
    act(() => r.result.current[1](false));
    expect(r.result.current[0]).toBe(false);
  });
});
