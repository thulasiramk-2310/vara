import React from 'react';
import { useToast } from '../../hooks/useToast';

// The live region is always in the page, and only its text changes. A region
// that appears together with its message is often not announced by screen
// readers; one that already exists is.
export const Toast: React.FC = () => {
  const { toast } = useToast();

  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      style={
        toast
          ? {
              position: 'fixed',
              left: '50%',
              bottom: '26px',
              transform: 'translateX(-50%)',
              zIndex: 60,
              maxWidth: 'calc(100vw - 32px)',
              background: 'var(--text)',
              color: 'var(--bg)',
              borderRadius: '9px',
              padding: '9px 15px',
              fontSize: '13px',
              fontWeight: 500,
              boxShadow: 'var(--shadow-lg)',
              animation: 'v-fade .18s ease',
            }
          : { position: 'absolute', width: 1, height: 1, overflow: 'hidden', clip: 'rect(0 0 0 0)' }
      }
    >
      {toast}
    </div>
  );
};
