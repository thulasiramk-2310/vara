import React from 'react';
import { ApiError } from '../../api/client';

// Loading is a quiet inline spinner for data fetches.
export const Loading: React.FC<{ label?: string }> = ({ label = 'Loading…' }) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '32px 4px', color: 'var(--dim)', fontSize: '13.5px' }}>
    <span
      style={{
        width: '15px',
        height: '15px',
        border: '2px solid var(--border-strong)',
        borderTopColor: 'var(--accent)',
        borderRadius: '50%',
        animation: 'v-spin .7s linear infinite',
      }}
    />
    {label}
  </div>
);

// Empty is a neutral "nothing here" panel.
export const Empty: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div
    style={{
      background: 'var(--panel)',
      border: '1px solid var(--border)',
      borderRadius: '12px',
      padding: '40px 20px',
      textAlign: 'center',
      color: 'var(--dim)',
      fontSize: '13.5px',
    }}
  >
    {children}
  </div>
);

// ErrorState renders an ApiError from the read layer, distinguishing the cases
// the backend can legitimately return: 401 (sign in), 403 (not permitted), and
// UNAVAILABLE (the route isn't enabled on this server).
export const ErrorState: React.FC<{ error: ApiError }> = ({ error }) => {
  let title = 'Something went wrong';
  let detail = error.message;
  if (error.status === 401) {
    title = 'Sign in required';
    detail = 'This repository requires you to sign in.';
  } else if (error.status === 403) {
    title = 'Not permitted';
    detail = `You don't have permission to read this (${error.code || 'forbidden'}).`;
  } else if (error.code === 'UNAVAILABLE') {
    title = 'Not enabled';
    detail = error.message;
  } else if (error.status === 404) {
    title = 'Not found';
  }
  return (
    <div
      style={{
        background: 'var(--panel)',
        border: '1px solid var(--border)',
        borderRadius: '12px',
        padding: '32px 24px',
        color: 'var(--text)',
      }}
    >
      <div style={{ fontSize: '15px', fontWeight: 650, marginBottom: '6px' }}>{title}</div>
      <div style={{ color: 'var(--dim)', fontSize: '13.5px' }}>{detail}</div>
    </div>
  );
};
