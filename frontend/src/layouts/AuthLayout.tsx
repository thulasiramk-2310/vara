import React from 'react';
import { useAppNavigation } from '../hooks/useAppNavigation';
import { Logo } from '../components/common/Logo';

interface AuthLayoutProps {
  children: React.ReactNode;
  authChips: string[];
}

export const AuthLayout: React.FC<AuthLayoutProps> = ({ children, authChips }) => {
  const { go } = useAppNavigation();

  return (
    <div style={{ minHeight: '100vh', display: 'grid', gridTemplateColumns: '1fr 1fr' }}>
      <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', padding: '40px' }}>
        <div style={{ width: '100%', maxWidth: '360px', animation: 'v-fade .25s ease' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '28px', cursor: 'pointer' }} onClick={() => go('/')}>
            <span style={{ color: 'var(--text)', display: 'flex' }}>
              <Logo size={26} />
            </span>
            <span style={{ fontWeight: 680, fontSize: '19px', letterSpacing: '.2px' }}>VARA <span style={{ color: 'var(--brand-red)' }}>HUB</span></span>
          </div>
          {children}
        </div>
      </div>
      <div
        style={{
          background: 'var(--inset)',
          borderLeft: '1px solid var(--border)',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          padding: '56px',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div style={{ maxWidth: '400px', animation: 'v-fade .3s ease' }}>
          <div style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', fontSize: '12px', color: 'var(--dim)', marginBottom: '18px' }}>
            $ vara measure .
          </div>
          <blockquote style={{ fontSize: '22px', lineHeight: 1.35, fontWeight: 560, letterSpacing: '-.3px', margin: '0 0 18px' }}>
            Version control, measured against a fixed rod. Every object is its own SHA-256 address — immutable, verifiable, yours.
          </blockquote>
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            {authChips.map((c, idx) => (
              <span
                key={idx}
                style={{
                  background: 'var(--panel)',
                  border: '1px solid var(--border)',
                  borderRadius: '7px',
                  padding: '5px 11px',
                  fontSize: '12px',
                  color: 'var(--dim)',
                  fontFamily: 'ui-monospace, Menlo, monospace',
                  whiteSpace: 'nowrap',
                }}
              >
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
            <div style={{ fontSize: '13px', color: 'var(--text)' }}>Hub read UI — repository browser, diff viewer, search.</div>
          </div>
        </div>
      </div>
    </div>
  );
};
