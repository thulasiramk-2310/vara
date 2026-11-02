import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../hooks/useAuth';
import { PageSkeleton } from './Skeleton';

// RequireAuth keeps signed-out visitors off pages that only make sense for an
// account (creating a repository, settings). It waits for the identity check
// before deciding, so a signed-in user is never bounced during page load, and it
// sends the visitor to sign in with the page they wanted as `next`.
export const RequireAuth: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { authed, loading } = useAuth();
  const location = useLocation();
  if (loading) return <PageSkeleton />;
  if (!authed) {
    const next = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?next=${next}`} replace />;
  }
  return <>{children}</>;
};

// safeNext accepts a `next` address only when it is a path inside this app. A
// full URL, or a "//host" address, would let a crafted link send a user to
// another site right after they sign in.
export function safeNext(raw: string | null, fallback = '/dashboard'): string {
  if (!raw) return fallback;
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return fallback;
  return raw;
}
