import React from 'react';
import { Link } from 'react-router-dom';
import { useToast } from '../hooks/useToast';
import { usePageTitle } from '../hooks/usePageTitle';

const INSTALL = 'curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh';
const REPO = 'https://github.com/thulasiramk-2310/vara';

// Real output, captured from a v0.5-dev build in a scratch repository. The
// integrity report is shortened to its result line; the elision is marked.
const SESSION: { cmd?: string; out?: string }[] = [
  { cmd: 'vara init' },
  { out: 'Initialized empty VARA repository in /work/proj/.vara' },
  { cmd: 'vara add notes.txt' },
  { cmd: 'vara commit -m "first note"' },
  { out: '[1ed50c7] first note' },
  { cmd: 'vara log' },
  { out: 'commit 1ed50c7f2842c6d405907952a3d58d95221045172bf6d68f0bfccaa991461fff' },
  { out: 'author: Ada <ada@example.com>' },
  { out: '    first note' },
  { cmd: 'vara verify' },
  { out: '...' },
  { out: 'Result    Repository Healthy' },
];

const TODAY: { title: string; body: string }[] = [
  {
    title: 'Content-addressed',
    body: 'Every file, tree and commit is stored under its own SHA-256 hash, and stored objects are read-only on disk.',
  },
  {
    title: 'Verifiable',
    body: 'vara verify re-hashes objects, trees, commits and refs, and exits non-zero when anything no longer matches.',
  },
  {
    title: 'Safe to interrupt',
    body: 'State files are written to a temporary file and renamed into place, so an interrupted write leaves the previous state intact.',
  },
  {
    title: 'Self-hosted Hub',
    body: 'vara serve runs a Hub with accounts, per-repository permissions and this browser interface. You run it; nobody else holds your history.',
  },
];

const NOT_YET: string[] = [
  'Pull requests, issues and an explore page are planned for v0.5. The Hub in v0.4 reads: files, history, diffs and search.',
  'A repository can be marked public, but that does not yet allow anonymous reads. Access is granted per account in the policy file.',
];

// Accent colour is spent only on what is specific to VARA: object hashes and
// the verified result.
const fmt = (line = ''): React.ReactNode => {
  const h = line.match(/^(\[|commit )([0-9a-f]{7})/);
  if (h) {
    const at = h[1].length;
    return <>{line.slice(0, at)}<span style={{ color: 'var(--accent-strong)', fontWeight: 600 }}>{line.slice(at, at + 7)}</span>{line.slice(at + 7)}</>;
  }
  if (line.startsWith('Result')) {
    return <>Result    <span style={{ color: 'var(--green)', fontWeight: 600 }}>Repository Healthy</span></>;
  }
  return line;
};

const mono: React.CSSProperties = { fontFamily: 'var(--font-mono)' };
const wrap: React.CSSProperties = { maxWidth: '1040px', margin: '0 auto', padding: '0 24px' };
const rule = '1px solid var(--border-strong)';

export const LandingPage: React.FC = () => {
  usePageTitle();
  const { showToast } = useToast();
  const copy = (text: string) => { navigator.clipboard.writeText(text); showToast('Copied to clipboard'); };

  return (
    <div style={{ animation: 'v-fade .2s ease', overflowX: 'hidden' }}>
      <section style={{ ...wrap, padding: '72px 24px 56px' }}>
        <p style={{ ...mono, fontSize: '12.5px', letterSpacing: '.04em', color: 'var(--dim)', margin: '0 0 28px' }}>
          VARA 0.4 &nbsp;/&nbsp; version control you host yourself
        </p>

        <h1 style={{ fontSize: 'clamp(38px, 7vw, 76px)', fontWeight: 700, lineHeight: 1.04, letterSpacing: '-.035em', margin: '0 0 28px', maxWidth: '880px', color: 'var(--text)' }}>
          Version control,
          <br />
          <span style={{ color: 'var(--dim)' }}>measured against a fixed rod.</span>
        </h1>

        <p style={{ fontSize: '18px', lineHeight: 1.6, color: 'var(--dim)', maxWidth: '600px', margin: '0 0 36px' }}>
          History is stored as SHA-256 hashes, so it can be re-measured at any time: change one byte and the hash no longer matches. That is what lets it be checked instead of trusted.
          VARA works on your machine with no account, and a Hub you run yourself adds sharing.
        </p>

        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '12px 28px' }}>
          <Link to="/dashboard" style={{ background: 'var(--text)', color: 'var(--bg)', borderRadius: '6px', padding: '12px 22px', fontSize: '15px', fontWeight: 600, textDecoration: 'none' }}>
            Browse repositories
          </Link>
          <a href={REPO} style={{ color: 'var(--text)', fontSize: '15px', fontWeight: 600, textDecoration: 'underline', textUnderlineOffset: '4px', textDecorationColor: 'var(--border-strong)' }}>
            Read the source
          </a>
        </div>
      </section>

      <section style={{ ...wrap, paddingBottom: '72px' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '8px 16px', borderTop: rule, borderBottom: rule, padding: '14px 0', ...mono, fontSize: '13.5px' }}>
          <span style={{ color: 'var(--faint)' }}>install</span>
          <span tabIndex={0} style={{ color: 'var(--text)', whiteSpace: 'nowrap', overflowX: 'auto', minWidth: 0, flex: '1 1 260px', paddingBottom: '2px' }}>{INSTALL}</span>
          <button onClick={() => copy(INSTALL)} style={{ ...mono, background: 'none', border: 'none', color: 'var(--accent-strong)', fontSize: '13px', fontWeight: 600, cursor: 'pointer', padding: 0, textDecoration: 'underline', textUnderlineOffset: '3px' }}>
            copy
          </button>
        </div>
      </section>

      <section style={{ ...wrap, paddingBottom: '88px' }}>
        <pre tabIndex={0} aria-label="A VARA session: init, add, commit, log and verify" style={{ ...mono, margin: 0, background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '6px', padding: '22px 24px', fontSize: '13.5px', lineHeight: 1.75, overflowX: 'auto', color: 'var(--dim)' }}>
          {SESSION.map((l, i) => (
            <div key={i} style={{ whiteSpace: 'pre' }}>
              {l.cmd !== undefined
                ? <span style={{ color: 'var(--text)', fontWeight: 600 }}><span style={{ color: 'var(--faint)' }}>$ </span>{l.cmd}</span>
                : fmt(l.out)}
            </div>
          ))}
        </pre>
      </section>

      <section style={{ ...wrap, paddingBottom: '88px' }}>
        <h2 style={{ fontSize: '13px', fontWeight: 600, letterSpacing: '.08em', textTransform: 'uppercase', color: 'var(--dim)', margin: '0 0 8px' }}>
          What it does today
        </h2>
        <div>
          {TODAY.map((row, i) => (
            <div key={row.title} style={{ display: 'flex', flexWrap: 'wrap', gap: '8px 32px', padding: '26px 0', borderBottom: rule, borderTop: i === 0 ? rule : 'none' }}>
              <span style={{ ...mono, fontSize: '13px', color: 'var(--faint)', width: '32px', paddingTop: '4px' }}>{String(i + 1).padStart(2, '0')}</span>
              <h3 style={{ fontSize: '21px', fontWeight: 600, letterSpacing: '-.015em', margin: 0, flex: '1 1 220px', maxWidth: '300px' }}>{row.title}</h3>
              <p style={{ fontSize: '16px', lineHeight: 1.65, color: 'var(--text)', opacity: 0.74, margin: 0, flex: '2 1 320px', maxWidth: '560px' }}>{row.body}</p>
            </div>
          ))}
        </div>
      </section>

      <section style={{ ...wrap, paddingBottom: '88px' }}>
        <h2 style={{ fontSize: '13px', fontWeight: 600, letterSpacing: '.08em', textTransform: 'uppercase', color: 'var(--dim)', margin: '0 0 20px' }}>
          Not yet
        </h2>
        {NOT_YET.map((t) => (
          <p key={t} style={{ fontSize: '16px', lineHeight: 1.65, color: 'var(--text)', opacity: 0.74, margin: '0 0 14px', maxWidth: '680px' }}>{t}</p>
        ))}
      </section>

      <footer style={{ ...wrap, paddingBottom: '48px' }}>
        <div style={{ borderTop: rule, paddingTop: '28px', display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: '16px', fontSize: '14px', color: 'var(--dim)' }}>
        <span>VARA, content-addressed version control. Self-hosted.</span>
        <span style={{ display: 'flex', gap: '24px' }}>
          <Link to="/dashboard" style={{ color: 'var(--dim)' }}>Repositories</Link>
          <Link to="/login" style={{ color: 'var(--dim)' }}>Sign in</Link>
          <a href={REPO} style={{ color: 'var(--dim)' }}>Source</a>
        </span>
        </div>
      </footer>
    </div>
  );
};
