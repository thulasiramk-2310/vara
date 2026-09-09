import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Lock, Globe, Check, Loader2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import { useAuth } from '../hooks/useAuth';
import { useToast } from '../hooks/useToast';

// NewRepoPage creates a real repository via the RFC-0019 control plane
// (POST /_vara/repositories). v0.4 repositories are private by design; the
// visibility control is shown for intent but the server keeps new repos private.
export const NewRepoPage: React.FC = () => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const { me, authed } = useAuth();
  const [name, setName] = useState('');
  const [vis, setVis] = useState<'public' | 'private'>('private');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const owner = me?.id && authed ? me.id : 'you';
  // Mirror the server's ValidName rule (RFC-0019 §10) for instant feedback.
  const nameOk = /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(name) && !name.startsWith('.') && !name.startsWith('_');

  const create = async () => {
    if (busy) return;
    setError('');
    if (!name.trim()) {
      setError('Enter a repository name.');
      return;
    }
    if (!nameOk) {
      setError('Names use letters, numbers, dot, dash, underscore — and cannot start with . or _');
      return;
    }
    setBusy(true);
    try {
      await api.createRepo(name.trim());
      showToast(`Created ${name.trim()}`);
      navigate(`/r/${encodeURIComponent(name.trim())}`);
    } catch (err) {
      const e = err as ApiError;
      if (e?.status === 409) setError(`A repository named "${name.trim()}" already exists.`);
      else if (e?.status === 401) setError('Sign in to create a repository.');
      else if (e?.status === 403) setError('You do not have permission to create repositories on this server.');
      else if (e?.code === 'UNAVAILABLE') setError('This server does not expose repository management.');
      else setError(e?.message || 'Could not create the repository.');
    } finally {
      setBusy(false);
    }
  };

  const visOptions = [
    { id: 'private' as const, title: 'Private', sub: 'Only people you grant access can see or commit. (Default in v0.4.)', icon: <Lock size={15} /> },
    { id: 'public' as const, title: 'Public', sub: 'Anyone can read. Public visibility ships with the v0.5 collaboration layer.', icon: <Globe size={15} />, disabled: true },
  ];

  return (
    <div style={{ animation: 'v-fade .2s ease', maxWidth: '620px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '26px', fontWeight: 700, margin: '0 0 6px', letterSpacing: '-0.5px' }}>Create a new repository</h1>
      <p style={{ color: 'var(--dim)', margin: '0 0 26px', fontSize: '14px', lineHeight: 1.5 }}>
        A repository is a measured tree of your project — every object addressed by its SHA-256.
      </p>

      <div style={{ display: 'grid', gap: '22px', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '14px', padding: '24px', boxShadow: 'var(--shadow)' }}>
        <div style={{ display: 'flex', alignItems: 'flex-end', gap: '10px', flexWrap: 'wrap' }}>
          <div style={{ display: 'grid', gap: '6px' }}>
            <span style={{ fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>Owner</span>
            <span style={{ background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '10px', padding: '11px 13px', fontSize: '13.5px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{owner}</span>
          </div>
          <span style={{ paddingBottom: '12px', color: 'var(--faint)', fontSize: '18px' }}>/</span>
          <label style={{ display: 'grid', gap: '6px', flex: 1, minWidth: '180px' }}>
            <span style={{ fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>Repository name</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && create()}
              autoFocus
              spellCheck={false}
              autoCapitalize="none"
              placeholder="my-project"
              style={{ background: 'var(--panel)', border: `1px solid ${name && !nameOk ? 'var(--red)' : 'var(--accent)'}`, borderRadius: '10px', padding: '11px 13px', fontSize: '13.5px', color: 'var(--text)', outline: 'none', fontFamily: 'var(--font-mono)' }}
            />
          </label>
        </div>

        <div style={{ display: 'grid', gap: '10px' }}>
          {visOptions.map((v) => {
            const on = vis === v.id;
            return (
              <label
                key={v.id}
                onClick={() => !v.disabled && setVis(v.id)}
                style={{
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '12px',
                  border: `1px solid ${on ? 'var(--accent)' : 'var(--border)'}`,
                  background: on ? 'var(--accent-soft)' : 'var(--inset)',
                  borderRadius: '12px',
                  padding: '13px 15px',
                  cursor: v.disabled ? 'not-allowed' : 'pointer',
                  opacity: v.disabled ? 0.55 : 1,
                }}
              >
                <span style={{ width: '16px', height: '16px', borderRadius: '50%', border: `2px solid ${on ? 'var(--accent)' : 'var(--border-strong)'}`, marginTop: '2px', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  {on && <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: 'var(--accent)' }} />}
                </span>
                <span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '8px', fontWeight: 560, fontSize: '13.5px' }}>{v.icon}{v.title}{v.disabled ? <span style={{ fontSize: '10.5px', color: 'var(--faint)', border: '1px solid var(--border)', borderRadius: '20px', padding: '1px 8px' }}>v0.5</span> : null}</span>
                  <span style={{ display: 'block', color: 'var(--dim)', fontSize: '12.5px', marginTop: '2px' }}>{v.sub}</span>
                </span>
              </label>
            );
          })}
        </div>

        {error ? (
          <div role="alert" style={{ background: 'var(--red-soft)', border: '1px solid color-mix(in srgb, var(--red) 35%, transparent)', color: 'var(--red)', borderRadius: '10px', padding: '10px 13px', fontSize: '12.5px', lineHeight: 1.45 }}>{error}</div>
        ) : null}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', borderTop: '1px solid var(--border)', paddingTop: '18px' }}>
          <button onClick={() => navigate('/dashboard')} style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', padding: '10px 16px', fontSize: '13.5px', cursor: 'pointer', color: 'var(--text)' }}>
            Cancel
          </button>
          <button
            onClick={create}
            disabled={busy}
            style={{ display: 'inline-flex', alignItems: 'center', gap: '7px', background: 'var(--accent)', color: 'var(--accent-fg)', border: 'none', borderRadius: '10px', padding: '10px 18px', fontSize: '13.5px', fontWeight: 620, cursor: busy ? 'default' : 'pointer', opacity: busy ? 0.75 : 1 }}
          >
            {busy ? <Loader2 size={15} className="v-spin" /> : <Check size={15} />}
            {busy ? 'Creating…' : 'Create repository'}
          </button>
        </div>
      </div>
    </div>
  );
};
