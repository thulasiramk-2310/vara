import React, { useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { usePageTitle } from '../hooks/usePageTitle';

// NotFoundPage answers any address the app has no page for. (The server cannot
// say 404 for these: it serves the same entry page for every app route, because
// the router decides what exists. The page asks search engines to skip it.)
export const NotFoundPage: React.FC = () => {
  const { pathname } = useLocation();
  usePageTitle('Page not found');

  useEffect(() => {
    const meta = document.createElement('meta');
    meta.name = 'robots';
    meta.content = 'noindex';
    document.head.appendChild(meta);
    return () => {
      document.head.removeChild(meta);
    };
  }, []);

  return (
    <div style={{ maxWidth: '560px', margin: '0 auto', padding: 'var(--s-8) var(--s-4)' }}>
      <p style={{ fontFamily: 'var(--font-mono)', color: 'var(--dim)', margin: '0 0 var(--s-3)', fontSize: '13px' }}>404</p>
      <h1 style={{ fontSize: '32px', fontWeight: 700, letterSpacing: '-.02em', margin: '0 0 var(--s-3)' }}>
        There is nothing at this address.
      </h1>
      <p style={{ color: 'var(--dim)', margin: '0 0 var(--s-5)', overflowWrap: 'anywhere' }}>
        <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{pathname}</span> is not a page in this Hub.
        The link may be old, or mistyped.
      </p>
      <div style={{ display: 'flex', gap: 'var(--s-3)', flexWrap: 'wrap' }}>
        <Link to="/dashboard" className="v-btn v-btn-primary">
          Your repositories
        </Link>
        <Link to="/" className="v-btn">
          Home
        </Link>
      </div>
    </div>
  );
};
