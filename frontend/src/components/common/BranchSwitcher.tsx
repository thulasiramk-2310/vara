import React from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { GitBranch } from 'lucide-react';
import { api } from '../../api/client';
import { useAsync } from '../../hooks/useAsync';

// useRepoRef reads the branch (or commit) the page is showing from the address,
// as `?ref=`. Empty means the repository's default branch.
export function useRepoRef(): string {
  const [params] = useSearchParams();
  return params.get('ref') || '';
}

// BranchSwitcher lists the repository's branches in a native <select>, so it is
// keyboard and screen-reader operable with no custom widget. Choosing one
// reloads the same page on that branch, and the choice lives in the URL, so it
// can be shared and survives a refresh.
export const BranchSwitcher: React.FC<{ repo: string; to: (ref: string) => string }> = ({ repo, to }) => {
  const navigate = useNavigate();
  const ref = useRepoRef();
  const branches = useAsync(() => api.branches(repo), [repo]);

  const list = branches.data?.branches ?? [];
  // Nothing to switch between until the list arrives, or when there is only one.
  if (branches.loading || branches.error || list.length < 2) return null;

  const head = list.find((b) => b.is_head)?.name ?? list[0].name;
  // A ref that is not a branch name (a commit id) still has to show as selected.
  const known = list.some((b) => b.name === ref);

  return (
    <label style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--s-2)', fontSize: '13px', color: 'var(--dim)' }}>
      <GitBranch size={14} aria-hidden="true" />
      <span className="v-sr-only">Branch</span>
      <select
        value={ref || head}
        onChange={(e) => navigate(to(e.target.value === head ? '' : e.target.value))}
        style={{
          minHeight: 32,
          padding: '0 var(--s-3)',
          borderRadius: 6,
          border: '1px solid var(--border-strong)',
          background: 'var(--panel)',
          color: 'var(--text)',
          fontFamily: 'var(--font-mono)',
          fontSize: '13px',
        }}
      >
        {!known && ref && <option value={ref}>{ref.slice(0, 10)}</option>}
        {list.map((b) => (
          <option key={b.name} value={b.name}>
            {b.name}
          </option>
        ))}
      </select>
    </label>
  );
};
