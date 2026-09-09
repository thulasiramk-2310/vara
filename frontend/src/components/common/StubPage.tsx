import React from 'react';
import { DevIcon } from './DevIcon';

interface StubPageProps {
  // Either `title` (global stubs) or `section` (repo-scoped stubs) names the page.
  title?: string;
  section?: string;
  icon?: string;
  blurb?: string;
}

// StubPage marks a feature that is designed but NOT yet backed by the engine —
// pull requests, issues, notifications, organizations, profiles, and settings all
// land in the v0.5 collaboration milestone. It is deliberately honest: no fake
// data, a clear "planned" badge, and a pointer to what the v0.4 backend does serve.
export const StubPage: React.FC<StubPageProps> = ({ title, section, icon = 'projects', blurb }) => {
  const heading = title || section || 'Coming soon';
  const text =
    blurb ||
    `${heading} is part of the v0.5 collaboration layer — designed here, but not yet backed by the engine.`;

  return (
    <div style={{ maxWidth: '640px', margin: '0 auto', padding: '64px 24px', textAlign: 'center', animation: 'v-fade .2s ease' }}>
      <div
        style={{
          width: '64px',
          height: '64px',
          margin: '0 auto 24px',
          borderRadius: '16px',
          background: 'var(--panel)',
          border: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          boxShadow: 'var(--shadow)',
        }}
      >
        <DevIcon name={icon} size={28} color="var(--dim)" />
      </div>

      <div
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          background: 'var(--accent-soft)',
          color: 'var(--accent)',
          border: '1px solid color-mix(in srgb, var(--accent) 30%, transparent)',
          borderRadius: '999px',
          padding: '4px 12px',
          fontSize: '11.5px',
          fontWeight: 700,
          letterSpacing: '0.4px',
          textTransform: 'uppercase',
          marginBottom: '16px',
        }}
      >
        Planned · v0.5
      </div>

      <h1 style={{ fontSize: '24px', fontWeight: 700, margin: '0 0 12px', letterSpacing: '-0.4px' }}>{heading}</h1>
      <p style={{ color: 'var(--dim)', fontSize: '15px', lineHeight: 1.6, margin: '0 auto', maxWidth: '460px' }}>{text}</p>

      <p style={{ color: 'var(--faint)', fontSize: '13px', marginTop: '24px' }}>
        The v0.4 backend serves reads only — browsing files, viewing diffs, and search are fully live.
      </p>
    </div>
  );
};
