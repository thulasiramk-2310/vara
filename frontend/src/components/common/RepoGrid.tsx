import React from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { RepoCard } from './RepoCard';
import { Empty } from './States';
import { ListedRepo, REPO_SORTS, filterAndSort, parseSort } from '../../utils/repoList';
import { routes } from '../../routes';

// A page draws at most this many cards, then offers "Show more". Each card makes
// its own requests, so an unbounded list would mean an unbounded burst.
export const PAGE_SIZE = 24;

const control: React.CSSProperties = {
  minHeight: 36,
  padding: '0 var(--s-3)',
  borderRadius: 6,
  border: '1px solid var(--border-strong)',
  background: 'var(--panel)',
  color: 'var(--text)',
  fontSize: '14px',
};

// RepoGrid shows a filterable, sortable list of repositories. The filter and the
// order live in the address (?q= and ?sort=), so a filtered view can be shared
// and survives a refresh.
export const RepoGrid: React.FC<{ repos: ListedRepo[]; minCard?: number }> = ({ repos, minCard = 260 }) => {
  const [params, setParams] = useSearchParams();
  const q = params.get('q') || '';
  const sort = parseSort(params.get('sort'));
  const [pages, setPages] = React.useState(1);

  const shown = React.useMemo(() => filterAndSort(repos, q, sort), [repos, q, sort]);
  const visible = shown.slice(0, pages * PAGE_SIZE);

  const set = (key: string, value: string, fallback = '') => {
    const next = new URLSearchParams(params);
    if (value && value !== fallback) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
    setPages(1);
  };

  if (repos.length === 0) {
    return (
      <Empty>
        No repositories yet. <Link to={routes.newRepo} className="v-link">Create one</Link>.
      </Empty>
    );
  }

  return (
    <div style={{ display: 'grid', gap: 'var(--s-4)' }}>
      {repos.length > 1 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--s-3)' }}>
          <input
            type="search"
            value={q}
            onChange={(e) => set('q', e.target.value)}
            placeholder="Filter repositories"
            aria-label="Filter repositories"
            style={{ ...control, flex: '1 1 200px', minWidth: 0 }}
          />
          <select value={sort} onChange={(e) => set('sort', e.target.value, 'name-asc')} aria-label="Sort repositories" style={control}>
            {REPO_SORTS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </div>
      )}

      {shown.length === 0 ? (
        <Empty>
          Nothing matches <strong style={{ color: 'var(--text)' }}>{q}</strong>.{' '}
          <button type="button" className="v-link" style={{ background: 'none', border: 0, padding: 0, cursor: 'pointer', font: 'inherit' }} onClick={() => set('q', '')}>
            Clear the filter
          </button>
        </Empty>
      ) : (
        <>
          <div role="list" style={{ display: 'grid', gridTemplateColumns: `repeat(auto-fill, minmax(${minCard}px, 1fr))`, gap: 'var(--s-4)' }}>
            {visible.map((r) => (
              <div role="listitem" key={r.name} style={{ display: 'contents' }}>
                <RepoCard repo={r} />
              </div>
            ))}
          </div>
          <div aria-live="polite" style={{ color: 'var(--dim)', fontSize: '13px', display: 'flex', alignItems: 'center', gap: 'var(--s-3)' }}>
            <span>
              Showing {visible.length} of {shown.length}
              {q ? ' matching' : ''}
            </span>
            {visible.length < shown.length && (
              <button type="button" className="v-btn" onClick={() => setPages((n) => n + 1)}>
                Show more
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
};
