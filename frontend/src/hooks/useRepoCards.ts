import { useAsync } from './useAsync';
import { api } from '../api/client';
import { ListedRepo } from '../utils/repoList';

// useRepoList fetches only the list of repositories. Each card then loads its own
// details when it is drawn (see RepoCard), through the shared cache, so a long
// list does not send two requests per repository at the moment the page opens.
export function useRepoList() {
  return useAsync<ListedRepo[]>('repo-list', async () => (await api.listRepos()).repositories, []);
}
