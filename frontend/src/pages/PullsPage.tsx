import React from 'react';
import { StubPage } from '../components/common/StubPage';

// Pull requests are part of the v0.5 collaboration layer — the v0.4 backend is
// read-only, so there is no PR data to show. Render an honest stub rather than
// mock rows (no fake authors, counts, or states).
export const PullsPage: React.FC = () => (
  <StubPage
    section="Pull requests"
    icon="git-pull-request"
    blurb="Propose, review, and merge changes. Pull requests arrive with the v0.5 collaboration layer — the v0.4 backend serves reads only."
  />
);
