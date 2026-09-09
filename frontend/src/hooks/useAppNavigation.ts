import { useLocation, useNavigate } from 'react-router-dom';
import { RepoSection } from '../routes';

// useAppNavigation derives the current repository and section from the URL. Repos
// are addressed by name only (no owner namespace). It exposes `go` (react-router
// navigate) so chrome can move around without hardcoded, single-repo routes.
export function useAppNavigation() {
  const location = useLocation();
  const navigate = useNavigate();

  const path = location.pathname;
  const m = path.match(/^\/r\/([^/]+)(?:\/([^/]+))?/);
  const repo = m ? decodeURIComponent(m[1]) : '';
  const seg = (m && m[2]) || '';
  const inRepo = !!repo;
  const isLanding = path === '/';

  let section: RepoSection = 'code';
  if (seg === 'commits' || seg === 'commit') section = 'commits';
  else if (seg === 'pulls') section = 'pulls';
  else if (seg === 'issues') section = 'issues';
  else if (seg === 'search') section = 'search';

  return { repo, section, inRepo, isLanding, go: navigate };
}
