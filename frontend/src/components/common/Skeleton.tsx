import React from 'react';

// Skel is one placeholder block. Callers give it the size of the thing it
// stands in for, so the page does not jump when the real content arrives.
export const Skel: React.FC<{ w?: string | number; h?: string | number; style?: React.CSSProperties }> = ({
  w = '100%',
  h = 14,
  style,
}) => <span className="v-skel" aria-hidden="true" style={{ width: w, height: h, ...style }} />;

export const ROW_HEIGHT = 44;

// SkelRows mimics a list of rows (file tree, commits, repositories): a leading
// mark, a title and a trailing detail, inside the same bordered panel the real
// list uses.
export const SkelRows: React.FC<{ rows?: number }> = ({ rows = 5 }) => (
  <div
    aria-hidden="true"
    style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: 'var(--s-3)', overflow: 'hidden' }}
  >
    {Array.from({ length: rows }, (_, i) => (
      <div
        key={i}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--s-3)',
          height: ROW_HEIGHT,
          padding: '0 var(--s-4)',
          borderTop: i === 0 ? 'none' : '1px solid var(--border)',
        }}
      >
        <Skel w={16} h={16} />
        <Skel w={`${38 + ((i * 17) % 32)}%`} h={13} />
        <span style={{ flex: 1 }} />
        <Skel w={56} h={11} />
      </div>
    ))}
  </div>
);

// PageSkeleton fills the space while a route's code is still arriving.
export const PageSkeleton: React.FC = () => (
  <div style={{ maxWidth: 'var(--content)', margin: '0 auto', padding: 'var(--s-6) var(--s-4)' }}>
    <span className="v-sr-only" role="status">Loading</span>
    <Skel w={220} h={28} style={{ marginBottom: 'var(--s-5)' }} />
    <SkelRows rows={6} />
  </div>
);
