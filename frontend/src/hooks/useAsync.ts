import { QueryClient, useQuery } from '@tanstack/react-query';
import { ApiError } from '../api/client';

export interface AsyncState<T> {
  data?: T;
  error?: ApiError;
  loading: boolean;
  retry: () => void; // fetch again (the Try again button)
}

// shouldRetry decides which failures are worth another attempt. A network
// failure or a 5xx may pass on its own; a 4xx (not found, not permitted, sign in)
// will give the same answer again, so retrying only delays the message.
export function shouldRetry(failures: number, error: unknown): boolean {
  const e = error as Partial<ApiError> | undefined;
  const transient = e?.code === 'NETWORK' || (typeof e?.status === 'number' && e.status >= 500);
  return transient && failures < 2;
}

// One cache for the whole app. Pages that ask for the same thing within
// `staleTime` share one request, and going Back shows the previous page at once.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      retry: shouldRetry,
      retryDelay: (attempt) => Math.min(500 * 2 ** attempt, 4000),
      // Refetching whenever the window gains focus would flash skeletons while
      // someone is reading; the Hub's data changes when a push lands, not by the second.
      refetchOnWindowFocus: false,
    },
  },
});

// useAsync runs one read. `name` and `deps` together identify it in the cache, so
// every distinct request needs its own name ("tree", "blob", ...) and every value
// the request depends on in `deps`.
export function useAsync<T>(name: string, fn: () => Promise<T>, deps: readonly unknown[]): AsyncState<T> {
  const q = useQuery<T, ApiError>({ queryKey: [name, ...deps], queryFn: fn });
  return {
    data: q.data,
    error: q.error ?? undefined,
    // `loading` means "nothing to show yet". A background refetch of data that is
    // already on screen does not blank the page.
    loading: q.isPending,
    retry: () => void q.refetch(),
  };
}
