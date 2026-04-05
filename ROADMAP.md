# Helm Roadmap

## Completed

- Multi-provider support (OpenAI, Anthropic, OpenRouter, MiniMax, Ollama, llama.cpp, LM Studio)
- Agent mode with autonomous tool execution (run_command, read/write/edit_file, search, find)
- Skills system — agent-created reusable tools (bash, python, node, ruby)
- Vector memory — sqlite-vec conversation recall with embeddings
- Multi-agent orchestration — delegation, escalation, cycle detection, parallel execution
- Agents as callable tools — assign agents to other agents
- Self-improvement loop — goals, prime directive, backup/restore, restart
- Web GUI — dashboard with themes, AI builders, delegation flow, sessions
- Pipe mode (`--pipe`) and remote SSH mode (`--remote`)
- Permission modes, hooks, integrations (ComfyUI, Webhook)
- Session management with save/load/resume
- Multiline input, slash commands, model switching

## In Progress

- Self-improvement loop iteration quality
- Agent collaboration patterns (reviewer + builder workflows)
- Skill testing and reliability improvements

## Planned

### Agent Intelligence
- Long-term memory with semantic search across sessions
- Agent-to-agent direct messaging (beyond delegation)
- Shared workspaces between concurrent agents
- Agent performance metrics and auto-tuning
- Context window management and summarization

### Platform
- Plugin system for community-contributed skills
- OAuth/auth for web GUI (multi-user support)
- WebSocket for real-time GUI updates (replace SSE)
- Token budget per agent/cycle for cost control
- Streaming in all contexts (builders currently non-streaming)

### Developer Experience
- `helm init` for project scaffolding with HELM.md
- LSP integration for code-aware editing
- Test runner integration (npm, pytest, go test)
- Sandbox mode for safe experimentation
- CI/CD integration templates

### Distribution
- Pre-built binaries for Linux/macOS/Windows
- Docker image
- Homebrew formula
