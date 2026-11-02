export interface ListedRepo {
  name: string;
  visibility: string;
  state: string;
}

export type RepoSort = 'name-asc' | 'name-desc';

export const REPO_SORTS: { value: RepoSort; label: string }[] = [
  { value: 'name-asc', label: 'Name, A to Z' },
  { value: 'name-desc', label: 'Name, Z to A' },
];

export function parseSort(raw: string | null): RepoSort {
  return REPO_SORTS.some((s) => s.value === raw) ? (raw as RepoSort) : 'name-asc';
}

// filterAndSort keeps the repositories whose name contains `query` (case
// insensitive, surrounding spaces ignored) and orders them. It never changes the
// input array.
export function filterAndSort<T extends ListedRepo>(repos: readonly T[], query: string, sort: RepoSort): T[] {
  const q = query.trim().toLowerCase();
  const kept = q ? repos.filter((r) => r.name.toLowerCase().includes(q)) : repos.slice();
  kept.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }));
  if (sort === 'name-desc') kept.reverse();
  return kept;
}
