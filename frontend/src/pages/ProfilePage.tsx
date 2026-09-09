import React from 'react';
import { Link } from 'react-router-dom';
import { api, Whoami } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { useContributions } from '../hooks/useContributions';
import { useRepoCards } from '../hooks/useRepoCards';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';
import { ContributionGraph, ActivityFeed } from '../components/common/Contributions';
import { RepoCard } from '../components/common/RepoCard';
import { Identicon } from '../components/common/Identicon';

// ProfilePage shows the signed-in identity (RFC-0020 whoami), the repositories on
// this server, and a real contribution graph + activity feed derived from commit
// history. Rich per-user ownership (orgs, followers) lands with v0.5; this is an
// honest projection of what the read API already answers.
export const ProfilePage: React.FC = () => {
  const me = useAsync<Whoami>(() => api.whoami().catch(() => ({ id: 'anonymous', anonymous: true })), []);
  const repos = useRepoCards();
  const contrib = useContributions();

  const identity = me.data?.id || 'anonymous';

  return (
    <div className="v-profile" style={{ animation: 'v-fade .2s ease' }}>
      {/* Identity sidebar */}
      <aside style={{ display: 'grid', gap: '16px', position: 'sticky', top: '80px' }}>
        <Identicon seed={identity} size={120} />
        <div>
          <h1 style={{ fontSize: '22px', fontWeight: 700, margin: 0, fontFamily: 'var(--font-mono)', wordBreak: 'break-all' }}>{identity}</h1>
          <p style={{ color: 'var(--dim)', fontSize: '13.5px', margin: '4px 0 0' }}>
            {me.data?.anonymous ? 'Browsing anonymously' : 'Signed in'}
          </p>
        </div>
        <div style={{ display: 'flex', gap: '18px', fontSize: '13px', color: 'var(--dim)' }}>
          <span><b style={{ color: 'var(--text)' }}>{repos.data?.length ?? '—'}</b> repositories</span>
          <span><b style={{ color: 'var(--text)' }}>{contrib.data?.total ?? '—'}</b> commits</span>
        </div>
        <Link to={routes.settings} style={{ textAlign: 'center', background: 'var(--panel)', border: '1px solid var(--border-strong)', borderRadius: '8px', padding: '8px', fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>
          Edit profile
        </Link>
      </aside>

      {/* Main */}
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: '24px', minWidth: 0 }}>
        {contrib.loading ? (
          <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '22px', boxShadow: 'var(--shadow)' }}><Loading /></div>
        ) : contrib.error ? (
          <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '22px', boxShadow: 'var(--shadow)' }}><ErrorState error={contrib.error} /></div>
        ) : contrib.data ? (
          <ContributionGraph counts={contrib.data.counts} years={contrib.data.years} />
        ) : null}

        <div>
          <h2 style={{ fontSize: '14px', fontWeight: 620, margin: '0 0 12px' }}>Repositories</h2>
          {repos.loading ? (
            <Loading />
          ) : repos.error ? (
            <ErrorState error={repos.error} />
          ) : repos.data && repos.data.length ? (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '14px' }}>
              {repos.data.map((r) => (
                <RepoCard key={r.name} repo={r} />
              ))}
            </div>
          ) : (
            <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', textAlign: 'center', color: 'var(--dim)' }}>No repositories.</div>
          )}
        </div>

        <div>
          <h2 style={{ fontSize: '14px', fontWeight: 620, margin: '0 0 12px' }}>Recent activity</h2>
          {contrib.loading ? <Loading /> : contrib.error ? <ErrorState error={contrib.error} /> : <ActivityFeed items={contrib.data?.activity || []} />}
        </div>
      </div>
    </div>
  );
};
