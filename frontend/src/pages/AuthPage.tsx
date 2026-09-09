import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../hooks/useToast';
import { useAuth } from '../hooks/useAuth';
import { ApiError } from '../api/client';
import { Logo } from '../components/common/Logo';

interface AuthPageProps {
  initialMode: 'login' | 'signup';
}

export const AuthPage: React.FC<AuthPageProps> = ({ initialMode }) => {
  const [mode, setMode] = useState(initialMode);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const { showToast } = useToast();
  const { login } = useAuth();

  const isLogin = mode === 'login';
  const isSignup = mode === 'signup';

  const authTitle = isLogin ? 'Welcome back' : 'Create your account';
  const authSub = isLogin ? 'Sign in to browse and measure your repositories.' : 'Start hosting content-addressed repositories in minutes.';
  const authCta = isLogin ? 'Sign in' : 'Create account';
  const authSwapText = isLogin ? 'New to VARA?' : 'Already have an account?';
  const authSwapLink = isLogin ? 'Create an account' : 'Sign in';

  const authChips = ['content-addressed', 'transactional', 'self-hosted', 'RFC-driven'];

  const doAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;
    setError('');

    // Self-service signup has no v0.4 endpoint — accounts are provisioned by the
    // server operator (RFC-0020). Be honest rather than faking success.
    if (isSignup) {
      setError('Accounts are provisioned by the server operator in v0.4. Ask your admin, then sign in.');
      return;
    }

    if (!username.trim() || !password) {
      setError('Enter your username and password.');
      return;
    }

    setSubmitting(true);
    try {
      await login(username.trim(), password);
      showToast('Signed in');
      navigate('/dashboard');
    } catch (err) {
      const e2 = err as ApiError;
      // The server returns an indistinguishable 401 for wrong/absent/disabled
      // credentials (RFC-0020 §12); a code 0 means the route isn't enabled here.
      if (e2?.code === 'UNAVAILABLE') {
        setError('This server has authentication disabled — browsing is anonymous.');
      } else if (e2?.status === 401) {
        setError('Incorrect username or password.');
      } else {
        setError(e2?.message || 'Sign in failed. Try again.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const swapAuth = (e: React.MouseEvent) => {
    e.preventDefault();
    setError('');
    setMode(isLogin ? 'signup' : 'login');
  };

  const noop = (e: React.MouseEvent) => {
    e.preventDefault();
    showToast('Prototype — not wired');
  };

  return (
    <div style={{ minHeight: '100vh', display: 'grid', gridTemplateColumns: '1fr 1fr' }}>
      
      {/* Left Form Panel */}
      <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', padding: '40px' }}>
        <div style={{ width: '100%', maxWidth: '360px', animation: 'v-fade .25s ease' }}>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '28px' }}>
            <span style={{ color: 'var(--text)', display: 'flex' }}>
              <Logo size={26} />
            </span>
            <span style={{ fontWeight: 680, fontSize: '19px', letterSpacing: '.2px' }}>VARA <span style={{ color: 'var(--brand-red)' }}>HUB</span></span>
          </div>
          
          <h1 style={{ fontSize: '25px', fontWeight: 660, margin: '0 0 6px', letterSpacing: '-.5px' }}>{authTitle}</h1>
          <p style={{ color: 'var(--dim)', margin: '0 0 26px', fontSize: '14px' }}>{authSub}</p>

          <form onSubmit={doAuth} style={{ display: 'grid', gap: '14px' }}>
            
            {isSignup && (
              <label style={{ display: 'grid', gap: '6px' }}>
                <span style={{ fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>Full name</span>
                <input placeholder="Ada Lovelace" style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '9px', padding: '11px 13px', fontSize: '14px', color: 'var(--text)', outline: 'none' }} />
              </label>
            )}
            
            <label style={{ display: 'grid', gap: '6px' }}>
              <span style={{ fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>Username</span>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                placeholder="alice"
                style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '9px', padding: '11px 13px', fontSize: '14px', color: 'var(--text)', outline: 'none' }}
              />
            </label>

            <label style={{ display: 'grid', gap: '6px' }}>
              <span style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>Password</span>
                {isLogin && <a href="#" onClick={noop} style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none' }}>Forgot?</a>}
              </span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete={isLogin ? 'current-password' : 'new-password'}
                placeholder="••••••••••"
                style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '9px', padding: '11px 13px', fontSize: '14px', color: 'var(--text)', outline: 'none' }}
              />
            </label>

            {error && (
              <div role="alert" style={{ background: 'var(--warn-soft)', border: '1px solid color-mix(in srgb, var(--warn) 35%, transparent)', color: 'var(--warn)', borderRadius: '9px', padding: '9px 12px', fontSize: '12.5px', lineHeight: 1.4 }}>
                {error}
              </div>
            )}

            <button type="submit" disabled={submitting} style={{ background: 'var(--accent)', color: 'var(--accent-fg)', border: 'none', borderRadius: '9px', padding: '12px', fontSize: '14px', fontWeight: 620, cursor: submitting ? 'default' : 'pointer', opacity: submitting ? 0.7 : 1, marginTop: '4px' }}>
              {submitting ? 'Signing in…' : authCta}
            </button>
            
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', color: 'var(--faint)', fontSize: '12px', margin: '2px 0' }}>
              <span style={{ flex: 1, height: '1px', background: 'var(--border)' }} />or<span style={{ flex: 1, height: '1px', background: 'var(--border)' }} />
            </div>
            
            <button type="button" onClick={noop} style={{ background: 'var(--panel)', color: 'var(--text)', border: '1px solid var(--border)', borderRadius: '9px', padding: '11px', fontSize: '13.5px', fontWeight: 560, cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '9px' }}>
              <svg viewBox="0 0 16 16" width="15" height="15" fill="currentColor">
                <path d="M8.5 1.75a.75.75 0 0 0-1 0L2.5 6.5a.75.75 0 0 0-.25.56v6.19c0 .41.34.75.75.75h3.25V10.5a1.75 1.75 0 0 1 3.5 0v3.5h3.25a.75.75 0 0 0 .75-.75V7.06a.75.75 0 0 0-.25-.56Z" />
              </svg>
              Continue with SSO
            </button>
            
          </form>

          <p style={{ textAlign: 'center', margin: '24px 0 0', color: 'var(--dim)', fontSize: '13px' }}>
            {authSwapText} <a href="#" onClick={swapAuth} style={{ color: 'var(--accent)', fontWeight: 560, textDecoration: 'none' }}>{authSwapLink}</a>
          </p>
        </div>
      </div>
      
      {/* Right Feature Panel */}
      <div style={{ background: 'var(--inset)', borderLeft: '1px solid var(--border)', display: 'flex', flexDirection: 'column', justifyContent: 'center', padding: '56px', position: 'relative', overflow: 'hidden' }}>
        <div style={{ maxWidth: '400px', animation: 'v-fade .3s ease' }}>
          
          <div style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', fontSize: '12px', color: 'var(--dim)', marginBottom: '18px' }}>
            $ vara measure .
          </div>
          
          <blockquote style={{ fontSize: '22px', lineHeight: 1.35, fontWeight: 560, letterSpacing: '-.3px', margin: '0 0 18px' }}>
            Version control, measured against a fixed rod. Every object is its own SHA-256 address — immutable, verifiable, yours.
          </blockquote>
          
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            {authChips.map((c) => (
              <span key={c} style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '7px', padding: '5px 11px', fontSize: '12px', color: 'var(--dim)', fontFamily: 'ui-monospace, Menlo, monospace', whiteSpace: 'nowrap' }}>
                {c}
              </span>
            ))}
          </div>
          
          <div style={{ marginTop: '34px', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', padding: '14px 16px', boxShadow: 'var(--shadow)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '10px' }}>
              <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: 'var(--green)' }} />
              <span style={{ fontSize: '12.5px', color: 'var(--dim)' }}>latest release</span>
              <span style={{ flex: 1 }} />
              <span style={{ fontFamily: 'ui-monospace, Menlo, monospace', fontSize: '12px', color: 'var(--text)' }}>v0.4.0</span>
            </div>
            <div style={{ fontSize: '13px', color: 'var(--text)' }}>
              Hub read UI — repository browser, diff viewer, search.
            </div>
          </div>
          
        </div>
      </div>
      
    </div>
  );
};
