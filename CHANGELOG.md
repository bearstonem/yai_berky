# Changelog

## 2.0.0 — Helm (renamed from Yai)

### Added — Multi-Agent Orchestration
- Primary agent as orchestrator that delegates to specialized sub-agents
- `create_agent` tool — agents create other agents on the fly
- `delegate_task` tool with context passing for shared state
- Collaborative delegation — any agent can delegate to any other (cycle detection prevents loops)
- Parallel execution of multiple delegations via goroutines
- `escalate_to_user` tool — agents pause and ask user for input
- Agents as tools — agent profiles appear as `agent_{id}` callable tools, assignable to other agents
- Delegation chain tracking (max depth 6) replaces hard depth limits

### Added — Self-Improvement Loop
- Autonomous heartbeat cycle with configurable interval (default 5 min)
- Prime Directive — user-set mission that guides all improvement, or fully autonomous mode
- Goals system — `list_goals`, `create_goal`, `update_goal` tools
- Goals stored as JSON in `~/.config/helm/goals/`
- Backup/restore safety — full source backup before each cycle
- `restart_helm` tool — rebuilds and relaunches after code changes
- Auto-restore from backup if build fails
- Real-time streaming of improvement progress via SSE

### Added — Web GUI (`--gui`)
- Full dashboard: Chat, Primary Agent, Sub-Agents, Skills, Sessions, Settings
- AI Builder — chat-based assistant for creating skills and agents
- Delegation flow panel — interactive tree visualization of agent activity
- Session resume — reload and continue past conversations
- Editable settings with provider selection and save
- 7 themes: Default, Matrix, Netrunner, Snow Crash, Neuromancer, Blade Runner, LCARS
- Theme-specific iconography
- Filter/search on skills and sessions pages
- Self-improvement panel with prime directive, goals, and event log

### Added — Skills System
- Agent-created reusable tools (bash, python, node, ruby scripts)
- `create_skill`, `list_skills`, `remove_skill` tools
- Skills persist in `~/.config/helm/skills/`
- Skill editing via GUI
- Double-encoded JSON parameter handling

### Added — Vector Memory
- SQLite + sqlite-vec for conversation recall
- 512-dim embeddings via OpenAI text-embedding-3-small / Ollama nomic-embed-text
- Indexes messages, skills, sessions
- Auto-injected into agent/chat prompts

### Added — Provider Support
- Multi-provider: OpenAI, Anthropic, OpenRouter, MiniMax, Ollama, llama.cpp, LM Studio, Custom
- Prompt-based tool calling fallback for providers without function calling
- `max_tokens` vs `max_completion_tokens` provider compatibility
- Provider-specific API key env vars
- Runtime model switching with `/model` and `--save`

### Added — Modes & Interfaces
- Pipe mode (`--pipe`) — headless operation for scripts/CI
- Remote mode (`--remote user@host`) — SSH agent mode
- Multiline input (Alt+Enter / Ctrl+J)
- Permission modes: read-only, workspace-write, full-access
- Tool integrations: ComfyUI, Webhook
- Hook system for pre/post tool use
- Session management with save/load/list/resume

### Changed
- Renamed from Yai to Helm
- Module path: `github.com/bearstonem/helm`
- Config: `~/.config/helm.json`, data: `~/.config/helm/`
- Star Trek-inspired iconography
- Auto-migration from yai config/data on install

---

## 0.6.0

### Changed
- Changed project name from Yo to Yai

## 0.5.0
### Added
- Display help when starting REPL mode

## 0.4.0
### Added
- Configuration for OpenAI API model (default gpt-3.5-turbo)

## 0.3.0
### Added
- Configuration for OpenAI API max-tokens (default 1000)

## 0.2.0
### Added
- Support for pipe input

## 0.1.0
### Added
- Exec prompt mode
- Chat prompt mode
