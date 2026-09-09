import React from 'react';
import { User, Shield, Bell, Palette, Check } from 'lucide-react';

// SettingsPage is a professional "under active development" workspace, not an empty
// placeholder. Every control is rendered realistically but disabled with a subtle
// "Coming Soon" treatment — honest about v0.4 being read-only while showing the
// shape of what v0.5 brings. Neutral palette; accent only on interactive/status.

const ComingSoon: React.FC = () => (
  <span style={{ fontSize: '10px', fontWeight: 600, letterSpacing: '0.3px', textTransform: 'uppercase', color: 'var(--dim)', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '20px', padding: '2px 9px' }}>
    Coming soon
  </span>
);

const cardStyle: React.CSSProperties = { background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', boxShadow: 'var(--shadow)' };

const Section: React.FC<{ icon: React.ReactNode; title: string; children: React.ReactNode }> = ({ icon, title, children }) => (
  <div style={cardStyle}>
    <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '15px 20px', borderBottom: '1px solid var(--border)' }}>
      <span style={{ color: 'var(--dim)', display: 'flex' }}>{icon}</span>
      <h2 style={{ margin: 0, fontSize: '15px', fontWeight: 640 }}>{title}</h2>
      <span style={{ flex: 1 }} />
      <ComingSoon />
    </div>
    <div style={{ padding: '6px 20px 8px', opacity: 0.62, pointerEvents: 'none' }}>{children}</div>
  </div>
);

const Field: React.FC<{ label: string; desc?: string; control: React.ReactNode }> = ({ label, desc, control }) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: '16px', padding: '14px 0', borderTop: '1px solid var(--border)' }}>
    <div style={{ minWidth: 0, flex: 1 }}>
      <div style={{ fontSize: '13.5px', fontWeight: 550 }}>{label}</div>
      {desc ? <div style={{ fontSize: '12.5px', color: 'var(--dim)', marginTop: '2px' }}>{desc}</div> : null}
    </div>
    <div style={{ flexShrink: 0 }}>{control}</div>
  </div>
);

const Input: React.FC<{ placeholder: string; wide?: boolean }> = ({ placeholder, wide }) => (
  <input disabled placeholder={placeholder} style={{ width: wide ? '260px' : '200px', maxWidth: '48vw', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '9px', padding: '9px 12px', fontSize: '13px', color: 'var(--dim)' }} />
);

const GhostButton: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span style={{ display: 'inline-block', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '9px', padding: '8px 14px', fontSize: '12.5px', fontWeight: 560, color: 'var(--dim)' }}>{children}</span>
);

const Toggle: React.FC = () => (
  <span style={{ display: 'inline-flex', width: '38px', height: '22px', borderRadius: '20px', background: 'var(--inset)', border: '1px solid var(--border)', padding: '2px', alignItems: 'center' }}>
    <span style={{ width: '16px', height: '16px', borderRadius: '50%', background: 'var(--border-strong)' }} />
  </span>
);

const Segmented: React.FC<{ options: string[]; active?: number }> = ({ options, active = 0 }) => (
  <span style={{ display: 'inline-flex', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '9px', padding: '3px' }}>
    {options.map((o, i) => (
      <span key={o} style={{ padding: '5px 12px', fontSize: '12.5px', fontWeight: 560, borderRadius: '6px', color: i === active ? 'var(--text)' : 'var(--dim)', background: i === active ? 'var(--panel)' : 'transparent' }}>{o}</span>
    ))}
  </span>
);

const SHIPPED = ['Repository management', 'Authentication', 'Search', 'Diff viewer', 'Commits', 'Clone / Push / Fetch'];
const NEXT = ['Organizations', 'Teams', 'Repository permissions', 'Collaboration', 'Notifications'];

export const SettingsPage: React.FC = () => (
  <div style={{ animation: 'v-fade .2s ease', display: 'grid', gap: '22px', minWidth: 0 }}>
    {/* Header */}
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
      <div>
        <h1 style={{ fontSize: '26px', fontWeight: 700, margin: 0, letterSpacing: '-0.5px' }}>Settings</h1>
        <p style={{ color: 'var(--dim)', margin: '5px 0 0', fontSize: '14px' }}>Manage your account and preferences.</p>
      </div>
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: '12px', fontWeight: 600, color: 'var(--accent)', background: 'var(--accent-soft)', border: '1px solid color-mix(in srgb, var(--accent) 30%, transparent)', borderRadius: '20px', padding: '5px 13px' }}>
        Coming in v0.5
      </span>
    </div>

    <div style={{ height: '1px', background: 'var(--border)' }} />

    {/* Two columns */}
    <div className="v-dash">
      {/* Setting sections */}
      <div style={{ display: 'grid', gap: '18px', minWidth: 0 }}>
        <Section icon={<User size={17} />} title="Account">
          <Field label="Username" desc="Your unique handle across the hub" control={<Input placeholder="alice" />} />
          <Field label="Email" desc="Used for sign-in and notifications" control={<Input placeholder="you@example.com" wide />} />
          <Field label="Profile" desc="Display name, bio, and avatar" control={<GhostButton>Edit profile</GhostButton>} />
        </Section>

        <Section icon={<Shield size={17} />} title="Security">
          <Field label="Password" desc="Change your account password" control={<GhostButton>Change</GhostButton>} />
          <Field label="Sessions" desc="Devices where you're signed in" control={<GhostButton>Manage</GhostButton>} />
          <Field label="API tokens" desc="Personal access tokens for the CLI" control={<GhostButton>Generate</GhostButton>} />
          <Field label="SSH keys" desc="Keys for authenticated push over SSH" control={<GhostButton>Add key</GhostButton>} />
        </Section>

        <Section icon={<Bell size={17} />} title="Notifications">
          <Field label="Email notifications" desc="Activity on repositories you follow" control={<Toggle />} />
          <Field label="Push notifications" desc="Real-time alerts in the browser" control={<Toggle />} />
        </Section>

        <Section icon={<Palette size={17} />} title="Appearance">
          <Field label="Theme" desc="Light, dark, or match your system" control={<Segmented options={['Light', 'Dark', 'System']} active={1} />} />
          <Field label="Accent" desc="Highlight color for interactive elements" control={<Segmented options={['Teal', 'Blue', 'Amber']} />} />
          <Field label="Editor preferences" desc="Tab size, font, and wrapping" control={<GhostButton>Configure</GhostButton>} />
        </Section>
      </div>

      {/* Right sidebar */}
      <aside style={{ display: 'grid', gap: '16px', minWidth: 0, alignContent: 'start', position: 'sticky', top: '80px' }}>
        <div style={{ ...cardStyle, padding: '16px 18px', display: 'grid', gap: '12px' }}>
          <span style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 700 }}>Project status</span>
          <div style={{ display: 'grid', gap: '9px' }}>
            {SHIPPED.map((s) => (
              <div key={s} style={{ display: 'flex', alignItems: 'center', gap: '9px', fontSize: '13px' }}>
                <span style={{ width: '18px', height: '18px', borderRadius: '50%', background: 'var(--green-soft)', color: 'var(--green)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                  <Check size={12} strokeWidth={3} />
                </span>
                {s}
              </div>
            ))}
          </div>
        </div>

        <div style={{ ...cardStyle, padding: '16px 18px', display: 'grid', gap: '12px' }}>
          <span style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 700 }}>Coming next</span>
          <div style={{ display: 'grid', gap: '9px' }}>
            {NEXT.map((s) => (
              <div key={s} style={{ display: 'flex', alignItems: 'center', gap: '9px', fontSize: '13px', color: 'var(--dim)' }}>
                <span style={{ width: '7px', height: '7px', borderRadius: '50%', border: '1.5px solid var(--border-strong)', flexShrink: 0, marginLeft: '5px', marginRight: '4px' }} />
                {s}
              </div>
            ))}
          </div>
        </div>

        <div style={{ ...cardStyle, padding: '16px 18px', display: 'grid', gap: '12px' }}>
          <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between' }}>
            <div>
              <div style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 700 }}>Current version</div>
              <div style={{ fontSize: '20px', fontWeight: 700, fontFamily: 'var(--font-mono)', marginTop: '2px' }}>v0.4</div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: '11px', letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--faint)', fontWeight: 700 }}>Next release</div>
              <div style={{ fontSize: '20px', fontWeight: 700, fontFamily: 'var(--font-mono)', marginTop: '2px', color: 'var(--accent)' }}>v0.5</div>
            </div>
          </div>
          <div style={{ height: '6px', borderRadius: '20px', background: 'var(--inset)', overflow: 'hidden' }}>
            <div style={{ width: '80%', height: '100%', borderRadius: '20px', background: 'var(--accent)' }} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--dim)' }}>Read UI shipped. Collaboration layer in progress.</div>
        </div>
      </aside>
    </div>
  </div>
);
