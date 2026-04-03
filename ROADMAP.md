# 🚀 Yai Roadmap: Claude Code Parity

This document tracks the implementation of specialized software engineering features to bring Yai closer to the capabilities of Claude Code.

## 🛠️ Version Control Integration
- [ ] **Native Git Workflows**: Implement high-level commands for common Git operations.
  - [ ] Automated commit message generation based on diffs.
  - [ ] PR description drafting using `gh` CLI integration.
  - [ ] Branch intent analysis using `git log`.
- [ ] **Git-aware Context**: Automatically include current branch and status in the agent's system prompt.

## 🔍 Codebase Intelligence
- [ ] **Semantic Search/Indexing**: Move beyond basic `grep`.
  - [ ] Implement a symbol map (functions, classes, variables) for faster navigation.
  - [ ] Integration with local embeddings or a lightweight index for "where is X" queries.
- [ ] **Smart File Discovery**: Improved heuristics for finding relevant files based on the task description.

## ✍️ Intelligent Editing
- [ ] **Patch-based Editing**: Implement a tool to modify specific lines or blocks instead of full file overwrites.
  - [ ] Support for `Search-and-Replace` blocks.
  - [ ] Integration of a diff-based update mechanism to reduce token usage and prevent data loss.

## 🧪 Test-Fix-Verify Loop
- [ ] **Automated Test Loop**: Create a specialized mode for iterative debugging.
  - [ ] Command to "Run tests until pass" with automatic fixing.
  - [ ] Integration with common test runners (npm, pytest, go test) to parse failures and feed them back to the agent.

## 🧠 Project Context & Memory
- [ ] **Persistent Project Memory**: Implement a local storage mechanism for project-specific knowledge.
  - [ ] Storage for architectural decisions, naming conventions, and "gotchas".
  - [ ] Ability for the agent to read/write to this knowledge base across different sessions.