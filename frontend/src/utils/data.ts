import {
  Repo,
  CommitItem,
  EntryItem,
  DiffFile,
  PullItem,
  IssueItem,
  ActivityItem,
  WeekContribution,
  Segment,
} from '../types';

export const reposData: Repo[] = [
  { name: 'vara-core', desc: 'Content-addressed object store and pack format', lang: 'Rust', langColor: '#e08a6f', stars: '2.1k', updated: '2h ago', vis: 'Public' },
  { name: 'hub-web', desc: 'Read-only web browser for repositories', lang: 'TypeScript', langColor: '#6f9be0', stars: '1.4k', updated: '5h ago', vis: 'Public' },
  { name: 'measure-cli', desc: 'SHA-256 measuring tool for any working tree', lang: 'Go', langColor: '#7fc79c', stars: '860', updated: '3d ago', vis: 'Public' },
  { name: 'rfcs', desc: 'Design proposals for the measure specification', lang: 'MDX', langColor: '#b09be0', stars: '430', updated: '1d ago', vis: 'Public' },
];

export const headCommit = {
  short: '3f9a1c7',
  oid: '3f9a1c7e2b5d84a1f0c9d3e7a6b2c1908f4e5d6a7b8c9d0e1f2a3b4c5d6e7f80',
  msg: 'rfc: finalize transaction isolation',
  when: '2h ago',
};

export const entriesData: EntryItem[] = [
  { name: 'cmd', isDir: true, lastMsg: 'serve: mount hub on same origin', when: '5h' },
  { name: 'internal', isDir: true, lastMsg: 'txn: isolate concurrent updates', when: '2h' },
  { name: 'rfcs', isDir: true, lastMsg: 'RFC-021: transaction isolation', when: '2h' },
  { name: 'Cargo.toml', isDir: false, lastMsg: 'deps: pin blake3', when: '11d' },
  { name: 'LICENSE', isDir: false, lastMsg: 'legal: Apache-2.0', when: '8mo' },
  { name: 'README.md', isDir: false, lastMsg: 'docs: describe the measure', when: '3d' },
];

export const blobText = [
  'package object',
  '',
  'import (',
  '\t"crypto/sha256"',
  '\t"encoding/hex"',
  ')',
  '',
  '// Measure is the SHA-256 address of a stored object.',
  '// It is the fixed reference from which all distances are taken.',
  'type Measure [32]byte',
  '',
  'func MeasureOf(kind Kind, payload []byte) Measure {',
  '\th := sha256.New()',
  '\th.Write(header(kind, len(payload)))',
  '\th.Write(payload)',
  '\tvar m Measure',
  '\tcopy(m[:], h.Sum(nil))',
  '\treturn m',
  '}',
];

export const commitList: CommitItem[] = [
  { short: '3f9a1c7', msg: 'rfc: finalize transaction isolation', author: 'a.rivest', initials: 'AR', avatar: '#5e8bff', when: '2h ago', absTime: '2026-07-28 09:14 UTC' },
  { short: '71ff0ab', msg: 'hub: inert render for untrusted blobs', author: 'l.mendez', initials: 'LM', avatar: '#c266d6', when: '5h ago', absTime: '2026-07-28 06:02 UTC' },
  { short: '9b2e004', msg: 'RFC-014: content-addressed object pruning', author: 'k.okafor', initials: 'KO', avatar: '#3fb27f', when: '1d ago', absTime: '2026-07-27 11:40 UTC' },
  { short: 'c14d8a2', msg: 'pack: verify measure on read', author: 'a.rivest', initials: 'AR', avatar: '#5e8bff', when: '3d ago', absTime: '2026-07-25 15:22 UTC' },
  { short: '6d2f118', msg: 'store: content-address loose objects', author: 'k.okafor', initials: 'KO', avatar: '#3fb27f', when: '4d ago', absTime: '2026-07-24 08:10 UTC' },
];

export const diffData: DiffFile[] = [
  {
    path: 'internal/txn/isolation.go',
    badge: 'M',
    add: 14,
    del: 3,
    rows: [
      { k: 'hunk', text: '@@ -18,7 +18,18 @@ func (t *Txn) commit() error {' },
      { k: 'ctx', oldN: 18, newN: 18, text: '\t\tif t.done {' },
      { k: 'ctx', oldN: 19, newN: 19, text: '\t\t\treturn ErrClosed' },
      { k: 'del', oldN: 20, text: '\t\tt.store.apply(t.pending)' },
      { k: 'add', newN: 20, text: '\tsnap := t.store.refs.snapshot()' },
      { k: 'add', newN: 21, text: '\tif err := t.store.apply(t.pending, snap); err != nil {' },
      { k: 'add', newN: 22, text: '\t\treturn fmt.Errorf("isolate: %w", err)' },
      { k: 'add', newN: 23, text: '\t}' },
      { k: 'ctx', oldN: 21, newN: 24, text: '\t\tt.done = true' },
    ],
  },
  {
    path: 'internal/ref/update.go',
    badge: 'M',
    add: 6,
    del: 6,
    rows: [
      { k: 'hunk', text: '@@ -40,9 +40,9 @@ func Update(name string, old, new Measure) error {' },
      { k: 'ctx', oldN: 40, newN: 40, text: '\tdefer lk.release()' },
      { k: 'del', oldN: 41, text: '\tcur := read(name)' },
      { k: 'add', newN: 41, text: '\tcur, ok := read(name)' },
      { k: 'ctx', oldN: 42, newN: 42, text: '\tif cur != old {' },
    ],
  },
  {
    path: 'rfcs/RFC-021.md',
    badge: 'A',
    add: 42,
    del: 0,
    rows: [
      { k: 'hunk', text: '@@ -0,0 +1,42 @@' },
      { k: 'add', newN: 1, text: '# RFC-021 — Transaction Isolation' },
      { k: 'add', newN: 2, text: '' },
      { k: 'add', newN: 3, text: 'Concurrent ref updates must observe a consistent' },
    ],
  },
];

export const pullsData: PullItem[] = [
  { num: 42, title: 'Isolate concurrent ref updates', state: 'open', meta: 'opened 2h ago by a.rivest', branch: 'txn/isolation', comments: 4 },
  { num: 39, title: 'Add prebuilt Windows binaries to release', state: 'open', meta: 'opened 1d ago by l.mendez', branch: 'ci/windows', comments: 2 },
  { num: 37, title: 'pack: verify measure on read', state: 'merged', meta: 'merged 3d ago by a.rivest', branch: 'pack/verify', comments: 6 },
  { num: 31, title: 'Drop legacy ref format', state: 'closed', meta: 'closed 6d ago by k.okafor', branch: 'ref/cleanup', comments: 0 },
];

export const issuesData: IssueItem[] = [
  { num: 58, title: 'Search should support case-sensitive toggle', state: 'open', meta: 'opened 4h ago by k.okafor', labels: [{ name: 'enhancement', bg: 'var(--green-soft)', fg: 'var(--green)' }], comments: 3 },
  { num: 55, title: 'Diff view wraps long lines on mobile', state: 'open', meta: 'opened 1d ago by l.mendez', labels: [{ name: 'bug', bg: 'var(--red-soft)', fg: 'var(--red)' }, { name: 'ui', bg: 'var(--accent-soft)', fg: 'var(--accent)' }], comments: 1 },
  { num: 52, title: 'Document the pack format on disk', state: 'open', meta: 'opened 2d ago by a.rivest', labels: [{ name: 'docs', bg: 'var(--purple-soft)', fg: 'var(--purple)' }], comments: 0 },
  { num: 47, title: '304 not returned for content-addressed ETags', state: 'open', meta: 'opened 5d ago by k.okafor', labels: [{ name: 'bug', bg: 'var(--red-soft)', fg: 'var(--red)' }], comments: 5 },
];

export const activityData: ActivityItem[] = [
  { who: 'a.rivest', action: 'pushed to', target: 'vara-core@main', when: '2h ago', initials: 'AR', avatar: '#5e8bff' },
  { who: 'k.okafor', action: 'opened issue', target: '#58', when: '4h ago', initials: 'KO', avatar: '#3fb27f' },
  { who: 'l.mendez', action: 'merged', target: 'hub-web#37', when: '5h ago', initials: 'LM', avatar: '#c266d6' },
  { who: 'a.rivest', action: 'released', target: 'v0.4.0', when: '5h ago', initials: 'AR', avatar: '#5e8bff' },
  { who: 'k.okafor', action: 'starred', target: 'measure-cli', when: '1d ago', initials: 'KO', avatar: '#3fb27f' },
];

export const searchDataC: Record<string, { truncated: boolean; files: Array<{ path: string; hits: string; lines: Array<{ n?: number; text: string }> }> }> = {
  content: {
    truncated: true,
    files: [
      { path: 'internal/object/hash.go', hits: '3 matches', lines: [{ n: 8, text: '// Measure is the SHA-256 address of a stored object.' }, { n: 9, text: '// It is the fixed reference from which all distances are taken.' }] },
      { path: 'internal/pack/verify.go', hits: '2 matches', lines: [{ n: 22, text: '\t// re-measure the object and compare against the index' }, { n: 23, text: '\tif got := MeasureOf(kind, buf); got != want {' }] },
    ],
  },
  paths: {
    truncated: false,
    files: [
      { path: 'internal/object/measure.go', hits: '', lines: [] },
      { path: 'testdata/measure/empty.bin', hits: '', lines: [] },
    ],
  },
  commits: {
    truncated: false,
    files: [
      { path: 'c14d8a2 · pack: verify measure on read', hits: 'a.rivest · 3d', lines: [] },
    ],
  },
};

export function buildHeatmap(): WeekContribution[] {
  const weeks: WeekContribution[] = [];
  let seed = 7;
  const rnd = () => {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return seed / 0x7fffffff;
  };
  const cols = ['var(--heat0)', 'var(--heat1)', 'var(--heat2)', 'var(--heat3)', 'var(--heat4)'];
  for (let w = 0; w < 52; w++) {
    const days = [];
    for (let d = 0; d < 7; d++) {
      const r = rnd();
      const lvl = r < 0.45 ? 0 : r < 0.68 ? 1 : r < 0.84 ? 2 : r < 0.94 ? 3 : 4;
      days.push({ color: cols[lvl], count: lvl * 3 });
    }
    weeks.push({ days });
  }
  return weeks;
}

export function segmentText(text: string, q: string): Segment[] {
  if (!q) return [{ t: text, isPlain: true }];
  const out: Segment[] = [];
  const low = text.toLowerCase();
  const ql = q.toLowerCase();
  let i = 0;
  while (i < text.length) {
    const idx = low.indexOf(ql, i);
    if (idx === -1) {
      out.push({ t: text.slice(i), isPlain: true });
      break;
    }
    if (idx > i) {
      out.push({ t: text.slice(i, idx), isPlain: true });
    }
    out.push({ t: text.slice(idx, idx + q.length), isMark: true });
    i = idx + q.length;
  }
  return out;
}

export const styledReposData: Repo[] = reposData.map((r, i) => {
  const tones = ['pink', 'yellow', 'lilac', 'mint'];
  const tn = tones[i % 4];
  return {
    ...r,
    cardBg: `var(--${tn}-bg)`,
    cardInk: `var(--${tn}-ink)`,
    cardSub: `var(--${tn}-sub)`,
  };
});
