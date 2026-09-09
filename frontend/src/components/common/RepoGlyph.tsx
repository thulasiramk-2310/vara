import React from 'react';
import { repoAccentColor, repoShape } from '../../utils/repo';

// Six polygon silhouettes. Each repository gets one deterministically (from its
// id/name) filled in its stable accent colour — a small generated identity that
// makes repos recognizable at a glance, like a favicon per repo.
const SHAPES = [
  '12,2.5 20.2,7.25 20.2,16.75 12,21.5 3.8,16.75 3.8,7.25', // hexagon (pointy)
  '12,2.5 21.5,12 12,21.5 2.5,12', // diamond
  '12,3 21,20 3,20', // triangle
  '12,2.5 21,9.2 17.6,20 6.4,20 3,9.2', // pentagon
  '7.25,3.8 16.75,3.8 21.5,12 16.75,20.2 7.25,20.2 2.5,12', // hexagon (flat)
  '5,5 19,5 19,19 5,19', // square
];

export const RepoGlyph: React.FC<{ seed: string; size?: number; title?: string }> = ({ seed, size = 18, title }) => {
  const color = repoAccentColor(seed);
  const points = SHAPES[repoShape(seed)];
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" role="img" aria-label={title || 'repository'} style={{ flexShrink: 0 }}>
      {title ? <title>{title}</title> : null}
      <polygon points={points} fill={color} stroke={color} strokeWidth="1.4" strokeLinejoin="round" />
    </svg>
  );
};
