export type Theme = 'light' | 'dark';

// NOTE: the interfaces below back the design-prototype mock data (src/utils/data.ts)
// used by the not-yet-wired chrome (dashboard feed, explore). The live read views
// (code, blob, commits, diff, search) use the typed API client in src/api/client.ts
// instead. These stay until the chrome is either wired or replaced.

export interface Repo {
  name: string;
  desc: string;
  lang: string;
  langColor: string;
  stars: string;
  updated: string;
  vis: string;
  cardBg?: string;
  cardInk?: string;
  cardSub?: string;
}

export interface CommitItem {
  short: string;
  msg: string;
  author: string;
  initials: string;
  avatar: string;
  when: string;
  absTime: string;
}

export interface EntryItem {
  name: string;
  isDir: boolean;
  lastMsg: string;
  when: string;
}

export interface DiffRow {
  k: 'hunk' | 'ctx' | 'add' | 'del';
  oldN?: number;
  newN?: number;
  text: string;
}

export interface DiffFile {
  path: string;
  badge: string;
  add: number;
  del: number;
  rows: DiffRow[];
}

export interface PullItem {
  num: number;
  title: string;
  state: 'open' | 'merged' | 'closed';
  meta: string;
  branch: string;
  comments: number;
}

export interface IssueItem {
  num: number;
  title: string;
  state: 'open' | 'closed';
  meta: string;
  labels: Array<{ name: string; bg: string; fg: string }>;
  comments: number;
}

export interface ActivityItem {
  who: string;
  action: string;
  target: string;
  when: string;
  initials: string;
  avatar: string;
}

export interface DayContribution {
  color: string;
  count: number;
}

export interface WeekContribution {
  days: DayContribution[];
}

export interface Segment {
  t: string;
  isPlain?: boolean;
  isMark?: boolean;
}
