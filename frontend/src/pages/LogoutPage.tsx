import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthLayout } from '../layouts/AuthLayout';
import { useAuth } from '../hooks/useAuth';

export const LogoutPage: React.FC = () => {
  const navigate = useNavigate();
  const { logout } = useAuth();
  const [clearing, setClearing] = useState(true);
  const ran = useRef(false);

  useEffect(() => {
    // Really revoke the session (DELETE /_vara/sessions/current) and clear the
    // httpOnly cookie. Guard against React 18 StrictMode's double-invoke.
    if (ran.current) return;
    ran.current = true;
    logout().finally(() => setClearing(false));
  }, [logout]);

  const chips = ['session closed', 'tokens cleared', 'immutable audit', 'secure signout'];

  return (
    <AuthLayout authChips={chips}>
      {clearing ? (
        <div style={{ textAlign: 'center', padding: '36px 0', animation: 'v-fade .2s ease' }}>
          <div style={{ fontSize: '38px', marginBottom: '16px', animation: 'v-glow-pulse 1s infinite' }}>🔒</div>
          <h2 style={{ fontSize: '20px', fontWeight: 700, margin: '0 0 8px', color: 'var(--text)' }}>
            Signing out…
          </h2>
          <p style={{ color: 'var(--dim)', fontSize: '13.5px', margin: 0 }}>
            Revoking your session on the server and clearing the session cookie.
          </p>
        </div>
      ) : (
        <div style={{ animation: 'v-slide-up .25s ease' }}>
          <div style={{ width: '56px', height: '56px', borderRadius: '14px', background: 'var(--green-soft)', color: 'var(--green)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '26px', border: '1px solid color-mix(in srgb, var(--green) 35%, transparent)', marginBottom: '20px' }}>
            ✓
          </div>

          <h1 style={{ fontSize: '26px', fontWeight: 750, margin: '0 0 8px', letterSpacing: '-.5px' }}>
            Signed out successfully
          </h1>
          
          <p style={{ color: 'var(--dim)', margin: '0 0 28px', fontSize: '14.5px', lineHeight: 1.5 }}>
            Your VARA session has been revoked on the server and the session cookie has been cleared from this browser.
          </p>

          <div style={{ display: 'grid', gap: '12px' }}>
            <button
              onClick={() => navigate('/login')}
              style={{
                background: 'var(--accent)',
                color: '#fff',
                border: 'none',
                borderRadius: '10px',
                padding: '13px 18px',
                fontSize: '14.5px',
                fontWeight: 680,
                cursor: 'pointer',
                boxShadow: '0 4px 12px color-mix(in srgb, var(--accent) 35%, transparent)',
                transition: 'transform .15s ease'
              }}
              onMouseEnter={(e) => e.currentTarget.style.transform = 'translateY(-1px)'}
              onMouseLeave={(e) => e.currentTarget.style.transform = 'none'}
            >
              Sign in again
            </button>

            <button
              onClick={() => navigate('/dashboard')}
              style={{
                background: 'var(--panel)',
                color: 'var(--text)',
                border: '1px solid var(--border-strong)',
                borderRadius: '10px',
                padding: '12px 18px',
                fontSize: '14px',
                fontWeight: 620,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '8px',
                transition: 'background .15s ease'
              }}
              onMouseEnter={(e) => e.currentTarget.style.background = 'var(--inset)'}
              onMouseLeave={(e) => e.currentTarget.style.background = 'var(--panel)'}
            >
              <span>🌐</span> Explore Public Ecosystem
            </button>
          </div>

          <div style={{ borderTop: '1px solid var(--border)', marginTop: '32px', paddingTop: '20px', textAlign: 'center' }}>
            <button
              onClick={() => navigate('/dashboard')}
              style={{ background: 'none', border: 'none', color: 'var(--dim)', fontSize: '13.5px', fontWeight: 560, cursor: 'pointer', font: 'inherit' }}
              onMouseEnter={(e) => e.currentTarget.style.color = 'var(--text)'}
              onMouseLeave={(e) => e.currentTarget.style.color = 'var(--dim)'}
            >
              ← Return to VARA Home
            </button>
          </div>
        </div>
      )}
    </AuthLayout>
  );
};
