VARA RFC: 0011
Title: AI Provider Integration
Status: Draft
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0005, RFC-0007, RFC-0010
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
VARA is an AI-native VCS. AI is not a bolted-on wrapper script; it is deeply integrated into the repository data structures. This RFC defines how AI engines interact with the Commit Graph and Index.

# 2. Provider Abstraction
VARA interacts with AI through a strict `LLMProvider` interface, allowing users to swap backends seamlessly via configuration (`ai.provider`).
```go
type LLMProvider interface {
    GenerateCommitMessage(diff string, context *CommitGraphContext) (string, error)
    ExplainHistory(commits []*Commit) (string, error)
    ResolveConflict(conflict *ConflictMarker) (string, error)
}
```

# 3. Context Building
When VARA queries the AI, it provides structured deterministic context, preventing hallucinations:
* **The Index:** AI reads the exact staging area to understand what is being committed.
* **The Graph:** AI receives the topological history (Generation N to Generation N-5) to understand the trajectory of the branch.
* **The Diff:** Granular file-level changes.

# 4. Privacy and Opt-In
AI operations that send code over the network (e.g., to OpenAI or Anthropic) require explicit user opt-in per repository. Local providers (e.g., Ollama) can run without network warnings.
