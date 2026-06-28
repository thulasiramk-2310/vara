VARA RFC: 0010
Title: Configuration System
Status: Draft
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the configuration format and resolution order for VARA. Configuration dictates user identity, repository behavior, and AI integration settings.

# 2. Format
VARA uses a standard, human-readable INI format (identical to Git for muscle-memory compatibility).
```ini
[user]
    name = Thulasiram K
    email = thulasiram@example.com

[core]
    compression = zstd
    autosnapshot = true

[ai]
    provider = ollama
    model = qwen3-coder
```

# 3. Resolution Cascade
Settings are resolved in the following order (highest precedence first):
1. **Command Line Flags:** e.g., `--ai-provider=openai`
2. **Environment Variables:** e.g., `VARA_AUTHOR_NAME`
3. **Repository Local:** `.vara/config`
4. **User Global:** `~/.varaconfig`
5. **System Wide:** `/etc/varaconfig`

# 4. Standard Keys
* `core.compression`: Deflate vs Zstd.
* `core.editor`: Default text editor.
* `core.autosnapshot`: Boolean (Default: true) to enable Layer 3 recovery.
* `ai.provider`: The LLM engine to use for `vara explain` or `vara commit`.
