import React, { useEffect, useState } from 'react';
import { useParams, useSearchParams, Link } from 'react-router-dom';
import {
  api,
  short,
  when,
  ApiError,
  ContentSearchResponse,
  PathSearchResponse,
  CommitSearchResponse,
} from '../api/client';
import { segmentText } from '../utils/highlight';
import { routes } from '../routes';
import { Loading, ErrorState } from '../components/common/States';

type Mode = 'content' | 'paths' | 'commits';
const MODES: Mode[] = ['content', 'paths', 'commits'];
const LABELS: Record<Mode, string> = { content: 'Content', paths: 'Files', commits: 'Commits' };

// HL renders text with case-insensitive matches wrapped in <mark>. It emits text
// nodes and <mark> elements — never HTML strings — so content stays inert (S4).
const HL: React.FC<{ text: string; q: string }> = ({ text, q }) => (
  <>
    {segmentText(text, q).map((s, i) =>
      s.isMark ? <mark key={i}>{s.t}</mark> : <React.Fragment key={i}>{s.t}</React.Fragment>,
    )}
  </>
);

const plural = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`;

type Result = ContentSearchResponse | PathSearchResponse | CommitSearchResponse;

// SearchPage is the RFC-0024 search view over the real read API: a query box, a
// Content/Files/Commits toggle, and inert, highlighted results. The query and mode
// live in the URL (?q=&mode=), so a search is shareable and survives a refresh.
export const SearchPage: React.FC = () => {
  const { repo = '' } = useParams();
  const [sp, setSp] = useSearchParams();

  const mode: Mode = MODES.includes(sp.get('mode') as Mode) ? (sp.get('mode') as Mode) : 'content';
  const urlQ = sp.get('q') || '';

  const [input, setInput] = useState(urlQ);
  const [result, setResult] = useState<Result | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiError | undefined>();

  useEffect(() => setInput(urlQ), [urlQ]);

  useEffect(() => {
    if (!urlQ) {
      setResult(undefined);
      setError(undefined);
      setLoading(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError(undefined);
    const req =
      mode === 'content'
        ? api.searchContent(repo, urlQ)
        : mode === 'paths'
        ? api.searchPaths(repo, urlQ)
        : api.searchCommits(repo, urlQ);
    req
      .then((d) => {
        if (alive) {
          setResult(d as Result);
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
  }, [repo, mode, urlQ]);

  const submit = (q: string, m: Mode) => {
    const params: Record<string, string> = { mode: m };
    if (q.trim()) params.q = q.trim();
    setSp(params, { replace: true });
  };

  const truncated = (result as { truncated?: boolean } | undefined)?.truncated;

  return (
    <div style={{ animation: 'v-fade .2s ease', display: 'flex', flexDirection: 'column', gap: '14px', maxWidth: '1020px', margin: '0 auto' }}>
      <form
        onSubmit={(e) => { e.preventDefault(); submit(input, mode); }}
        style={{ display: 'flex', alignItems: 'center', gap: '10px', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', padding: '4px 4px 4px 14px', boxShadow: 'var(--shadow)' }}
      >
        <svg viewBox="0 0 16 16" width="16" height="16" fill="var(--faint)"><path d="M10.68 11.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06ZM11.5 7a4.5 4.5 0 1 0-9 0 4.5 4.5 0 0 0 9 0Z" /></svg>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Search this repository…"
          autoFocus
          style={{ flex: 1, background: 'none', border: 'none', outline: 'none', color: 'var(--text)', fontSize: '14.5px', padding: '8px 0', fontFamily: 'var(--font-mono)' }}
        />
        <button type="submit" style={{ background: 'var(--accent)', color: 'var(--accent-fg)', border: 'none', borderRadius: '7px', padding: '8px 16px', fontSize: '13px', fontWeight: 600, cursor: 'pointer' }}>
          Search
        </button>
      </form>

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '10px' }}>
        <div style={{ display: 'inline-flex', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '9px', padding: '3px', gap: '3px' }}>
          {MODES.map((m) => {
            const active = mode === m;
            return (
              <button
                key={m}
                onClick={() => submit(input, m)}
                style={{ background: active ? 'var(--panel)' : 'transparent', color: active ? 'var(--text)' : 'var(--dim)', border: 'none', borderRadius: '6px', padding: '6px 14px', fontSize: '13px', fontWeight: active ? 600 : 500, cursor: 'pointer', boxShadow: active ? 'var(--shadow)' : 'none' }}
              >
                {LABELS[m]}
              </button>
            );
          })}
        </div>
      </div>

      {truncated && (
        <div style={{ display: 'flex', alignItems: 'center', gap: '9px', background: 'var(--warn-soft)', border: '1px solid color-mix(in srgb, var(--warn) 35%, transparent)', borderRadius: '9px', padding: '9px 13px', color: 'var(--warn)', fontSize: '12.5px' }}>
          <svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor"><path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm9-3a1 1 0 1 0-2 0v3a1 1 0 1 0 2 0Zm-1 5.75A1.25 1.25 0 1 0 8 13.25a1.25 1.25 0 0 0 0-2.5Z" /></svg>
          Showing partial results — refine your query for a complete set.
        </div>
      )}

      {loading ? (
        <Loading label="Searching…" />
      ) : error ? (
        <ErrorState error={error} />
      ) : !urlQ ? (
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '48px 20px', textAlign: 'center', color: 'var(--dim)', fontSize: '13.5px' }}>
          Enter a query to search commits, file names, and content.
        </div>
      ) : result ? (
        <Results repo={repo} mode={mode} q={urlQ} result={result} />
      ) : null}
    </div>
  );
};

const emptyCard = (text: string) => (
  <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '40px 20px', textAlign: 'center', color: 'var(--dim)', fontSize: '13.5px' }}>{text}</div>
);

const countLine = (text: string) => (
  <div style={{ color: 'var(--dim)', fontSize: '13px', fontWeight: 550 }}>{text}</div>
);

const fileHead: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '9px',
  padding: '10px 14px',
  background: 'var(--inset)',
  borderBottom: '1px solid var(--border)',
  textDecoration: 'none',
  color: 'var(--text)',
};

const Results: React.FC<{ repo: string; mode: Mode; q: string; result: Result }> = ({ repo, mode, q, result }) => {
  if (mode === 'commits') {
    const matches = (result as CommitSearchResponse).matches || [];
    if (!matches.length) return emptyCard('No matching commits.');
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
        {countLine(plural(matches.length, 'commit'))}
        {matches.map((c) => (
          <Link key={c.id} to={routes.commit(repo, c.id)} style={{ display: 'grid', gap: '6px', textDecoration: 'none', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', padding: '12px 15px', color: 'var(--text)' }}>
            <div style={{ fontWeight: 560, fontSize: '14px' }}><HL text={c.message.split('\n')[0]} q={q} /></div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '9px', color: 'var(--faint)', fontSize: '12.5px', flexWrap: 'wrap' }}>
              <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--dim)' }}>{short(c.id)}</span>
              <span style={{ color: 'var(--dim)' }}><HL text={c.author} q={q} /></span>
              <span>{when(c.timestamp)}</span>
            </div>
          </Link>
        ))}
      </div>
    );
  }

  if (mode === 'paths') {
    const matches = (result as PathSearchResponse).matches || [];
    if (!matches.length) return emptyCard('No matching files.');
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
        {countLine(plural(matches.length, 'file'))}
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', overflow: 'hidden' }}>
          {matches.map((m, i) => (
            <Link
              key={m.path}
              to={m.is_dir ? routes.tree(repo, m.path) : routes.blob(repo, m.path)}
              style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 16px', textDecoration: 'none', color: 'var(--text)', borderTop: i > 0 ? '1px solid var(--border)' : 'none' }}
            >
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}><HL text={m.path} q={q} /></span>
              <span style={{ flex: 1 }} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--faint)' }}>{m.mode}</span>
            </Link>
          ))}
        </div>
      </div>
    );
  }

  // content
  const matches = (result as ContentSearchResponse).matches || [];
  if (!matches.length) return emptyCard('No matching lines.');
  const total = matches.reduce((acc, m) => acc + (m.lines?.length || 0), 0);
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {countLine(`${plural(total, 'line')} in ${plural(matches.length, 'file')}`)}
      {matches.map((m) => (
        <div key={m.path} style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', overflow: 'hidden' }}>
          <Link to={routes.blob(repo, m.path)} style={fileHead}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>{m.path}</span>
            <span style={{ flex: 1 }} />
            <span style={{ color: 'var(--faint)', fontSize: '11.5px' }}>{plural(m.lines?.length || 0, 'line')}</span>
          </Link>
          {(m.lines || []).map((l, li) => (
            <div key={li} style={{ display: 'grid', gridTemplateColumns: '52px 1fr', borderTop: '1px solid var(--border)', fontFamily: 'var(--font-mono)', fontSize: '12.5px', lineHeight: '20px' }}>
              <span style={{ textAlign: 'right', padding: '3px 12px 3px 0', color: 'var(--faint)', userSelect: 'none', background: 'var(--inset)' }}>{l.line}</span>
              <span style={{ padding: '3px 14px', whiteSpace: 'pre', overflowX: 'auto', color: 'var(--text)' }}><HL text={l.content} q={q} /></span>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
};
