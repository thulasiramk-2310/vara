import React, { useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { SkelRows, ROW_HEIGHT } from './Skeleton';

// A load that finishes quickly should show nothing at all: a skeleton that
// flashes for 80 ms is worse than none. The placeholder appears only if the wait
// passes this threshold, and its space is reserved from the start.
const SHOW_AFTER_MS = 200;

function useDelayed(ms: number): boolean {
  const [on, setOn] = useState(false);
  useEffect(() => {
    const t = window.setTimeout(() => setOn(true), ms);
    return () => window.clearTimeout(t);
  }, [ms]);
  return on;
}

// Loading reserves room for `rows` rows, announces itself to screen readers at
// once, and paints skeleton rows only after a short delay.
export const Loading: React.FC<{ label?: string; rows?: number }> = ({ label = 'Loading', rows = 5 }) => {
  const visible = useDelayed(SHOW_AFTER_MS);
  return (
    <div role="status" aria-busy="true" style={{ minHeight: rows * ROW_HEIGHT }}>
      <span className="v-sr-only">{label}</span>
      {visible && <SkelRows rows={rows} />}
    </div>
  );
};

// Empty is a neutral "nothing here" panel. Say what is missing and, where there
// is one, offer the next step as `children`.
export const Empty: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div
    style={{
      background: 'var(--panel)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--s-3)',
      padding: 'var(--s-7) var(--s-5)',
      textAlign: 'center',
      color: 'var(--dim)',
      fontSize: '14px',
    }}
  >
    {children}
  </div>
);

// describe turns a failure into plain words for the reader. 401 is a sign-in
// prompt, 403 a permission message, a network failure is its own case, and 5xx
// is the server's fault, not the reader's.
function describe(error: ApiError): { title: string; detail: string; kind: 'auth' | 'retry' | 'none' } {
  if (error.status === 401) {
    return { title: 'Sign in required', detail: 'This repository requires you to sign in.', kind: 'auth' };
  }
  if (error.status === 403) {
    return {
      title: 'Not permitted',
      detail: `Your account cannot read this (${error.code || 'forbidden'}). If you expected access, ask the server's administrator.`,
      kind: 'none',
    };
  }
  if (error.status === 404) {
    return { title: 'Not found', detail: error.message || 'This does not exist, or was removed.', kind: 'none' };
  }
  if (error.code === 'UNAVAILABLE') {
    return { title: 'Not enabled', detail: error.message, kind: 'none' };
  }
  if (error.code === 'BAD_RESPONSE') {
    return { title: 'Unexpected response', detail: error.message, kind: 'retry' };
  }
  if (error.code === 'NETWORK') {
    return { title: 'Cannot reach the server', detail: 'Check your connection, then try again.', kind: 'retry' };
  }
  if (error.status >= 500) {
    return { title: 'The server hit an error', detail: error.message || 'Try again in a moment.', kind: 'retry' };
  }
  return { title: 'Something went wrong', detail: error.message, kind: 'retry' };
}

// ErrorState renders a failed request. It always says what happened and offers
// the one action that can help: sign in, or try again.
export const ErrorState: React.FC<{ error: ApiError; onRetry?: () => void }> = ({ error, onRetry }) => {
  const location = useLocation();
  const { title, detail, kind } = describe(error);
  const next = encodeURIComponent(location.pathname + location.search);
  return (
    <div
      role="alert"
      style={{
        background: 'var(--panel)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--s-3)',
        padding: 'var(--s-6) var(--s-5)',
        color: 'var(--text)',
      }}
    >
      <div style={{ fontSize: '16px', fontWeight: 650, marginBottom: 'var(--s-1)' }}>{title}</div>
      <div style={{ color: 'var(--dim)', fontSize: '14px', maxWidth: '560px' }}>{detail}</div>
      {(error.status > 0 || error.code || error.requestId) && (
        <div style={{ color: 'var(--faint)', fontFamily: 'var(--font-mono)', fontSize: '12px', marginTop: 'var(--s-2)' }}>
          {[error.status > 0 ? `HTTP ${error.status}` : '', error.code || '', error.requestId ? `request ${error.requestId}` : ''].filter(Boolean).join(' · ')}
        </div>
      )}
      {kind === 'auth' && (
        <Link to={`/login?next=${next}`} className="v-btn v-btn-primary" style={{ marginTop: 'var(--s-4)' }}>
          Sign in
        </Link>
      )}
      {kind === 'retry' && (
        <button
          type="button"
          className="v-btn"
          style={{ marginTop: 'var(--s-4)' }}
          onClick={onRetry ?? (() => window.location.reload())}
        >
          Try again
        </button>
      )}
    </div>
  );
};
