import React from 'react';
import { useParams, Link } from 'react-router-dom';
import { Folder, File as FileIcon, GitBranch, GitCommit, Hash, Copy, Clock, BookText } from 'lucide-react';
import { api, short, when, TreeEntry } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { useToast } from '../hooks/useToast';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';
import { Markdown } from '../components/common/Markdown';
import { RepoGlyph } from '../components/common/RepoGlyph';
import { inferLanguage } from '../utils/repo';

function sortEntries(entries: TreeEntry[]): TreeEntry[] {
  return entries.slice().sort((a, b) => {
    const ad = a.type === 'dir';
    const bd = b.type === 'dir';
    if (ad !== bd) return ad ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
}

const README_RE = /^readme(\.(md|markdown|mdown|txt))?$/i;

// CodePage is the repository overview and directory browser (RFC-0022). Two-column
// layout: the file tree + rendered README on the left, an "About" sidebar (language,
// branch, commits, clone) on the right — all from the real read API, no mock data.
export const CodePage: React.FC = () => {
  const params = useParams();
  const repo = params.repo || '';
  const path = params['*'] || '';
  const { copy } = useToast();

  const sum = useAsync(() => api.summary(repo), [repo]);
  const tree = useAsync(() => api.tree(repo, path), [repo, path]);

  const entries = tree.data ? sortEntries(tree.data.entries) : [];
  const language = tree.data ? inferLanguage(tree.data.entries.filter((e) => e.type !== 'dir').map((e) => e.name)) : null;

  // Root README, rendered as a preview below the file list.
  const readmeName = path === '' ? tree.data?.entries.find((e) => e.type !== 'dir' && README_RE.test(e.name))?.name : undefined;
  const isMd = !!readmeName && /\.(md|markdown|mdown)$/i.test(readmeName);
  const readme = useAsync(async () => {
    if (!readmeName) return null;
    const b = await api.blob(repo, readmeName);
    return b.binary ? null : b;
  }, [repo, path, readmeName]);

  const cloneCmd = `vara clone ${window.location.origin}/${repo}`;

  const segs = path.split('/').filter(Boolean);
  const crumb = [{ label: repo, to: routes.tree(repo, '') }];
  let acc = '';
  segs.forEach((s) => {
    acc = acc ? acc + '/' + s : s;
    crumb.push({ label: s, to: routes.tree(repo, acc) });
  });

  const cardStyle: React.CSSProperties = { background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '14px', boxShadow: 'var(--shadow)' };

  return (
    <div style={{ animation: 'v-fade .2s ease', display: 'grid', gap: '18px', minWidth: 0 }}>
      {/* Compact header: glyph + name, then a single meta line, id secondary */}
      <div style={{ display: 'grid', gap: '7px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
          <RepoGlyph seed={sum.data?.id || repo} size={24} title={repo} />
          <h1 style={{ margin: 0, fontSize: '24px', fontWeight: 700, letterSpacing: '-0.5px', fontFamily: 'var(--font-mono)' }}>{repo}</h1>
          <span style={{ fontSize: '10px', letterSpacing: '0.4px', textTransform: 'uppercase', color: 'var(--dim)', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '20px', padding: '2px 9px', fontWeight: 600 }}>read-only</span>
        </div>
        {sum.data ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: '9px', flexWrap: 'wrap', fontSize: '12.5px', color: 'var(--dim)' }}>
            {language ? (
              <>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
                  <span style={{ width: '9px', height: '9px', borderRadius: '50%', background: language.color, boxShadow: '0 0 0 1px rgba(0,0,0,0.08)' }} />
                  {language.name}
                </span>
                <span style={{ color: 'var(--faint)' }}>·</span>
              </>
            ) : null}
            <span>{sum.data.commit_count} commit{sum.data.commit_count === 1 ? '' : 's'}</span>
            <span style={{ color: 'var(--faint)' }}>·</span>
            <span>{sum.data.branch_count} branch{sum.data.branch_count === 1 ? '' : 'es'}</span>
            {sum.data.last_commit ? (
              <>
                <span style={{ color: 'var(--faint)' }}>·</span>
                <span>Updated {when(sum.data.last_commit.timestamp)}</span>
              </>
            ) : null}
          </div>
        ) : null}
        {sum.data ? (
          <div title={sum.data.id} style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--faint)', wordBreak: 'break-all' }}>{sum.data.id}</div>
        ) : null}
      </div>

      <div className="v-dash">
        {/* Main column: files + README */}
        <div style={{ display: 'grid', gap: '16px', minWidth: 0, alignContent: 'start' }}>
          {/* Breadcrumb */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap', fontFamily: 'var(--font-mono)', fontSize: '13.5px' }}>
            {crumb.map((c, i) => (
              <React.Fragment key={c.to}>
                {i > 0 && <span style={{ color: 'var(--faint)' }}>/</span>}
                {i === crumb.length - 1 ? (
                  <span style={{ color: 'var(--text)', fontWeight: 600 }}>{c.label}</span>
                ) : (
                  <Link to={c.to} style={{ color: 'var(--accent)', textDecoration: 'none', fontWeight: 600 }}>{c.label}</Link>
                )}
              </React.Fragment>
            ))}
          </div>

          {/* File tree */}
          {tree.loading ? (
            <Loading />
          ) : tree.error ? (
            <ErrorState error={tree.error} />
          ) : (
            <div style={{ ...cardStyle, overflow: 'hidden' }}>
              {entries.length ? (
                entries.map((e, i) => {
                  const child = path ? `${path}/${e.name}` : e.name;
                  const isDir = e.type === 'dir';
                  const to = isDir ? routes.tree(repo, child) : routes.blob(repo, child);
                  return (
                    <Link key={e.name} to={to} style={{ display: 'flex', alignItems: 'center', gap: '11px', padding: '10px 16px', textDecoration: 'none', color: 'var(--text)', borderTop: i > 0 ? '1px solid var(--border)' : 'none' }}>
                      {isDir ? <Folder size={16} style={{ color: 'var(--accent)', flexShrink: 0 }} /> : <FileIcon size={16} style={{ color: 'var(--dim)', flexShrink: 0 }} />}
                      <span style={{ fontWeight: 520, fontSize: '13.5px' }}>{e.name}</span>
                      <span style={{ flex: 1 }} />
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--faint)' }}>{e.mode}</span>
                    </Link>
                  );
                })
              ) : (
                <div style={{ padding: '32px', textAlign: 'center', color: 'var(--dim)', fontSize: '13.5px' }}>Empty directory.</div>
              )}
            </div>
          )}

          {/* README preview */}
          {readmeName && readme.data && readme.data.content ? (
            <div style={cardStyle}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '12px 18px', borderBottom: '1px solid var(--border)', fontSize: '12.5px', fontWeight: 600, color: 'var(--dim)' }}>
                <BookText size={15} /> {readmeName}
                {readme.data.truncated ? <span style={{ color: 'var(--faint)', fontWeight: 400 }}>· truncated</span> : null}
              </div>
              <div style={{ padding: '22px 24px' }}>
                {isMd ? (
                  <Markdown source={readme.data.content} />
                ) : (
                  <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'var(--font-mono)', fontSize: '13px', lineHeight: 1.6, color: 'var(--text)' }}>{readme.data.content}</pre>
                )}
              </div>
            </div>
          ) : null}
        </div>

        {/* About sidebar */}
        <aside style={{ display: 'grid', gap: '14px', minWidth: 0, alignContent: 'start', position: 'sticky', top: '80px' }}>
          <div style={{ ...cardStyle, padding: '16px 18px', display: 'grid', gap: '16px' }}>
            {/* Repository */}
            <div style={{ display: 'grid', gap: '11px' }}>
              <SectionLabel>Repository</SectionLabel>
              {language ? (
                <MetaRow icon={<span style={{ width: '11px', height: '11px', borderRadius: '50%', background: language.color, display: 'inline-block', boxShadow: '0 0 0 1px rgba(0,0,0,0.08)' }} />} label="Language" value={language.name} />
              ) : null}
              <MetaRow icon={<GitBranch size={14} />} label="Default branch" value={sum.data?.default_branch || '—'} mono />
              <MetaRow icon={<GitCommit size={14} />} label="Commits" value={sum.data ? String(sum.data.commit_count) : '—'} />
              <MetaRow icon={<GitBranch size={14} />} label="Branches" value={sum.data ? String(sum.data.branch_count) : '—'} />
              <MetaRow icon={<Hash size={14} />} label="HEAD" value={short(sum.data?.head) || '—'} mono />
            </div>

            {/* Latest activity */}
            {sum.data?.last_commit ? (
              <div style={{ display: 'grid', gap: '8px', borderTop: '1px solid var(--border)', paddingTop: '16px' }}>
                <SectionLabel>Latest activity</SectionLabel>
                <Link to={routes.commit(repo, sum.data.last_commit.id)} style={{ display: 'grid', gap: '4px', textDecoration: 'none', color: 'var(--text)' }}>
                  <span style={{ fontSize: '13px', fontWeight: 550, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{sum.data.last_commit.message.split('\n')[0]}</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '7px', fontSize: '12px', color: 'var(--faint)' }}>
                    <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>{short(sum.data.last_commit.id)}</span>
                    <Clock size={11} /> {when(sum.data.last_commit.timestamp)}
                  </span>
                </Link>
              </div>
            ) : null}
          </div>

          {/* Clone */}
          <div style={{ ...cardStyle, padding: '14px 16px', display: 'grid', gap: '8px' }}>
            <span style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 600 }}>Clone</span>
            <div style={{ display: 'flex', alignItems: 'center', border: '1px solid var(--border)', borderRadius: '9px', overflow: 'hidden', background: 'var(--inset)' }}>
              <code style={{ padding: '8px 12px', fontSize: '12px', color: 'var(--dim)', fontFamily: 'var(--font-mono)', overflowX: 'auto', whiteSpace: 'nowrap', flex: 1 }}>{cloneCmd}</code>
              <button onClick={() => copy(cloneCmd, 'Clone command')} title="Copy" style={{ border: 'none', borderLeft: '1px solid var(--border)', background: 'var(--panel)', color: 'var(--dim)', padding: '0 12px', alignSelf: 'stretch', cursor: 'pointer', display: 'flex', alignItems: 'center' }}>
                <Copy size={14} />
              </button>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
};

const SectionLabel: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 700 }}>{children}</span>
);

const MetaRow: React.FC<{ icon: React.ReactNode; label: string; value: string; mono?: boolean }> = ({ icon, label, value, mono }) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: '9px', fontSize: '13px' }}>
    <span style={{ color: 'var(--faint)', display: 'flex' }}>{icon}</span>
    <span style={{ color: 'var(--dim)' }}>{label}</span>
    <span style={{ flex: 1 }} />
    <span style={{ fontWeight: 600, fontFamily: mono ? 'var(--font-mono)' : 'inherit' }}>{value}</span>
  </div>
);
