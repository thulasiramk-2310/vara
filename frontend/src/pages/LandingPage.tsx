import React from 'react';
import { Link } from 'react-router-dom';
import { useToast } from '../hooks/useToast';
import { DevIcon } from '../components/common/DevIcon';

// HowItWorks renders the animated VARA terminal demo (docs/assets/vara-demo.svg,
// copied into public/). Its CSS keyframe animations — typed command lines and a
// blinking cursor — play when embedded via <img>, and it stays fully
// self-contained (no CDN, no external references).
const HowItWorks: React.FC = () => (
  <img
    src="/vara-demo.svg"
    alt="VARA terminal demo — vara init, add, commit, history, and verify"
    style={{ width: '100%', maxWidth: '700px', height: 'auto', display: 'block' }}
  />
);

export const LandingPage: React.FC = () => {
  const { showToast } = useToast();
  const copy = (text: string) => { navigator.clipboard.writeText(text); showToast('Copied to clipboard'); };

  return (
    <div style={{ overflowX: 'hidden', animation: 'v-fade .2s ease' }}>
      {/* Hero */}
      <section style={{ position: 'relative', padding: '40px 0 48px', display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', overflow: 'hidden' }}>
        <div style={{ position: 'absolute', top: '10%', left: '50%', transform: 'translate(-50%, -50%)', width: '520px', height: '520px', background: 'radial-gradient(circle, color-mix(in srgb, var(--accent) 16%, transparent), transparent 65%)', animation: 'v-glow-pulse 4s ease-in-out infinite', zIndex: 0, pointerEvents: 'none' }} />

        <h1 style={{ position: 'relative', zIndex: 1, fontSize: 'clamp(38px, 6vw, 62px)', fontWeight: 800, lineHeight: 1.08, letterSpacing: '-1.5px', margin: '0 0 18px', maxWidth: '760px' }}>
          <span style={{ background: 'linear-gradient(120deg, var(--accent), var(--purple), #60a5fa, var(--accent))', backgroundSize: '200% auto', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent', animation: 'v-gradient-shift 6s linear infinite' }}>
            Modern Version Control
          </span>
          <br />for Developer Teams.
        </h1>

        <p style={{ position: 'relative', zIndex: 1, fontSize: 'clamp(15px, 1.6vw, 18px)', color: 'var(--dim)', lineHeight: 1.55, maxWidth: '560px', margin: '0 0 26px' }}>
          Blazing-fast operations, cryptographic integrity, and content-addressed history. Ship code with confidence at any scale.
        </p>

        <div style={{ position: 'relative', zIndex: 1, display: 'flex', gap: '12px', flexWrap: 'wrap', justifyContent: 'center' }}>
          <Link to="/signup" style={{ background: 'var(--accent)', color: 'var(--accent-fg)', borderRadius: '10px', padding: '13px 28px', fontSize: '15px', fontWeight: 650, textDecoration: 'none', boxShadow: '0 8px 24px -6px color-mix(in srgb, var(--accent) 50%, transparent)', transition: 'transform .15s ease' }}
            onMouseEnter={(e) => { e.currentTarget.style.transform = 'translateY(-2px)'; }} onMouseLeave={(e) => { e.currentTarget.style.transform = 'none'; }}>
            Get Started Free
          </Link>
          <Link to="/dashboard" style={{ background: 'var(--panel)', color: 'var(--text)', border: '1px solid var(--border-strong)', borderRadius: '10px', padding: '13px 24px', fontSize: '15px', fontWeight: 600, textDecoration: 'none', transition: 'border-color .15s ease' }}
            onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--text)'; }} onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; }}>
            View Live Demo
          </Link>
        </div>

        <div style={{ position: 'relative', zIndex: 1, marginTop: '28px', background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '10px', padding: '10px 16px', display: 'inline-flex', alignItems: 'center', gap: '12px', fontFamily: 'var(--font-mono)', fontSize: '13.5px', boxShadow: 'var(--shadow-md)' }}>
          <span style={{ color: 'var(--dim)' }}>$</span>
          <span style={{ color: 'var(--text)' }}>curl -sL https://get.vara.dev | bash</span>
          <button onClick={() => copy('curl -sL https://get.vara.dev | bash')} style={{ background: 'var(--inset)', color: 'var(--dim)', border: '1px solid var(--border)', borderRadius: '6px', padding: '3px 9px', fontSize: '12px', fontWeight: 600, cursor: 'pointer' }}>
            Copy
          </button>
        </div>
      </section>

      {/* How it works */}
      <section style={{ maxWidth: '960px', margin: '0 auto 52px', padding: '0 24px' }}>
        <div style={{ textAlign: 'center', marginBottom: '20px' }}>
          <span style={{ fontSize: '12px', color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '1px', fontWeight: 700 }}>How it works</span>
          <h2 style={{ fontSize: '26px', fontWeight: 700, letterSpacing: '-.6px', margin: '8px 0 0' }}>Every object is its own address</h2>
        </div>
        <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '18px', padding: '24px 20px', display: 'flex', justifyContent: 'center', boxShadow: 'var(--shadow)' }}>
          <HowItWorks />
        </div>
      </section>

      {/* Features */}
      <section style={{ maxWidth: '1120px', margin: '0 auto 52px', padding: '0 24px' }}>
        <div style={{ textAlign: 'center', marginBottom: '32px' }}>
          <h2 style={{ fontSize: '28px', fontWeight: 700, letterSpacing: '-.6px', margin: '0 0 10px' }}>Everything you need to ship with confidence</h2>
          <p style={{ color: 'var(--dim)', fontSize: '15px', margin: 0 }}>Built from the ground up for modern development workflows.</p>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '16px' }}>
          {[
            { icon: 'lightning' as const, color: 'var(--accent)', title: 'Blazing Fast', desc: 'Instant diffs and operations across repos of any size. No waiting, no slowdowns.' },
            { icon: 'shield' as const, color: 'var(--green)', title: 'Cryptographic Integrity', desc: 'Every change is verified and tamper-proof. Your history is always trustworthy.' },
            { icon: 'users' as const, color: 'var(--purple)', title: 'Built for Teams', desc: 'Branches, merges, and review — all built in. Collaborate without friction.' },
          ].map((feat) => (
            <div key={feat.title} style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '26px 24px', transition: 'transform .2s ease, box-shadow .2s ease' }}
              onMouseEnter={(e) => { e.currentTarget.style.transform = 'translateY(-4px)'; e.currentTarget.style.boxShadow = 'var(--shadow-lg)'; }}
              onMouseLeave={(e) => { e.currentTarget.style.transform = 'none'; e.currentTarget.style.boxShadow = 'none'; }}>
              <div style={{ marginBottom: '16px', width: '48px', height: '48px', borderRadius: '12px', background: 'var(--inset)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <DevIcon name={feat.icon} size={24} color={feat.color} />
              </div>
              <h3 style={{ fontSize: '17px', fontWeight: 700, margin: '0 0 8px' }}>{feat.title}</h3>
              <p style={{ color: 'var(--dim)', fontSize: '14px', lineHeight: 1.55, margin: 0 }}>{feat.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Bottom CTA */}
      <section style={{ background: 'radial-gradient(circle at 50% 100%, color-mix(in srgb, var(--accent) 14%, var(--panel)), var(--panel))', border: '1px solid var(--border)', borderRadius: '20px', padding: '44px 32px', textAlign: 'center', margin: '0 0 40px', boxShadow: 'var(--shadow-lg)' }}>
        <h2 style={{ fontSize: '30px', fontWeight: 750, letterSpacing: '-.8px', margin: '0 0 12px' }}>Ready to ship faster?</h2>
        <p style={{ color: 'var(--dim)', fontSize: '15px', maxWidth: '460px', margin: '0 auto 24px' }}>Join thousands of developers building with confidence.</p>
        <Link to="/signup" style={{ display: 'inline-block', background: 'linear-gradient(135deg, var(--accent), var(--purple))', color: '#fff', textDecoration: 'none', borderRadius: '10px', padding: '14px 32px', fontSize: '15px', fontWeight: 650, boxShadow: '0 8px 24px -6px color-mix(in srgb, var(--accent) 50%, transparent)', transition: 'transform .15s ease' }}
          onMouseEnter={(e) => { e.currentTarget.style.transform = 'translateY(-2px)'; }} onMouseLeave={(e) => { e.currentTarget.style.transform = 'none'; }}>
          Create Free Account
        </Link>
      </section>

      {/* Footer */}
      <footer style={{ borderTop: '1px solid var(--border)', paddingTop: '28px', display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: '24px', color: 'var(--dim)', fontSize: '13px' }}>
        <div style={{ display: 'grid', gap: '8px', maxWidth: '280px' }}>
          <span style={{ fontWeight: 700, fontSize: '16px', color: 'var(--text)' }}>VARA</span>
          <span>Content-addressed version control for speed, security, and scale.</span>
          <span style={{ color: 'var(--faint)' }}>© {new Date().getFullYear()} VARA</span>
        </div>
        <div style={{ display: 'flex', gap: '48px', flexWrap: 'wrap' }}>
          <div style={{ display: 'grid', gap: '8px', alignContent: 'start' }}>
            <span style={{ fontWeight: 650, color: 'var(--text)', marginBottom: '4px' }}>Product</span>
            <Link to="/dashboard" style={{ color: 'var(--dim)', textDecoration: 'none' }}>Repositories</Link>
            <Link to="/dashboard" style={{ color: 'var(--dim)', textDecoration: 'none' }}>Dashboard</Link>
            <Link to="/new" style={{ color: 'var(--dim)', textDecoration: 'none' }}>New repository</Link>
          </div>
          <div style={{ display: 'grid', gap: '8px', alignContent: 'start' }}>
            <span style={{ fontWeight: 650, color: 'var(--text)', marginBottom: '4px' }}>Account</span>
            <Link to="/login" style={{ color: 'var(--dim)', textDecoration: 'none' }}>Sign in</Link>
            <Link to="/signup" style={{ color: 'var(--dim)', textDecoration: 'none' }}>Sign up</Link>
            <Link to="/profile" style={{ color: 'var(--dim)', textDecoration: 'none' }}>Profile</Link>
          </div>
        </div>
      </footer>
    </div>
  );
};
