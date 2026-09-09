// Per-repository visual identity + language inference — all derived from REAL data
// (the repo name/id and its actual file extensions), never fabricated.

// --- deterministic accent per repository -----------------------------------

function hashStr(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

// A repository tone is one of the app's pastel palettes, each defined as CSS custom
// properties with BOTH a light and a dark value — so the card colour adapts to the
// active theme (unlike a fixed HSL). A repo maps to a stable tone via its id/name,
// so its colour never changes between sessions.
export interface Tone {
  bg: string;
  ink: string;
  sub: string;
}

const TONES: Tone[] = [
  { bg: 'var(--pink-bg)', ink: 'var(--pink-ink)', sub: 'var(--pink-sub)' },
  { bg: 'var(--yellow-bg)', ink: 'var(--yellow-ink)', sub: 'var(--yellow-sub)' },
  { bg: 'var(--lilac-bg)', ink: 'var(--lilac-ink)', sub: 'var(--lilac-sub)' },
  { bg: 'var(--mint-bg)', ink: 'var(--mint-ink)', sub: 'var(--mint-sub)' },
  { bg: 'var(--peach-bg)', ink: 'var(--peach-ink)', sub: 'var(--peach-sub)' },
];

export function repoTone(seed: string): Tone {
  return TONES[hashStr(seed || 'repo') % TONES.length];
}

// A stable, vivid accent colour for a repository (its "brand"), derived from the
// id/name. Hue avoids the violet/purple band. Used for the generated repo glyph.
const ACCENT_BANDS: Array<[number, number]> = [
  [8, 40],
  [40, 66],
  [96, 150],
  [150, 195],
  [200, 248],
];
export function repoAccentColor(seed: string): string {
  const h = hashStr(seed || 'repo');
  const b = ACCENT_BANDS[h % ACCENT_BANDS.length];
  const hue = b[0] + (Math.floor(h / 13) % (b[1] - b[0]));
  return `hsl(${hue} 58% 52%)`;
}

// A stable shape index (0..5) so each repository gets a recognizable glyph.
export function repoShape(seed: string): number {
  return Math.floor(hashStr(seed || 'repo') / 3) % 6;
}

// --- language inference from file extensions --------------------------------

export interface Lang {
  name: string;
  color: string; // the language's conventional dot color
}

// Common languages by extension. Docs/config are excluded from "dominant language"
// so a repo isn't labelled "Markdown" just because it has a README.
const LANGS: Record<string, Lang> = {
  go: { name: 'Go', color: '#00ADD8' },
  ts: { name: 'TypeScript', color: '#3178C6' },
  tsx: { name: 'TypeScript', color: '#3178C6' },
  js: { name: 'JavaScript', color: '#F1E05A' },
  jsx: { name: 'JavaScript', color: '#F1E05A' },
  mjs: { name: 'JavaScript', color: '#F1E05A' },
  py: { name: 'Python', color: '#3572A5' },
  rs: { name: 'Rust', color: '#DEA584' },
  java: { name: 'Java', color: '#B07219' },
  rb: { name: 'Ruby', color: '#701516' },
  c: { name: 'C', color: '#555555' },
  h: { name: 'C', color: '#555555' },
  cpp: { name: 'C++', color: '#F34B7D' },
  cc: { name: 'C++', color: '#F34B7D' },
  cs: { name: 'C#', color: '#178600' },
  php: { name: 'PHP', color: '#4F5D95' },
  swift: { name: 'Swift', color: '#F05138' },
  kt: { name: 'Kotlin', color: '#A97BFF' },
  sh: { name: 'Shell', color: '#89E051' },
  html: { name: 'HTML', color: '#E34C26' },
  css: { name: 'CSS', color: '#563D7C' },
  scss: { name: 'SCSS', color: '#C6538C' },
  vue: { name: 'Vue', color: '#41B883' },
  lua: { name: 'Lua', color: '#000080' },
  dart: { name: 'Dart', color: '#00B4AB' },
  ex: { name: 'Elixir', color: '#6E4A7E' },
  zig: { name: 'Zig', color: '#EC915C' },
};

function ext(name: string): string {
  const i = name.lastIndexOf('.');
  return i > 0 ? name.slice(i + 1).toLowerCase() : '';
}

// inferLanguage picks the most common recognized code language among a set of file
// names (typically a repository's root listing). Returns null when nothing matches.
export function inferLanguage(names: string[]): Lang | null {
  const tally = new Map<string, number>();
  for (const n of names) {
    const l = LANGS[ext(n)];
    if (l) tally.set(l.name, (tally.get(l.name) || 0) + 1);
  }
  if (!tally.size) return null;
  let best = '';
  let bestN = 0;
  for (const [name, n] of tally) {
    if (n > bestN) {
      best = name;
      bestN = n;
    }
  }
  const color = Object.values(LANGS).find((l) => l.name === best)?.color || '#888';
  return { name: best, color };
}
