import React from 'react';
import { useToast } from '../../hooks/useToast';

export const Toast: React.FC = () => {
  const { toast } = useToast();

  if (!toast) return null;

  return (
    <div
      role="status"
      style={{
        position: 'fixed',
        left: '50%',
        bottom: '26px',
        transform: 'translateX(-50%)',
        zIndex: 60,
        background: 'var(--text)',
        color: 'var(--bg)',
        borderRadius: '9px',
        padding: '9px 15px',
        fontSize: '13px',
        fontWeight: 500,
        boxShadow: 'var(--shadow-lg)',
        animation: 'v-fade .18s ease',
      }}
    >
      {toast}
    </div>
  );
};
