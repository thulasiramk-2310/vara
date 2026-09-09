import React from 'react';
import { Link } from 'react-router-dom';
import { useContributions } from '../hooks/useContributions';
import { useRepoCards } from '../hooks/useRepoCards';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';
import { ContributionGraph, ActivityFeed } from '../components/common/Contributions';
import { RepoCard } from '../components/common/RepoCard';

// DashboardPage lists the repositories this server exposes (RFC-0021), addressed
// by name — no owner namespace. Repository cards, the contribution graph, and the
// activity feed are all derived from real read-API data.
export const DashboardPage: React.FC = () => {
  const repos = useRepoCards();
  const contrib = useContributions();

  return (
    <div style={{ animation: 'v-fade .2s ease', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: '24px', minWidth: 0 }}>
      {/* Contribution graph — full width across the top (GitHub-profile style).
          The component renders its own card + the year selector column beside it. */}
      {contrib.loading ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '22px', boxShadow: 'var(--shadow)' }}><Loading /></div>
      ) : contrib.error ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '22px', boxShadow: 'var(--shadow)' }}><ErrorState error={contrib.error} /></div>
      ) : contrib.data ? (
        <ContributionGraph counts={contrib.data.counts} years={contrib.data.years} />
      ) : null}

      {/* Repositories (main) + Recent activity (rail) */}
      <div className="v-dash">
        <div style={{ display: 'grid', gap: '16px', minWidth: 0, alignContent: 'start' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '10px' }}>
            <div>
              <h1 style={{ fontSize: '22px', fontWeight: 660, margin: 0, letterSpacing: '-0.4px' }}>Repositories</h1>
              <p style={{ color: 'var(--dim)', margin: '3px 0 0', fontSize: '14px' }}>Repositories you can access on this server.</p>
            </div>
            <Link to={routes.newRepo} style={{ background: 'var(--accent)', color: 'var(--accent-fg)', borderRadius: '8px', padding: '8px 16px', fontSize: '13px', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap' }}>
              New repository
            </Link>
          </div>

          {repos.loading ? (
            <Loading />
          ) : repos.error ? (
            <ErrorState error={repos.error} />
          ) : repos.data && repos.data.length ? (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: '16px' }}>
              {repos.data.map((r) => (
                <RepoCard key={r.name} repo={r} />
              ))}
            </div>
          ) : (
            <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '14px', padding: '48px 20px', textAlign: 'center', color: 'var(--dim)' }}>
              No repositories yet. <Link to={routes.newRepo} style={{ color: 'var(--accent)' }}>Create one</Link>.
            </div>
          )}
        </div>

        {/* Recent activity rail */}
        <aside style={{ display: 'grid', gap: '12px', minWidth: 0, alignContent: 'start' }}>
          <h2 style={{ fontSize: '14px', fontWeight: 620, margin: 0 }}>Recent activity</h2>
          {contrib.loading ? <Loading /> : contrib.error ? <ErrorState error={contrib.error} /> : <ActivityFeed items={contrib.data?.activity || []} />}
        </aside>
      </div>
    </div>
  );
};
