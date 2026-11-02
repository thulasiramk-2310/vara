import React from 'react';
import { Link } from 'react-router-dom';
import { api, short, when } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';
import { routes } from '../../routes';
import { Skel } from './Skeleton';

const bar: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  flexWrap: 'wrap',
  gap: '6px 12px',
  minHeight: 46,
  padding: '8px 14px',
  background: 'var(--panel)',
  border: '1px solid var(--border)',
  borderRadius: 12,
  fontSize: '13px',
};

// FileCommitBar names the latest commit that changed this file: who, what, when,
// and the short id, so a reader knows how current the file is before reading it.
// The height is fixed while it loads so the page does not jump.
export const FileCommitBar: React.FC<{ repo: string; path: string; reference: string }> = ({ repo, path, reference }) => {
  const q = useAsync('file-commit', () => api.commits(repo, 1, '', path, reference), [repo, path, reference]);

  if (q.loading) {
    return (
      <div style={bar} role="status">
        <span className="v-sr-only">Loading the latest commit</span>
        <Skel w={180} h={13} />
        <Skel w={90} h={12} />
      </div>
    );
  }
  if (q.error) {
    return <div style={{ ...bar, color: 'var(--dim)' }}>The latest commit for this file could not be loaded.</div>;
  }
  const c = q.data?.commits[0];
  if (!c) return null;

  return (
    <div style={bar}>
      <span style={{ color: 'var(--dim)' }}>Latest change</span>
      <Link to={routes.commit(repo, c.id)} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', maxWidth: '48ch', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {c.message.split('\n')[0] || '(no message)'}
      </Link>
      <span style={{ flex: 1 }} />
      <span style={{ color: 'var(--dim)' }}>{c.author}</span>
      <span style={{ color: 'var(--dim)' }}>{when(c.timestamp)}</span>
      <Link to={routes.commit(repo, c.id)} style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', background: 'var(--chip)', border: '1px solid var(--chip-border)', borderRadius: 6, padding: '2px 8px', color: 'var(--accent-strong)', textDecoration: 'none' }}>
        {short(c.id)}
      </Link>
    </div>
  );
};
