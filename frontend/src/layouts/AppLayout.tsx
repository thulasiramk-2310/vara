import React, { useState, useEffect, useRef } from 'react';
import { Outlet, Link, useNavigate, useLocation, useParams } from 'react-router-dom';
import { routes } from '../routes';
import { useAuth } from '../hooks/useAuth';
import { Identicon } from '../components/common/Identicon';
import { Logo } from '../components/common/Logo';

interface AppLayoutProps {
  theme: string;
  toggleTheme: () => void;
}

export const AppLayout: React.FC<AppLayoutProps> = ({ theme, toggleTheme }) => {
  const location = useLocation();
  const navigate = useNavigate();
  const { repo = '' } = useParams();
  const { me, authed } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const identity = me?.id || 'anonymous';

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);
  
  const isDark = theme === 'dark';
  const isLight = theme === 'light';

  // We are in a repo if the URL starts with /r/
  const inRepo = location.pathname.startsWith('/r/') && repo !== '';
  const isLanding = location.pathname === '/';

  const activeTab = (() => {
    if (location.pathname.includes('/commits') || location.pathname.includes('/commit/')) return 'commits';
    if (location.pathname.includes('/pulls') || location.pathname.includes('/pull/')) return 'pulls';
    if (location.pathname.includes('/issues') || location.pathname.includes('/issue/')) return 'issues';
    if (location.pathname.includes('/search')) return 'search';
    return 'code';
  })();

  const tab = (key: string, label: string, count?: string) => {
    const on = activeTab === key;
    let iconPath = null;
    if (key === 'code') iconPath = <path d="M2 1.75C2 .784 2.784 0 3.75 0h5.586c.464 0 .909.184 1.237.513l2.914 2.914c.329.328.513.773.513 1.237v9.586A1.75 1.75 0 0 1 12.25 16h-8.5A1.75 1.75 0 0 1 2 14.25Z" />;
    if (key === 'commits') iconPath = <path d="M8 0a8 8 0 1 0 0 16A8 8 0 0 0 8 0Zm.75 4a.75.75 0 0 0-1.5 0v4c0 .2.08.39.22.53l2 2a.75.75 0 1 0 1.06-1.06L8.75 7.94Z" />;
    if (key === 'pulls') iconPath = <path d="M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm10 0a2.25 2.25 0 1 1 3 2.122V9A2.5 2.5 0 0 1 12 11.5H9.622a2.251 2.251 0 1 1 0-1.5H12a1 1 0 0 0 1-1V5.372A2.25 2.25 0 0 1 11.5 3.25Z" />;
    if (key === 'issues') iconPath = <><path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Z" /><path d="M8 4a1 1 0 1 1 0 2 1 1 0 0 1 0-2Z" /></>;
    if (key === 'search') iconPath = <path d="M10.68 11.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06ZM11.5 7a4.5 4.5 0 1 0-9 0 4.5 4.5 0 0 0 9 0Z" />;

    return {
      key, label, count, on, iconPath,
      to: key === 'code' ? routes.tree(repo, '') : `/r/${repo}/${key === 'commits' ? 'commits' : key}`
    };
  };

  // No hardcoded counts — commits is real (a count would need a fetch) and
  // pulls/issues are v0.5 stubs with nothing to count.
  const repoTabs = [
    tab('code', 'Code'),
    tab('commits', 'Commits'),
    tab('pulls', 'Pull requests'),
    tab('issues', 'Issues'),
    tab('search', 'Search')
  ];

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <header style={{ position: 'sticky', top: 0, zIndex: 40, display: 'flex', alignItems: 'center', gap: '16px', height: '56px', padding: '0 20px', background: 'color-mix(in srgb, var(--panel) 90%, transparent)', backdropFilter: 'blur(10px)', borderBottom: '1px solid var(--border)' }}>
        <Link to={routes.dashboard} aria-label="Home" style={{ display: 'flex', alignItems: 'center', gap: '9px', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text)', textDecoration: 'none' }}>
          <span style={{ color: 'var(--text)', display: 'flex' }}>
            <Logo size={22} />
          </span>
          <span style={{ fontWeight: 680, fontSize: '16px', letterSpacing: '.2px' }}>VARA <span style={{ color: 'var(--brand-red)' }}>HUB</span></span>
        </Link>
        
        {inRepo && (
          <>
            {/* Repositories are addressed by name only in v0.4 (no owner
                namespaces — RFC-0019 §14 deferred). Show the signed-in identity
                as an owner-ish crumb only when actually authenticated. */}
            {authed && (
              <>
                <span style={{ color: 'var(--faint)' }}>/</span>
                <Link to={routes.profile} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--dim)', fontSize: '14px', padding: 0, textDecoration: 'none', fontFamily: 'ui-monospace, Menlo, monospace' }}>
                  {identity}
                </Link>
              </>
            )}
            <span style={{ color: 'var(--faint)' }}>/</span>
            <Link to={routes.repo(repo)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text)', fontWeight: 620, fontSize: '14px', padding: 0, fontFamily: 'ui-monospace, Menlo, monospace', textDecoration: 'none' }}>
              {repo}
            </Link>
          </>
        )}
        
        <div style={{ flex: 1 }} />
        
        {!isLanding && (
          <div style={{ position: 'relative', width: '280px', maxWidth: '34vw' }}>
            <svg viewBox="0 0 16 16" width="14" height="14" fill="var(--faint)" style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)' }}>
              <path d="M10.68 11.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06ZM11.5 7a4.5 4.5 0 1 0-9 0 4.5 4.5 0 0 0 9 0Z" />
            </svg>
            <input
              onFocus={() => navigate(repo ? routes.search(repo) : routes.dashboard)}
              placeholder="Search VARA…"
              style={{ width: '100%', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '9px', padding: '8px 12px 8px 34px', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
            />
          </div>
        )}

        {authed && (
          <Link to={routes.newRepo} style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', background: 'var(--accent)', color: 'var(--accent-fg)', border: 'none', borderRadius: '8px', padding: '8px 13px', fontSize: '13px', fontWeight: 600, cursor: 'pointer', textDecoration: 'none' }}>
            <svg viewBox="0 0 16 16" width="13" height="13" fill="currentColor">
              <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
            </svg>
            New
          </Link>
        )}

        <button onClick={toggleTheme} aria-label="Toggle theme" style={{ width: '34px', height: '34px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '8px', cursor: 'pointer', color: 'var(--dim)' }}>
          {isDark && (
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <circle cx="12" cy="12" r="4" />
              <line x1="12" y1="3" x2="12" y2="5" />
              <line x1="12" y1="19" x2="12" y2="21" />
              <line x1="5" y1="5" x2="6.4" y2="6.4" />
              <line x1="17.6" y1="17.6" x2="19" y2="19" />
              <line x1="3" y1="12" x2="5" y2="12" />
              <line x1="19" y1="12" x2="21" y2="12" />
              <line x1="5" y1="19" x2="6.4" y2="17.6" />
              <line x1="17.6" y1="6.4" x2="19" y2="5" />
            </svg>
          )}
          {isLight && (
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z" />
            </svg>
          )}
        </button>
        
        {!authed ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
            <Link to="/login" style={{ color: 'var(--text)', fontSize: '13.5px', fontWeight: 560, textDecoration: 'none' }}>Sign in</Link>
            {isLanding && <Link to="/signup" style={{ background: 'var(--accent)', color: 'var(--accent-fg)', borderRadius: '8px', padding: '7px 15px', fontSize: '13.5px', fontWeight: 600, textDecoration: 'none' }}>Sign up</Link>}
          </div>
        ) : (
        <div ref={menuRef} style={{ position: 'relative' }}>
          <button onClick={() => setMenuOpen(!menuOpen)} aria-label="Account" style={{ width: '32px', height: '32px', borderRadius: '50%', border: '1px solid var(--border)', cursor: 'pointer', background: 'none', padding: 0, overflow: 'hidden', display: 'flex' }}>
            <Identicon seed={identity} size={30} />
          </button>

          {menuOpen && (
            <div style={{ position: 'absolute', top: 'calc(100% + 8px)', right: 0, width: '200px', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', boxShadow: 'var(--shadow-lg)', overflow: 'hidden', animation: 'v-slide-up 0.15s ease', zIndex: 50 }}>
              <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', display: 'grid', gap: '2px' }}>
                <span style={{ fontWeight: 600, color: 'var(--text)', fontSize: '13.5px' }}>Signed in as</span>
                <span style={{ color: 'var(--dim)', fontSize: '12.5px', fontFamily: 'ui-monospace, Menlo, monospace', wordBreak: 'break-all' }}>{identity}</span>
              </div>
              <div style={{ padding: '6px' }}>
                <Link to={routes.profile} onClick={() => setMenuOpen(false)} style={{ display: 'block', padding: '8px 10px', color: 'var(--text)', textDecoration: 'none', fontSize: '13.5px', borderRadius: '6px' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--inset)'} onMouseLeave={(e) => e.currentTarget.style.background = 'none'}>
                  Your Profile
                </Link>
                <Link to={routes.settings} onClick={() => setMenuOpen(false)} style={{ display: 'block', padding: '8px 10px', color: 'var(--text)', textDecoration: 'none', fontSize: '13.5px', borderRadius: '6px' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--inset)'} onMouseLeave={(e) => e.currentTarget.style.background = 'none'}>
                  Settings
                </Link>
                <div style={{ height: '1px', background: 'var(--border)', margin: '6px 0' }} />
                <Link to={routes.logout} onClick={() => setMenuOpen(false)} style={{ display: 'block', padding: '8px 10px', color: 'var(--warn)', textDecoration: 'none', fontSize: '13.5px', borderRadius: '6px' }} onMouseEnter={(e) => e.currentTarget.style.background = 'var(--warn-soft)'} onMouseLeave={(e) => e.currentTarget.style.background = 'none'}>
                  Sign out
                </Link>
              </div>
            </div>
          )}
        </div>
        )}
      </header>

      {inRepo && (
        <div style={{ position: 'sticky', top: '56px', zIndex: 30, background: 'var(--panel)', borderBottom: '1px solid var(--border)' }}>
          <div style={{ maxWidth: '1120px', margin: '0 auto', padding: '0 24px', display: 'flex', alignItems: 'center', gap: '2px', overflowX: 'auto' }}>
            {repoTabs.map((t) => (
              <Link 
                key={t.key} 
                to={t.to} 
                style={{ 
                  display: 'inline-flex', alignItems: 'center', gap: '8px', whiteSpace: 'nowrap', background: 'none', 
                  border: 'none', borderBottom: `2px solid ${t.on ? 'var(--accent)' : 'transparent'}`, 
                  color: t.on ? 'var(--text)' : 'var(--dim)', fontWeight: t.on ? 650 : 500, 
                  fontSize: '13.5px', padding: '12px 12px', marginBottom: '-1px', cursor: 'pointer', textDecoration: 'none' 
                }}
              >
                <svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor">{t.iconPath}</svg>
                {t.label}
                {t.count && (
                  <span style={{ fontSize: '11px', color: 'var(--dim)', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '20px', padding: '0 7px', fontWeight: 500 }}>
                    {t.count}
                  </span>
                )}
              </Link>
            ))}
          </div>
        </div>
      )}

      <main style={{ flex: 1, maxWidth: '1120px', margin: '0 auto', padding: '24px 24px 90px', width: '100%' }}>
        <Outlet />
      </main>
    </div>
  );
};
