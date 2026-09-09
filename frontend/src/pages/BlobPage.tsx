import React from 'react';
import { useParams, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { useToast } from '../hooks/useToast';
import { routes } from '../routes';
import { Glyph } from '../components/common/Glyphs';
import { Loading, ErrorState } from '../components/common/States';

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

  const blob = useAsync(() => api.blob(repo, path), [repo, path]);
  const rawHref = api.rawUrl(repo, path);
  const fname = path.split('/').pop() || path;

  const segs = path.split('/').filter(Boolean);
  const dirPath = segs.slice(0, -1);
  const crumb: Array<{ label: string; to?: string }> = [{ label: repo, to: routes.tree(repo, '') }];
  let acc = '';
  dirPath.forEach((s) => {
    acc = acc ? acc + '/' + s : s;
    crumb.push({ label: s, to: routes.tree(repo, acc) });
  });
  crumb.push({ label: fname });

  const lines = (() => {
    const src = String(blob.data?.content ?? '');
    const arr = src.split('\n');
    if (arr.length > 1 && arr[arr.length - 1] === '') arr.pop();
    return arr;
  })();

  return (
    <div style={{ animation: 'v-fade .2s ease', display: 'flex', flexDirection: 'column', gap: '16px', maxWidth: '1020px', margin: '0 auto' }}>
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

      {blob.loading ? (
        <Loading />
      ) : blob.error ? (
        <ErrorState error={blob.error} />
      ) : blob.data ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', overflow: 'hidden', boxShadow: 'var(--shadow)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '9px 14px', borderBottom: '1px solid var(--border)', background: 'var(--inset)', flexWrap: 'wrap' }}>
            <span style={{ color: 'var(--dim)', display: 'flex' }}>{Glyph.file(15)}</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600 }}>{fname}</span>
            {!blob.data.binary && <span style={{ color: 'var(--faint)', fontSize: '12.5px' }}>{lines.length} lines</span>}
            <span style={{ color: 'var(--faint)', fontSize: '12.5px' }}>{blob.data.size} B</span>
            <div style={{ flex: 1 }} />
            <button onClick={() => copy(path, 'Path')} style={chipBtn}>Copy path</button>
            <a href={rawHref} style={chipBtn}>Raw</a>
            <a href={rawHref} download style={chipBtn}>Download</a>
            <Link to={routes.commits(repo)} style={chipBtn}>History</Link>
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
                      <td style={{ padding: '0 16px', whiteSpace: 'pre', color: 'var(--text)' }}>{ln === '' ? ' ' : ln}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
};
