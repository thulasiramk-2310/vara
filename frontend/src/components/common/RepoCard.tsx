import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Lock, Globe, GitBranch, GitCommit, Clock } from 'lucide-react';
import { RepoCardData } from '../../hooks/useRepoCards';
import { repoTone } from '../../utils/repo';
import { RepoGlyph } from './RepoGlyph';
import { when } from '../../api/client';
import { routes } from '../../routes';

// RepoCard: a premium repository card. Its colour is a stable pastel tone derived
// from the repo id/name (so repos look distinct and keep their colour), and the
// tone is a CSS variable pair — it adapts to light and dark theme. Everything shown
// is real: visibility, language (inferred from files), default branch, commit count,
// and last-updated.
export const RepoCard: React.FC<{ repo: RepoCardData }> = ({ repo }) => {
  const [hover, setHover] = useState(false);
  const t = repoTone(repo.summary?.id || repo.name);
  const s = repo.summary;
  const isPrivate = repo.visibility !== 'public';

  return (
    <Link
      to={routes.repo(repo.name)}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: 'grid',
        gap: '13px',
        alignContent: 'start',
        background: t.bg,
        color: t.ink,
        border: '1px solid transparent',
        borderRadius: '14px',
        padding: '16px 18px',
        textDecoration: 'none',
        transform: hover ? 'translateY(-3px)' : 'none',
        boxShadow: hover ? 'var(--shadow-lg)' : '0 1px 2px rgba(0,0,0,0.05)',
        transition: 'transform .18s ease, box-shadow .18s ease',
      }}
    >
      {/* Title row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '9px' }}>
        <RepoGlyph seed={repo.summary?.id || repo.name} size={18} />
        <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 700, fontSize: '15px', letterSpacing: '-0.2px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {repo.name}
        </span>
        <span style={{ flex: 1 }} />
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '10.5px', fontWeight: 600, color: t.sub, border: '1px solid currentColor', borderRadius: '20px', padding: '2px 9px', opacity: 0.85 }}>
          {isPrivate ? <Lock size={11} /> : <Globe size={11} />}
          {isPrivate ? 'Private' : 'Public'}
        </span>
      </div>

      {/* Language + state */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '14px', fontSize: '12px', color: t.sub, minHeight: '16px' }}>
        {repo.language ? (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
            <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: repo.language.color, flexShrink: 0, boxShadow: '0 0 0 1px rgba(0,0,0,0.08)' }} />
            {repo.language.name}
          </span>
        ) : (
          <span style={{ opacity: 0.7 }}>—</span>
        )}
        <span style={{ textTransform: 'capitalize', opacity: 0.8 }}>{repo.state}</span>
      </div>

      {/* Metadata footer */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px', fontSize: '12px', color: t.sub, flexWrap: 'wrap' }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '5px' }} title="Default branch">
          <GitBranch size={13} /> {s?.default_branch || '—'}
        </span>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '5px' }} title="Commits">
          <GitCommit size={13} /> {s ? s.commit_count : '—'}
        </span>
        <span style={{ flex: 1 }} />
        {s?.last_commit ? (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', opacity: 0.85 }} title="Last updated">
            <Clock size={12} /> {when(s.last_commit.timestamp)}
          </span>
        ) : null}
      </div>
    </Link>
  );
};
