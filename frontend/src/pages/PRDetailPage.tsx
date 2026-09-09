import React from 'react';
import { StubPage } from '../components/common/StubPage';

// A single pull request. Same story as PullsPage — no PR data exists in v0.4, so
// this deep link resolves to an honest stub instead of a mock thread.
export const PRDetailPage: React.FC = () => (
  <StubPage
    section="Pull request"
    icon="git-pull-request"
    blurb="Review a proposed change — its diff, checks, and discussion. Pull requests arrive with the v0.5 collaboration layer."
  />
);
