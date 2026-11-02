import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '../api/client';

export interface AsyncState<T> {
  data?: T;
  error?: ApiError;
  loading: boolean;
  retry: () => void; // run the loader again (the Try again button)
}

// useAsync runs an async loader and tracks {loading, data, error}, cancelling
// stale results when deps change or the component unmounts.
export function useAsync<T>(fn: () => Promise<T>, deps: React.DependencyList): AsyncState<T> {
  const [state, setState] = useState<Omit<AsyncState<T>, 'retry'>>({ loading: true });
  const [attempt, setAttempt] = useState(0);
  const retry = useCallback(() => setAttempt((n) => n + 1), []);

  useEffect(() => {
    let alive = true;
    setState({ loading: true });
    fn()
      .then((data) => alive && setState({ loading: false, data }))
      .catch((error: ApiError) => alive && setState({ loading: false, error }));
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, attempt]);

  return { ...state, retry };
}
