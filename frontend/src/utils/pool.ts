// mapLimit runs `fn` over `items` with at most `limit` calls in flight at once,
// and returns the results in the original order. Firing one request per item
// all at once would flood the browser (which allows only a handful of parallel
// connections to one host) and the server when a list is long.
export async function mapLimit<T, R>(items: readonly T[], limit: number, fn: (item: T, index: number) => Promise<R>): Promise<R[]> {
  const results = new Array<R>(items.length);
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      results[i] = await fn(items[i], i);
    }
  };
  await Promise.all(Array.from({ length: Math.min(Math.max(limit, 1), items.length) }, worker));
  return results;
}
