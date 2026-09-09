import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { Sparkles, Bug, BookText, Recycle, FlaskConical, Wrench, Palette, Zap, Package, Cog, Undo2, GitMerge, GitCommit, Copy } from 'lucide-react';
import { api, short, when, CommitRef, ApiError } from '../api/client';
import { useToast } from '../hooks/useToast';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';
import { Identicon } from '../components/common/Identicon';

// Conventional-commit type → a Lucide icon + label. Colours stay neutral (the icon
// conveys the type); the author identicon carries the per-person colour.
type IconType = React.ComponentType<{ size?: number }>;
const TYPES: Record<string, { label: string; Icon: IconType }> = {
  feat: { label: 'feat', Icon: Sparkles },
  fix: { label: 'fix', Icon: Bug },
  docs: { label: 'docs', Icon: BookText },
  refactor: { label: 'refactor', Icon: Recycle },
  test: { label: 'test', Icon: FlaskConical },
  chore: { label: 'chore', Icon: Wrench },
  style: { label: 'style', Icon: Palette },
  perf: { label: 'perf', Icon: Zap },
  build: { label: 'build', Icon: Package },
  ci: { label: 'ci', Icon: Cog },
  revert: { label: 'revert', Icon: Undo2 },
  merge: { label: 'merge', Icon: GitMerge },
};

// classify infers a commit's type from its subject line and returns the type meta
// plus the subject with a recognized "type(scope): " prefix stripped for cleaner
// display. Unknown → a generic commit.
function classify(message: string): { label: string; Icon: IconType; subject: string } {
  const subject = message.split('\n')[0];
  if (/^merge\b/i.test(subject)) return { ...TYPES.merge, subject };
  const m = /^(\w+)(\([^)]*\))?!?:\s*(.*)$/.exec(subject);
  if (m && TYPES[m[1].toLowerCase()]) {
    const t = TYPES[m[1].toLowerCase()];
    return { label: t.label, Icon: t.Icon, subject: m[3] || subject };
  }
  return { label: '', Icon: GitCommit, subject };
}

// authorName strips a "Name <email>" author string to just the name.
function authorName(a: string): string {
  return (a || '').replace(/\s*<[^>]*>\s*/, '').trim() || a;
}

function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  return d.toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' });
}

export const CommitsPage: React.FC = () => {
  const { repo = '' } = useParams();
  const { copy, showToast } = useToast();

  const [items, setItems] = useState<CommitRef[]>([]);
  const [next, setNext] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | undefined>();
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    let alive = true;
    setItems([]);
    setNext('');
    setLoading(true);
    setError(undefined);
    api
      .commits(repo, 30, '')
      .then((d) => {
        if (alive) {
          setItems(d.commits || []);
          setNext(d.next || '');
          setLoading(false);
        }
      })
      .catch((e: ApiError) => {
        if (alive) {
          setError(e);
          setLoading(false);
        }
      });
    return () => {
      alive = false;
    };
  }, [repo]);

  const loadMore = async () => {
    setLoadingMore(true);
    try {
      const d = await api.commits(repo, 30, next);
      setItems((p) => [...p, ...(d.commits || [])]);
      setNext(d.next || '');
    } catch (e) {
      showToast((e as ApiError).message);
    } finally {
      setLoadingMore(false);
    }
  };

  if (loading) return <div style={{ maxWidth: '1020px', margin: '0 auto' }}><Loading /></div>;
  if (error) return <div style={{ maxWidth: '1020px', margin: '0 auto' }}><ErrorState error={error} /></div>;

  return (
    <div style={{ animation: 'v-fade .2s ease', maxWidth: '1020px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '20px', fontWeight: 680, margin: '0 0 20px', letterSpacing: '-0.3px' }}>
        Commits <span style={{ color: 'var(--faint)', fontWeight: 500, fontSize: '15px' }}>· {items.length}{next ? '+' : ''}</span>
      </h1>

      {items.length === 0 ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '14px', padding: '40px', textAlign: 'center', color: 'var(--dim)' }}>No commits.</div>
      ) : (
        items.map((c, idx) => {
          const t = classify(c.message);
          const day = dayLabel(c.timestamp);
          const prevDay = idx > 0 ? dayLabel(items[idx - 1].timestamp) : '';
          const newDay = day && day !== prevDay;
          const last = idx === items.length - 1;
          return (
            <React.Fragment key={c.id}>
              {newDay && (
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px', margin: idx === 0 ? '0 0 12px' : '10px 0 12px', paddingLeft: '3px' }}>
                  <GitCommit size={14} style={{ color: 'var(--faint)' }} />
                  <span style={{ fontSize: '12.5px', fontWeight: 600, color: 'var(--dim)' }}>Commits on {day}</span>
                </div>
              )}
              <div style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '14px' }}>
                {/* Timeline rail */}
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', paddingTop: '18px' }}>
                  <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: 'var(--panel)', border: '2px solid var(--accent)', flexShrink: 0 }} />
                  {!last && <span style={{ flex: 1, width: '2px', background: 'var(--border)', margin: '3px 0' }} />}
                </div>
                {/* Commit card */}
                <div style={{ paddingBottom: '12px', minWidth: 0 }}>
                  <Link
                    to={routes.commit(repo, c.id)}
                    style={{ display: 'flex', alignItems: 'center', gap: '12px', textDecoration: 'none', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '12px 15px', color: 'var(--text)' }}
                  >
                    <Identicon seed={c.author} size={26} round />
                    <div style={{ minWidth: 0, flex: 1, display: 'grid', gap: '3px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '9px', minWidth: 0 }}>
                        {t.label ? (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '10.5px', fontWeight: 600, color: 'var(--dim)', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '6px', padding: '2px 7px', flexShrink: 0, textTransform: 'lowercase' }}>
                            <t.Icon size={11} /> {t.label}
                          </span>
                        ) : (
                          <t.Icon size={13} />
                        )}
                        <span style={{ fontWeight: 560, fontSize: '13.5px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.subject}</span>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--faint)', fontSize: '12px' }}>
                        <span style={{ color: 'var(--dim)', fontWeight: 500 }}>{authorName(c.author)}</span>
                        <span>·</span>
                        <span>{when(c.timestamp)}</span>
                      </div>
                    </div>
                    <span
                      onClick={(ev) => { ev.preventDefault(); copy(c.id, short(c.id)); }}
                      title="Copy commit id"
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '7px', padding: '4px 9px', color: 'var(--dim)', fontFamily: 'var(--font-mono)', fontSize: '12px', flexShrink: 0 }}
                    >
                      {short(c.id)} <Copy size={12} />
                    </span>
                  </Link>
                </div>
              </div>
            </React.Fragment>
          );
        })
      )}

      {next && (
        <div style={{ display: 'flex', justifyContent: 'center', marginTop: '4px' }}>
          <button onClick={loadMore} disabled={loadingMore} style={{ background: 'var(--panel)', border: '1px solid var(--border-strong)', borderRadius: '9px', padding: '9px 20px', fontSize: '13px', fontWeight: 600, cursor: loadingMore ? 'default' : 'pointer', color: 'var(--text)' }}>
            {loadingMore ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
    </div>
  );
};
