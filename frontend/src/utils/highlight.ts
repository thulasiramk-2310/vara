// segmentText splits text into plain and matched runs for case-insensitive,
// literal-substring highlighting (the client mirror of RFC-0024 §6 matching).
// It returns SEGMENTS, never HTML — the caller renders each run as a text node
// or a <mark>, so repository content is always inert (S4). There is no string
// concatenation of markup and therefore no dangerouslySetInnerHTML anywhere.

export interface Segment {
  t: string;
  isMark?: boolean;
}

export function segmentText(text: string, q: string): Segment[] {
  const s = String(text ?? '');
  if (!q) return [{ t: s }];
  const low = s.toLowerCase();
  const ql = q.toLowerCase();
  const out: Segment[] = [];
  let i = 0;
  while (i < s.length) {
    const idx = low.indexOf(ql, i);
    if (idx === -1) {
      out.push({ t: s.slice(i) });
      break;
    }
    if (idx > i) out.push({ t: s.slice(i, idx) });
    out.push({ t: s.slice(idx, idx + q.length), isMark: true });
    i = idx + q.length;
  }
  return out;
}
