import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, Folder, File as FileIcon } from 'lucide-react';
import { api } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';
import { routes } from '../../routes';
import { sortEntries, joinPath } from '../../utils/tree';
import { Skel } from './Skeleton';

// A folder lists this many entries at first; "Show more" adds the rest in steps.
// A folder of 3,000 files must not mean 3,000 rows drawn at once.
export const FIRST_ROWS = 200;
export const MORE_ROWS = 500;

// Past this depth folders stop indenting further, so names in a very deep path
// stay readable inside the sidebar.
const MAX_INDENT_LEVELS = 8;
const pad = (depth: number) => 8 + Math.min(depth, MAX_INDENT_LEVELS) * 14;

const row: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  minHeight: 28,
  padding: '0 8px',
  borderRadius: 6,
  color: 'var(--text)',
  textDecoration: 'none',
  fontSize: '13px',
  width: '100%',
  background: 'none',
  border: 0,
  cursor: 'pointer',
  textAlign: 'left',
  fontFamily: 'inherit',
};

// Folder lists one directory. Its contents are fetched when it is first drawn,
// through the same cache the repository page uses, so a folder you already looked
// at opens instantly.
const Folder_: React.FC<{ repo: string; reference: string; dir: string; active: string; depth: number }> = ({
  repo,
  reference,
  dir,
  active,
  depth,
}) => {
  const q = useAsync('tree', () => api.tree(repo, dir, reference), [repo, dir, reference]);
  const [shown, setShown] = useState(FIRST_ROWS);

  if (q.loading) {
    return (
      <div role="status" style={{ padding: '4px 8px', paddingLeft: pad(depth), display: 'grid', gap: 8 }}>
        <span className="v-sr-only">Loading folder</span>
        <Skel w="70%" h={12} />
        <Skel w="55%" h={12} />
      </div>
    );
  }
  if (q.error) {
    return (
      <div style={{ padding: '4px 8px', paddingLeft: pad(depth), fontSize: 12.5, color: 'var(--dim)' }}>
        Could not list this folder.{' '}
        <button type="button" className="v-link" style={{ background: 'none', border: 0, padding: 0, cursor: 'pointer', font: 'inherit' }} onClick={q.retry}>
          Try again
        </button>
      </div>
    );
  }

  const entries = sortEntries(q.data?.entries ?? []);
  if (entries.length === 0) {
    return <div style={{ padding: '4px 8px', paddingLeft: pad(depth), fontSize: 12.5, color: 'var(--faint)' }}>Empty</div>;
  }
  const visible = entries.slice(0, shown);

  return (
    <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
      {visible.map((e) => (
        <Entry key={e.name} repo={repo} reference={reference} dir={dir} entry={e} active={active} depth={depth} />
      ))}
      {entries.length > shown && (
        <li style={{ paddingLeft: pad(depth) }}>
          <button type="button" className="v-link" style={{ background: 'none', border: 0, padding: '4px 8px', cursor: 'pointer', font: 'inherit', fontSize: 12.5 }} onClick={() => setShown((n) => n + MORE_ROWS)}>
            Show {Math.min(MORE_ROWS, entries.length - shown)} more of {entries.length - shown}
          </button>
        </li>
      )}
    </ul>
  );
};

const Entry: React.FC<{
  repo: string;
  reference: string;
  dir: string;
  entry: { name: string; type: string };
  active: string;
  depth: number;
}> = ({ repo, reference, dir, entry, active, depth }) => {
  const full = joinPath(dir, entry.name);
  const isDir = entry.type === 'dir';
  // The folders that lead to the open file start open, so the file is visible.
  const [open, setOpen] = useState(isDir && active.startsWith(full + '/'));
  const indent = pad(depth);

  if (!isDir) {
    const current = full === active;
    return (
      <li>
        <Link
          to={routes.blob(repo, full, reference)}
          aria-current={current ? 'page' : undefined}
          style={{ ...row, paddingLeft: indent + 18, background: current ? 'var(--accent-soft)' : 'none', fontWeight: current ? 650 : 400 }}
        >
          <FileIcon size={14} aria-hidden="true" style={{ color: 'var(--dim)', flexShrink: 0 }} />
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{entry.name}</span>
        </Link>
      </li>
    );
  }
  return (
    <li>
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} style={{ ...row, paddingLeft: indent }}>
        <ChevronRight size={14} aria-hidden="true" style={{ flexShrink: 0, transform: open ? 'rotate(90deg)' : 'none', transition: 'transform .12s' }} />
        <Folder size={14} aria-hidden="true" style={{ color: 'var(--accent-strong)', flexShrink: 0 }} />
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{entry.name}</span>
      </button>
      {open && <Folder_ repo={repo} reference={reference} dir={full} active={active} depth={depth + 1} />}
    </li>
  );
};

// FileTree is the collapsible folder tree beside a file. It is built from plain
// buttons and links (a disclosure list), so Tab, Enter and Space work and screen
// readers announce each folder as expanded or collapsed.
export const FileTree: React.FC<{ repo: string; reference: string; active: string }> = ({ repo, reference, active }) => (
  <nav aria-label="Files in this repository">
    <Folder_ repo={repo} reference={reference} dir="" active={active} depth={0} />
  </nav>
);
