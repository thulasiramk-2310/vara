import { useState, useEffect } from 'react';
import { Theme } from '../types';

export function useTheme() {
  const [theme, setTheme] = useState<Theme>('light');

  useEffect(() => {
    let t: Theme = 'light';
    try {
      const s = localStorage.getItem('vara-theme');
      if (s === 'light' || s === 'dark') {
        t = s;
      } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
        t = 'dark';
      }
    } catch {
      // Ignore fallback
    }
    setTheme(t);
  }, []);

  // Apply the theme to <html> (documentElement), not just the .app div, so the
  // themed CSS variables cascade to <body> too. Otherwise body's var(--page)
  // resolves to nothing (white), which shows through on any overflow.
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const toggleTheme = () => {
    const nextTheme = theme === 'light' ? 'dark' : 'light';
    try {
      localStorage.setItem('vara-theme', nextTheme);
    } catch {
      // Ignore storage error
    }
    setTheme(nextTheme);
  };

  return {
    theme,
    isDark: theme === 'dark',
    isLight: theme === 'light',
    toggleTheme,
  };
}
