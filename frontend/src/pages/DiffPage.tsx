import React, { useState } from 'react';
import { useParams } from 'react-router-dom';
import { api, short, when, FileDiffResponse, DiffFileInfo, ApiError } from '../api/client';
import { useAsync } from '../hooks/useAsync';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';

function statusMeta(s: string): { label: string; bg: string; fg: string } {
  const k = (s || '').toLowerCase();
  if (k.startsWith('a')) return { label: 'A', bg: 'var(--add-bg)', fg: 'var(--add)' };
  if (k.startsWith('d')) return { label: 'D', bg: 'var(--del-bg)', fg: 'var(--del)' };
  if (k.startsWith('r')) return { label: 'R', bg: 'var(--accent-soft)', fg: 'var(--accent)' };
  return { label: 'M', bg: 'var(--warn-soft)', fg: 'var(--warn)' };
}

// FileDiffView renders one file's unified diff. Every line is a React text child
// (escaped, inert) — the client mirror of the RFC-0023 D4 inert-diff guarantee.
const FileDiffView: React.FC<{ fd: FileDiffResponse }> = ({ fd }) => {
  if (fd.binary) return <div style={{ padding: '20px', color: 'var(--dim)', fontSize: '13px' }}>Binary file — no textual diff.</div>;
  if (fd.truncated) return <div style={{ padding: '20px', color: 'var(--dim)', fontSize: '13px' }}>Diff too large to display inline.</div>;
  if (!fd.hunks || !fd.hunks.length) return <div style={{ padding: '20px', color: 'var(--dim)', fontSize: '13px' }}>No changes.</div>;

  return (
    <div style={{ overflowX: 'auto', background: 'var(--inset)' }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontFamily: 'var(--font-mono)', fontSize: '12.5px', lineHeight: '20px' }}>
        <tbody>
          {fd.hunks.map((hk, hi) => {
            let oldNo = hk.old_start;
            let newNo = hk.new_start;
            const rows: React.ReactNode[] = [
              <tr key={`h-${hi}`} style={{ background: 'var(--accent-soft)' }}>
                <td colSpan={3} style={{ padding: '2px 12px', color: 'var(--accent)', whiteSpace: 'pre' }}>{hk.header}</td>
              </tr>,
            ];
            hk.lines.forEach((ln, li) => {
              let o: number | string = '';
              let n: number | string = '';
              let sign = ' ';
              let bg = 'transparent';
              let gutter: React.CSSProperties = {};
              if (ln.type === 'add') {
                n = newNo++;
                sign = '+';
                bg = 'var(--add-bg)';
                gutter = { background: 'var(--add-gutter)' };
              } else if (ln.type === 'del') {
                o = oldNo++;
                sign = '−';
                bg = 'var(--del-bg)';
                gutter = { background: 'var(--del-gutter)' };
              } else {
                o = oldNo++;
                n = newNo++;
              }
              rows.push(
                <tr key={`${hi}-${li}`} style={{ background: bg }}>
                  <td style={{ width: '1%', textAlign: 'right', padding: '0 8px 0 12px', color: 'var(--faint)', userSelect: 'none', ...gutter }}>{o}</td>
                  <td style={{ width: '1%', textAlign: 'right', padding: '0 10px', color: 'var(--faint)', userSelect: 'none', ...gutter }}>{n}</td>
                  <td style={{ padding: '0 14px', whiteSpace: 'pre', color: 'var(--text)' }}>
                    <span style={{ userSelect: 'none', color: 'var(--faint)', marginRight: '8px' }}>{sign}</span>
                    {ln.content === '' ? ' ' : ln.content}
                  </td>
                </tr>,
              );
            });
            return rows;
          })}
        </tbody>
      </table>
    </div>
  );
};

const FileRow: React.FC<{ repo: string; id: string; base: string; file: DiffFileInfo }> = ({ repo, id, base, file }) => {
  const [open, setOpen] = useState(false);
  const [fd, setFd] = useState<FileDiffResponse | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);
  const meta = statusMeta(file.status);

  const toggle = async () => {
    const nextOpen = !open;
    setOpen(nextOpen);
    if (nextOpen && !fd && !err) {
      setLoading(true);
      try {
        setFd(await api.fileDiff(repo, file.path, id, base));
      } catch (e) {
        setErr(e as ApiError);
      } finally {
        setLoading(false);
      }
    }
  };

  return (
    <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', overflow: 'hidden', boxShadow: 'var(--shadow)' }}>
      <button
        onClick={toggle}
        style={{ width: '100%', textAlign: 'left', display: 'flex', alignItems: 'center', gap: '10px', padding: '11px 14px', background: 'var(--inset)', border: 'none', borderBottom: open ? '1px solid var(--border)' : 'none', cursor: 'pointer', color: 'var(--text)' }}
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="var(--faint)" style={{ transform: open ? 'rotate(90deg)' : 'none', transition: 'transform .12s' }}>
          <path d="M6 4l4 4-4 4" />
        </svg>
        <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: '17px', height: '17px', borderRadius: '5px', background: meta.bg, color: meta.fg, fontSize: '11px', fontWeight: 700 }}>{meta.label}</span>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{file.path}</span>
      </button>
      {open && (loading ? <div style={{ padding: '16px' }}><Loading label="Loading diff…" /></div> : err ? <div style={{ padding: '16px' }}><ErrorState error={err} /></div> : fd ? <FileDiffView fd={fd} /> : null)}
    </div>
  );
};

// DiffPage shows what a commit did (RFC-0023): header, changed-file summary, and
// each file's unified diff, lazily loaded on expand — all from the real read API.
export const DiffPage: React.FC = () => {
  const { repo = '', id = '' } = useParams();
  const detail = useAsync(() => api.commit(repo, id), [repo, id]);
  const summary = useAsync(() => api.commitDiff(repo, id), [repo, id]);

  if (detail.loading || summary.loading) return <div style={{ animation: 'v-fade .2s ease' }}><Loading /></div>;
  if (detail.error) return <ErrorState error={detail.error} />;
  if (summary.error) return <ErrorState error={summary.error} />;

  const files = summary.data?.files || [];
  const base = summary.data?.base_commit || '';

  return (
    <div style={{ animation: 'v-fade .2s ease', display: 'flex', flexDirection: 'column', gap: '16px', maxWidth: '1020px', margin: '0 auto' }}>
      <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '16px 18px', boxShadow: 'var(--shadow)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap', marginBottom: '8px' }}>
          <span style={{ fontWeight: 600, fontSize: '15px' }}>{detail.data?.message.split('\n')[0]}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap', color: 'var(--dim)', fontSize: '12.5px' }}>
          <span style={{ fontFamily: 'var(--font-mono)', background: 'var(--chip)', border: '1px solid var(--chip-border)', borderRadius: '6px', padding: '2px 8px' }}>{short(detail.data?.id)}</span>
          <span>{detail.data?.author}</span>
          <span>{when(detail.data?.timestamp)}</span>
          <span style={{ flex: 1 }} />
          <a href={routes.commits(repo)} style={{ color: 'var(--accent)', textDecoration: 'none' }}>All commits</a>
        </div>
      </div>

      <h2 style={{ fontSize: '14px', fontWeight: 640, margin: 0 }}>
        {files.length} changed file{files.length === 1 ? '' : 's'}
      </h2>

      {files.length === 0 ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '32px', textAlign: 'center', color: 'var(--dim)' }}>No changes.</div>
      ) : (
        files.map((f) => <FileRow key={f.path} repo={repo} id={id} base={base} file={f} />)
      )}
    </div>
  );
};
