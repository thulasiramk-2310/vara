import React from 'react';

// The VARA HUB mark: a hexagon enclosing a branch graph — two arms converging
// into a stem (the "Y"), with a short branch out to a commit node. The hexagon
// and graph inherit currentColor (so the mark works on any background, light or
// dark); only the commit node is brand red (--brand-red). This is the Hub's
// identity — distinct from the VARA core-engine "V" mark.
export const Logo: React.FC<{ size?: number; title?: string }> = ({ size = 24, title }) => (
  <svg width={size} height={size} viewBox="0 0 32 32" fill="none" role="img" aria-label={title || 'VARA HUB'}>
    {title ? <title>{title}</title> : null}
    {/* Hexagon shell (flat top/bottom, pointed sides) */}
    <path
      d="M9.75 5.2 L22.25 5.2 L28.5 16 L22.25 26.8 L9.75 26.8 L3.5 16 Z"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinejoin="round"
    />
    {/* Branch graph: two arms → stem (the "Y") */}
    <path
      d="M11 11 L16 16.6 L21 11 M16 16.6 V22.6"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    {/* Short branch out to the commit node */}
    <path d="M16 18.6 L20.2 21" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    {/* The commit node — the one brand-red accent */}
    <circle cx="20.7" cy="21.3" r="2.4" fill="var(--brand-red)" />
  </svg>
);
