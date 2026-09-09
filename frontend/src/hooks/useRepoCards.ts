import { useAsync } from './useAsync';
import { api, RepoSummary } from '../api/client';
import { inferLanguage, Lang } from '../utils/repo';

// A repository enriched with the real data the read API can answer: its summary
// (commits, branches, default branch, last commit) and a language inferred from the
// actual root file listing. Best-effort per field — a repo still renders if some
// call fails.
export interface RepoCardData {
  name: string;
  visibility: string;
  state: string;
  summary?: RepoSummary;
  language?: Lang;
}

async function load(): Promise<RepoCardData[]> {
  const { repositories } = await api.listRepos();
  return Promise.all(
    repositories.map(async (r) => {
      const [sum, tree] = await Promise.allSettled([api.summary(r.name), api.tree(r.name, '')]);
      const summary = sum.status === 'fulfilled' ? sum.value : undefined;
      const language =
        tree.status === 'fulfilled'
          ? inferLanguage(tree.value.entries.filter((e) => e.type !== 'dir').map((e) => e.name)) || undefined
          : undefined;
      return { name: r.name, visibility: r.visibility, state: r.state, summary, language };
    }),
  );
}

export function useRepoCards() {
  return useAsync<RepoCardData[]>(load, []);
}
