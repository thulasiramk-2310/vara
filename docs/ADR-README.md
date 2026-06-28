# Architecture Decision Records (ADRs)

While RFCs define the *what* and the *how* of the VARA protocol, ADRs define the *why*.
They capture the historical context and reasoning behind major engineering decisions.
This is a pattern used in mature engineering organizations to make onboarding and
rationale clear without cluttering the rigid protocol specs.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-0001](ADR/ADR-0001-TRANSACTIONAL-REPOSITORY.md) | Transactional Repository Design | Accepted |
| [ADR-0002](ADR/ADR-0002-SNAPSHOTS-NOT-COMMITS.md) | Snapshots Are Not Commits | Accepted |
| [ADR-0003](ADR/ADR-0003-HIERARCHICAL-LOCKS.md) | Hierarchical Lock Ordering | Accepted |
| [ADR-0004](ADR/ADR-0004-INDEX-AUTHORITATIVE.md) | The Index Is Authoritative for Staging | Accepted |
| [ADR-0005](ADR/ADR-0005-TREE-BASED-MERGE.md) | Tree-Based Merge Instead of Patch-Based Merge | Accepted |
| [ADR-0006](ADR/ADR-0006-GRAPH-INDEX-DERIVED-STATE.md) | The Commit Graph Index Is Derived State | Accepted |
| [ADR-0007](ADR/ADR-0007-OBJECT-IDENTITY-INCLUDES-HEADER.md) | Object Identity Includes the Serialized Header | Accepted |

## Format

Each ADR contains:

- **Context** — the problem or question that needed to be answered
- **Decision** — the choice made and which RFC it is codified in
- **Consequences** — what the decision enables and what it costs
- **Alternatives Considered** — the roads not taken and why
