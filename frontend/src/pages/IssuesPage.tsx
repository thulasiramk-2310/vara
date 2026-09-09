import React from 'react';
import { StubPage } from '../components/common/StubPage';

// Issues are part of the v0.5 collaboration layer. The v0.4 backend has no issue
// store, so this is an honest stub rather than mock issue rows.
export const IssuesPage: React.FC = () => (
  <StubPage
    section="Issues"
    icon="issue-opened"
    blurb="Track bugs, ideas, and tasks. Issues arrive with the v0.5 collaboration layer — the v0.4 backend serves reads only."
  />
);
