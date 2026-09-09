import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { api, Whoami } from '../api/client';

// AuthProvider holds the real signed-in identity (RFC-0020 whoami) and exposes
// login/logout backed by the session-cookie endpoints. It is the single source of
// truth for "who am I" across the app — the header menu, profile, and auth pages
// all read from here instead of hardcoding a user. The session secret lives only
// in an httpOnly cookie; this context never sees or stores it.

interface AuthContextValue {
  me: Whoami | null; // null until the first whoami resolves
  loading: boolean; // true during the initial identity probe
  authed: boolean; // signed in as a real (non-anonymous) account
  refresh: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const ANON: Whoami = { id: 'anonymous', anonymous: true };

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [me, setMe] = useState<Whoami | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setMe(await api.whoami());
    } catch {
      // whoami is unavailable (e.g. an anonymous server without accounts) — treat
      // the caller as anonymous rather than surfacing an error.
      setMe(ANON);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const login = useCallback(
    async (username: string, password: string) => {
      // The cookie is set by this call; re-probe whoami to learn the identity.
      await api.login(username, password);
      await refresh();
    },
    [refresh],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      // Whether or not the server acknowledged, drop to anonymous locally and
      // reconcile with the server's view.
      setMe(ANON);
      await refresh();
    }
  }, [refresh]);

  const authed = !!me && !me.anonymous && me.id !== ANON.id;

  return (
    <AuthContext.Provider value={{ me, loading, authed, refresh, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return ctx;
}
