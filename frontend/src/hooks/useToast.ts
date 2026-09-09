import React, { createContext, useContext, useState, useRef } from 'react';

interface ToastContextValue {
  toast: string | null;
  showToast: (message: string) => void;
  copy: (text: string, label?: string) => void;
  noop: () => void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toast, setToast] = useState<string | null>(null);
  const timeoutRef = useRef<number | null>(null);

  const showToast = (message: string) => {
    setToast(message);
    if (timeoutRef.current !== null) {
      window.clearTimeout(timeoutRef.current);
    }
    timeoutRef.current = window.setTimeout(() => {
      setToast(null);
      timeoutRef.current = null;
    }, 1500);
  };

  const copy = (text: string, label?: string) => {
    try {
      navigator.clipboard.writeText(text);
    } catch {
      // Ignore fallback
    }
    showToast(`${label || 'Copied'} to clipboard`);
  };

  const noop = () => showToast('Prototype — not wired');

  return React.createElement(
    ToastContext.Provider,
    { value: { toast, showToast, copy, noop } },
    children
  );
};

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return context;
}
