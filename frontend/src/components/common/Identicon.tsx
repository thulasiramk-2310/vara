import React from 'react';

// A deterministic, GitHub-style identicon generated from a seed string (username,
// author, etc.). Same seed → same avatar. Hues are restricted to warm / amber /
// green / cyan / blue bands — violet and purple are deliberately excluded.

function hashStr(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

// Allowed hue bands (degrees), excluding the ~255–330 violet/purple/magenta range.
const HUE_BANDS: Array<[number, number]> = [
  [12, 42],   // warm orange
  [42, 70],   // amber
  [140, 175], // green
  [160, 195], // teal
  [195, 245], // cyan → blue
];

export const Identicon: React.FC<{ seed: string; size?: number; round?: boolean }> = ({ seed, size = 32, round = true }) => {
  const h = hashStr(seed || 'anonymous');
  const band = HUE_BANDS[h % HUE_BANDS.length];
  const hue = band[0] + ((h >> 8) % (band[1] - band[0]));
  const fg = `hsl(${hue} 58% 45%)`;
  const bg = `hsl(${hue} 38% 93%)`;

  const grid = 5;
  const cell = size / grid;
  const rects: React.ReactNode[] = [];
  // 5×5, vertically mirrored: decide the left 3 columns, mirror to the right.
  for (let r = 0; r < grid; r++) {
    for (let c = 0; c < 3; c++) {
      const on = ((h >> (r * 3 + c)) & 1) === 1;
      if (!on) continue;
      const cols = c === 2 ? [2] : [c, 4 - c];
      for (const cc of cols) {
        rects.push(<rect key={`${r}-${cc}`} x={cc * cell} y={r * cell} width={cell + 0.5} height={cell + 0.5} fill={fg} />);
      }
    }
  }

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      style={{ borderRadius: round ? '50%' : '20%', display: 'block', flexShrink: 0 }}
      role="img"
      aria-label={`${seed} avatar`}
    >
      <rect width={size} height={size} fill={bg} />
      {rects}
    </svg>
  );
};
