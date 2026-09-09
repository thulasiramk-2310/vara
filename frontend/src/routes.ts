// Central route builders. Repositories are addressed by name only — there is no
// owner/namespace segment, matching the v0.4 backend (repositories are keyed by
// immutable ID; owner namespaces are deferred to v0.5, RFC-0019 §14).

import { encPath } from './api/client';

const r = encodeURIComponent;

export const routes = {
  landing: '/',
  login: '/login',
  logout: '/logout',
  dashboard: '/dashboard',
  newRepo: '/new',

  // v0.5 stubs (clearly marked in the UI, no backend yet)
  explore: '/explore',
  notifications: '/notifications',
  profile: '/profile',
  settings: '/settings',

  repo: (repo: string) => `/r/${r(repo)}`,
  tree: (repo: string, path = '') => `/r/${r(repo)}/tree${path ? '/' + encPath(path) : ''}`,
  blob: (repo: string, path: string) => `/r/${r(repo)}/blob/${encPath(path)}`,
  commits: (repo: string) => `/r/${r(repo)}/commits`,
  commit: (repo: string, id: string) => `/r/${r(repo)}/commit/${r(id)}`,
  search: (repo: string) => `/r/${r(repo)}/search`,

  // repo-scoped v0.5 stubs
  pulls: (repo: string) => `/r/${r(repo)}/pulls`,
  pull: (repo: string, num: string | number) => `/r/${r(repo)}/pulls/${num}`,
  issues: (repo: string) => `/r/${r(repo)}/issues`,
};

export type RepoSection = 'code' | 'commits' | 'pulls' | 'issues' | 'search';
