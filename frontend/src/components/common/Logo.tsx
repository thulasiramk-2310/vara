import React from 'react';

// The VARA mark ("the Fold"): two ivory ribbons folding into a hexagon around a
// void. This is the single-colour form of the brand artwork supplied for VARA (see
// docs/assets/brand/vara-fold-monochrome.svg), drawn in currentColor so it follows
// the theme: dark ink on the light theme, ivory on the dark theme. The two inner
// faces are the same colour at half strength, which keeps the fold readable down to
// about 16 px. The full-colour icon is used where there is a dark tile behind it
// (the browser tab icon, public/favicon.svg).
export const Logo: React.FC<{ size?: number; title?: string }> = ({ size = 24, title }) => (
  <svg width={size} height={size} viewBox="60 15 870 950" role="img" aria-label={title || 'VARA HUB'}>
    {title ? <title>{title}</title> : null}
    <path d="M 145 446 Q 145 410 185 380 L 516 112 Q 576 63 607 119 L 607 254 Q 605 292 567 321 L 199 581 Q 145 619 145 663 Z" fill="currentColor" />
    <path d="M 838 362 Q 850 376 850 405 L 850 550 Q 850 593 806 626 L 493 858 Q 413 921 413 840 L 413 670 Q 413 626 457 594 L 802 402 Q 838 379 838 362 Z" fill="currentColor" />
    <path d="M 615 128 L 806 294 Q 847 329 835 367 Q 818 405 646 508 Q 653 374 575 334 Q 616 285 615 244 Z" fill="currentColor" opacity={0.5} />
    <path d="M 150 634 Q 146 663 191 695 L 399 866 Q 410 880 426 883 Q 401 844 399 738 Q 370 655 382 489 L 196 572 Q 150 607 150 634 Z" fill="currentColor" opacity={0.5} />
  </svg>
);
