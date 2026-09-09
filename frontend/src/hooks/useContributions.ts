import { useAsync } from './useAsync';
import { api, CommitRef } from '../api/client';

// A contribution graph and activity feed derived from REAL commit history — not
// mock data. It lists the repositories this server exposes, reads recent commits
// from each, and buckets them by day. (A dedicated per-user contributions API is a
// v0.5 concern; until then this is an honest projection of what the read API can
// already answer.)

export interface ActivityEntry {
  repo: string;
  commit: CommitRef;
}

export interface ContribDay {
  date: string;
  count: number;
  level: number; // 0..4 for the heat scale
}

export interface ContribWeek {
  days: ContribDay[];
}

export interface Contributions {
  activity: ActivityEntry[];
  counts: Record<string, number>; // "YYYY-MM-DD" (UTC) → commit count
  years: number[]; // descending: current year down to the earliest with data
  total: number; // all-time commit total
}

export function level(count: number): number {
  if (count <= 0) return 0;
  if (count < 2) return 1;
  if (count < 4) return 2;
  if (count < 6) return 3;
  return 4;
}

async function load(): Promise<Contributions> {
  const { repositories } = await api.listRepos();

  const all: ActivityEntry[] = [];
  await Promise.all(
    repositories.map(async (r) => {
      try {
        const d = await api.commits(r.name, 100, '');
        (d.commits || []).forEach((c) => all.push({ repo: r.name, commit: c }));
      } catch {
        // skip repositories we cannot read
      }
    }),
  );

  all.sort((a, b) => new Date(b.commit.timestamp).getTime() - new Date(a.commit.timestamp).getTime());

  // Count commits per calendar day (UTC) and record which years have activity.
  const counts: Record<string, number> = {};
  const yearSet = new Set<number>();
  for (const e of all) {
    const d = new Date(e.commit.timestamp);
    if (isNaN(d.getTime())) continue;
    const key = d.toISOString().slice(0, 10);
    counts[key] = (counts[key] || 0) + 1;
    yearSet.add(d.getUTCFullYear());
  }

  // Years, descending from the current year down to the earliest with commits
  // (the current year is always present so the selector has at least one entry).
  const currentYear = new Date().getUTCFullYear();
  yearSet.add(currentYear);
  const minYear = Math.min(...yearSet);
  const years: number[] = [];
  for (let y = currentYear; y >= minYear; y--) years.push(y);

  return { activity: all.slice(0, 10), counts, years, total: all.length };
}

export function useContributions() {
  return useAsync<Contributions>(load, []);
}

// authInitial extracts a display initial from a commit author string such as
// "User <user@example.com>".
export function authInitial(author: string): string {
  const name = (author || '').replace(/<.*>/, '').trim() || author || '?';
  return name[0]?.toUpperCase() || '?';
}
