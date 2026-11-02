import React, { createContext, useContext, useState, useRef } from 'react';
import { copyText } from '../utils/clipboard';

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
    }, 4000);
  };

  const copy = (text: string, label?: string) => {
    void copyText(text).then((ok) =>
      showToast(ok ? `${label || 'Copied'} to clipboard` : 'Could not copy. Select the text and copy it by hand.'),
    );
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
