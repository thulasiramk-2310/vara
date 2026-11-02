import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { useToast } from '../hooks/useToast';
import { routes } from '../routes';
import { Glyph } from '../components/common/Glyphs';
import { Loading, ErrorState } from '../components/common/States';
import { BranchSwitcher, useRepoRef } from '../components/common/BranchSwitcher';
import { usePageTitle } from '../hooks/usePageTitle';
import { highlightLines, languageFor, HighlightedLines } from '../utils/syntax';
import { FileTree } from '../components/common/FileTree';
import { FileCommitBar } from '../components/common/FileCommitBar';
import { useStoredFlag } from '../hooks/useStoredFlag';

const chipBtn: React.CSSProperties = {
  background: 'var(--chip)',
  border: '1px solid var(--chip-border)',
  borderRadius: '6px',
  padding: '4px 9px',
  cursor: 'pointer',
  color: 'var(--dim)',
  fontSize: '12px',
  textDecoration: 'none',
  whiteSpace: 'nowrap',
};

// BlobPage renders a file (RFC-0022 blob). Text is shown as INERT content: every
// line is a React text child, which React escapes — never HTML — the client mirror
// of the server's B5 inert-bytes guarantee. Binary or over-cap files link to the
// raw endpoint (served inert: text/plain or an attachment, always nosniff).
export const BlobPage: React.FC = () => {
  const params = useParams();
  const repo = params.repo || '';
  const path = params['*'] || '';
  const { copy } = useToast();

  const ref = useRepoRef();
  const blob = useAsync('blob', () => api.blob(repo, path, ref), [repo, path, ref]);
  const rawHref = api.rawUrl(repo, path, ref);
  const fname = path.split('/').pop() || path;
  usePageTitle(fname, repo);

  // Two reading preferences, remembered between visits. The file tree starts
  // closed on a phone, where it would push the file below the fold.
  const [wrap, setWrap] = useStoredFlag('vara.wrap', false);
  const narrow = typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 900px)').matches;
  const [treeOpen, setTreeOpen] = useStoredFlag('vara.tree', !narrow);

  const segs = path.split('/').filter(Boolean);
  const dirPath = segs.slice(0, -1);
  const crumb: Array<{ label: string; to?: string }> = [{ label: repo, to: routes.tree(repo, '', ref) }];
  let acc = '';
  dirPath.forEach((s) => {
    acc = acc ? acc + '/' + s : s;
    crumb.push({ label: s, to: routes.tree(repo, acc, ref) });
  });
  crumb.push({ label: fname });

  // Highlighting loads on demand and arrives after the plain text is already on
  // screen, so opening a file is never slower for it.
  const [colored, setColored] = useState<HighlightedLines | null>(null);
  const content = blob.data?.binary || blob.data?.truncated ? undefined : blob.data?.content;
  useEffect(() => {
    let alive = true;
    setColored(null);
    if (content === undefined) return;
    highlightLines(content, languageFor(path)).then((l) => alive && setColored(l));
    return () => {
      alive = false;
    };
  }, [content, path]);

  const lines = (() => {
    const src = String(blob.data?.content ?? '');
    const arr = src.split('\n');
    if (arr.length > 1 && arr[arr.length - 1] === '') arr.pop();
    return arr;
  })();

  return (
    <div className="v-blob" data-tree={treeOpen}>
      {treeOpen && (
        <aside className="v-blob-tree">
          <FileTree repo={repo} reference={ref} active={path} />
        </aside>
      )}
    <div style={{ animation: 'v-fade .2s ease', display: 'flex', flexDirection: 'column', gap: '16px', minWidth: 0 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <button type="button" aria-pressed={treeOpen} onClick={() => setTreeOpen(!treeOpen)} style={chipBtn}>
          Files
        </button>
        <BranchSwitcher repo={repo} to={(r) => routes.blob(repo, path, r)} />
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap', fontFamily: 'var(--font-mono)', fontSize: '13.5px' }}>
        {crumb.map((c, i) => (
          <React.Fragment key={i}>
            {i > 0 && <span style={{ color: 'var(--faint)' }}>/</span>}
            {c.to ? (
              <Link to={c.to} style={{ color: 'var(--accent)', textDecoration: 'none', fontWeight: 600 }}>{c.label}</Link>
            ) : (
              <span style={{ color: 'var(--text)', fontWeight: 600 }}>{c.label}</span>
            )}
          </React.Fragment>
        ))}
      </div>

      {!blob.error && <FileCommitBar repo={repo} path={path} reference={ref} />}

      {blob.loading ? (
        <Loading />
      ) : blob.error ? (
        <ErrorState error={blob.error} onRetry={blob.retry} />
      ) : blob.data ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', overflow: 'hidden', boxShadow: 'var(--shadow)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '9px 14px', borderBottom: '1px solid var(--border)', background: 'var(--inset)', flexWrap: 'wrap' }}>
            <span style={{ color: 'var(--dim)', display: 'flex' }}>{Glyph.file(15)}</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600 }}>{fname}</span>
            {!blob.data.binary && <span style={{ color: 'var(--faint)', fontSize: '12.5px' }}>{lines.length} lines</span>}
            <span style={{ color: 'var(--faint)', fontSize: '12.5px' }}>{blob.data.size} B</span>
            <div style={{ flex: 1 }} />
            {!blob.data.binary && !blob.data.truncated && (
              <button type="button" aria-pressed={wrap} onClick={() => setWrap(!wrap)} style={chipBtn}>
                Wrap lines
              </button>
            )}
            <button onClick={() => copy(path, 'Path')} style={chipBtn}>Copy path</button>
            <a href={rawHref} style={chipBtn}>Raw</a>
            <a href={rawHref} download style={chipBtn}>Download</a>
            <Link to={routes.commits(repo, path, ref)} style={chipBtn}>History</Link>
          </div>

          {blob.data.binary ? (
            <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--dim)', fontSize: '13.5px', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '10px' }}>
              <span style={{ color: 'var(--faint)' }}>{Glyph.binary(28)}</span>
              Binary file ({blob.data.size} bytes) — <a href={rawHref} style={{ color: 'var(--accent)' }}>download</a>.
            </div>
          ) : blob.data.truncated ? (
            <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--dim)', fontSize: '13.5px' }}>
              File too large to display inline ({blob.data.size} bytes) — <a href={rawHref} style={{ color: 'var(--accent)' }}>download raw</a>.
            </div>
          ) : (
            <div style={{ overflowX: 'auto', background: 'var(--inset)' }}>
              <table style={{ borderCollapse: 'collapse', width: '100%', fontFamily: 'var(--font-mono)', fontSize: '12.5px', lineHeight: '20px' }}>
                <tbody>
                  {lines.map((ln, i) => (
                    <tr key={i} id={`L${i + 1}`}>
                      <td style={{ width: '1%', textAlign: 'right', padding: '0 14px 0 16px', color: 'var(--faint)', userSelect: 'none', borderRight: '1px solid var(--border)' }}>
                        <a href={`#L${i + 1}`} style={{ color: 'inherit', textDecoration: 'none' }}>{i + 1}</a>
                      </td>
                      <td style={{ padding: '0 16px', whiteSpace: wrap ? 'pre-wrap' : 'pre', overflowWrap: wrap ? 'anywhere' : 'normal', color: 'var(--text)' }}>
                        {colored && colored[i] && colored[i].length
                          ? colored[i].map((t, k) => (t.cls ? <span key={k} className={t.cls}>{t.text}</span> : t.text))
                          : ln === '' ? ' ' : ln}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ) : null}
    </div>
    </div>
  );
};
