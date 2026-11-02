import { useCallback, useState } from 'react';

// useStoredFlag is a true/false preference that is remembered between visits (the
// line-wrap switch, whether the file tree is open). Browsers can refuse storage
// (private windows, blocked site data), so every read and write is guarded and
// the switch still works for the current page when storage is unavailable.
export function useStoredFlag(key: string, fallback: boolean): [boolean, (v: boolean) => void] {
  const [value, setValue] = useState<boolean>(() => {
    try {
      const raw = window.localStorage.getItem(key);
      return raw === null ? fallback : raw === '1';
    } catch {
      return fallback;
    }
  });
  const set = useCallback(
    (v: boolean) => {
      setValue(v);
      try {
        window.localStorage.setItem(key, v ? '1' : '0');
      } catch {
        // not remembered, but the page keeps working
      }
    },
    [key],
  );
  return [value, set];
}
