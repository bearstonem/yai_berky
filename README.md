# Helm - AI Agent Platform

> An autonomous AI agent platform with multi-agent orchestration, self-improvement, and a web GUI.

## What is Helm?

`Helm` is an AI agent platform that goes beyond a terminal assistant. It manages fleets of autonomous agents, creates and evolves its own tools, remembers across sessions, and can even modify its own code with safety rollbacks. Describe what you need in everyday language — Helm delegates, orchestrates, and delivers.

**Three modes** (press `tab` to cycle in the TUI):
- **Exec** `▶` — describe what you want, get a single command, confirm with `y`
- **Chat** `📡` — ask questions, get markdown-rendered answers
- **Agent** `🖖` — give a task, the AI autonomously runs commands, reads/writes files, creates tools, and iterates until it's done

**Three interfaces:**
- **TUI** — interactive terminal REPL with sub-agent delegation, escalation prompts, and full slash commands (`helm`)
- **Web GUI** — full dashboard with agent management, skills, themes, delegation flow visualization (`helm --gui`)
- **Pipe** — headless mode for scripts and CI (`helm --pipe -a "task"`)

## Features

### Multi-Agent Orchestration

Helm's primary agent automatically delegates tasks to specialized sub-agents:

- **Automatic delegation** — the primary agent identifies which sub-agent matches a task and delegates immediately
- **Agent creation on the fly** — if no agent matches, the primary creates one with a tailored system prompt and tool set
- **Collaborative delegation** — any agent can delegate to any other agent, with cycle detection preventing infinite loops
- **Parallel execution** — multiple agents run concurrently when the primary delegates to several at once
- **Escalation to user** — any agent can pause and ask the user a question, then resume with the answer
- **Agents as tools** — agent profiles appear as `agent_{id}` tools, assignable to other agents

### Skills — Self-Creating Tools

The agent can build its own reusable tools called **skills**:

- Executable scripts (bash, python, node, ruby) that receive JSON args on stdin
- Persist in `~/.config/helm/skills/` — available in all future sessions
- Agents can assign skills to other agents when creating them
- Create via the agent (`create_skill`), the web GUI, or the AI Builder chat

### Self-Improvement Loop

A heartbeat cycle where Helm autonomously evolves itself:

- **Prime Directive** — set a high-level mission (e.g. "Focus on DevOps automation") or leave empty for fully autonomous operation
- **Goals** — the agent creates, tracks, and updates goals in `~/.config/helm/goals/`
- Each cycle: reviews goals → audits skills/agents → creates or improves one capability → tests it → logs progress
- **Backup/restore safety** — full source backup before each cycle, auto-restore on failed builds
- **Restart** — `restart_helm` tool rebuilds and relaunches after code changes
- Configurable interval (default 5 minutes), real-time streaming in the GUI

### Web GUI (`helm --gui`)

A full dashboard at `http://localhost:6900`:

- **Chat** — streaming chat with think-block rendering and code highlighting
- **Primary Agent** — delegation flow panel, status chips, escalation UI
- **Sub-Agents** — create/edit/delete agent profiles with system prompts and tool assignments
- **Skills** — manage skills with an AI Builder assistant, filter by language
- **Sessions** — browse, resume, and delete conversation history
- **Settings** — editable config (provider, model, API key, temperature, permissions), provider cards
- **Themes** — 7 themes: Default, Matrix, Netrunner, Snow Crash, Neuromancer, Blade Runner, LCARS
- **Delegation Flow** — interactive side panel showing real-time agent tree with click-to-detail
- **Self-Improve** — start/stop the evolution loop, set prime directive, watch goals and progress

### Vector Memory

Local vector memory powered by SQLite + sqlite-vec:

- Indexes messages, skills, and sessions as 512-dim embeddings
- OpenAI `text-embedding-3-small` with Ollama `nomic-embed-text` fallback
- Relevant past conversations auto-injected into agent/chat prompts
- Check stats with `/memory`

### Multi-Provider Support

| Provider | Type | Default Model |
|---|---|---|
| [OpenAI](https://platform.openai.com/) | Cloud | `gpt-4o-mini` |
| [Anthropic Claude](https://console.anthropic.com/) | Cloud | `claude-sonnet-4-6` |
| [OpenRouter](https://openrouter.ai/) | Cloud (multi-model) | `openai/gpt-4o-mini` |
| [MiniMax](https://platform.minimax.io/) | Cloud | `MiniMax-M2.7` |
| [Ollama](https://ollama.com/) | Local | `llama3.2` |
| [llama.cpp](https://github.com/ggerganov/llama.cpp) | Local | `default` |
| [LM Studio](https://lmstudio.ai/) | Local | `default` |
| Custom (OpenAI-compatible) | Any | `default` |

Providers without OpenAI-style function calling automatically use prompt-based tool calling.

### Tool Integrations

- **ComfyUI** — generate images via a local or remote ComfyUI server
- **Webhook** — call arbitrary HTTP endpoints as agent tools

Set up with `/integrate`.

### Remote Mode

```shell
helm --remote user@192.168.1.81 check disk usage
```

Runs agent mode on a remote machine via SSH. All commands, file reads, and writes tunnel through SSH. No install needed on the remote host.

### Pipe Mode

```shell
helm --pipe -a "find all TODO comments"    # agent, stdout only
helm --pipe -c "explain goroutines"         # chat
helm --pipe -e "list docker containers"     # exec, just the command
echo "refactor this" | helm --pipe -a       # stdin
```

Headless mode for scripts and CI. Agent auto-executes tools. Stdout has the final answer, stderr has diagnostics.

## Quick Start

### Install from source

Requires [Go](https://go.dev/dl/) 1.21+ and a C compiler (CGO for sqlite-vec).

```shell
git clone https://github.com/bearstonem/helm.git && cd helm
./install-local.sh
```

Or manually:

```shell
CGO_ENABLED=1 go build -o helm .
mv helm ~/.local/bin/
```

### Usage

```shell
helm                           # interactive TUI
helm --gui                     # web GUI at localhost:6900
helm --gui --port 8080         # custom port
helm -a refactor the logging   # agent mode
helm -c what is a mutex        # chat mode
helm list all docker containers # exec mode
helm --pipe -a "deploy it"     # headless
helm --remote user@host task   # remote SSH
```

## Keyboard Shortcuts (TUI)

| Key | Action |
|---|---|
| `tab` | Switch modes (▶ exec / 📡 chat / 🖖 agent) |
| `↑` / `↓` | Navigate input history |
| `enter` | Submit |
| `alt+enter` / `ctrl+j` | Insert newline |
| `ctrl+v` | Paste from clipboard |
| `ctrl+h` | Help |
| `ctrl+s` | Edit settings |
| `ctrl+r` | Clear + reset history |
| `ctrl+l` | Clear (keep history) |
| `ctrl+c` | Exit or interrupt |

## Slash Commands (TUI)

| Command | Description |
|---|---|
| `/help` | Show help |
| `/clear` | Clear terminal |
| `/reset` | Clear + reset conversation |
| `/compact` | Compact history to save tokens |
| `/cost` | Show token usage |
| `/session [save\|load\|list]` | Manage sessions |
| `/mode [exec\|chat\|agent]` | Switch mode |
| `/model [provider/model] [--save]` | Switch model |
| `/yolo` | Toggle auto-execute |
| `/integrate` | Manage integrations |
| `/skill [list\|remove <name>]` | Manage skills |
| `/agent [select <id>\|clear]` | List, select, or clear agent profile |
| `/goals` | List self-improvement goals |
| `/memory` | Vector memory stats |
| `/diff` | Git diff |
| `/commit <message>` | Git commit |
| `/status` | Git status |
| `/log` | Git log |

## Agent Tools

| Tool | Description |
|---|---|
| `web_search` | Search the web via Brave Search API (built-in, requires `BRAVE_API_KEY`) |
| `run_command` | Execute shell commands (60s timeout) |
| `read_file` | Read file contents |
| `write_file` | Create or overwrite files |
| `edit_file` | Search-and-replace edits |
| `list_directory` | List directory contents |
| `search_files` | Regex search (like grep) |
| `find_files` | Glob file search |
| `create_skill` | Create a reusable skill |
| `list_skills` / `remove_skill` | Manage skills |
| `create_agent` | Create a sub-agent profile |
| `delegate_task` | Delegate work to a sub-agent |
| `escalate_to_user` | Ask the user a question |
| `list_goals` / `create_goal` / `update_goal` | Manage self-improvement goals |
| `restart_helm` | Rebuild and relaunch after code changes |

Plus: `agent_{id}` tools for each agent profile, `skill_{name}` tools for each skill.

## Configuration

Config file: `~/.config/helm.json`

```json
{
  "AI_PROVIDER": "openai",
  "AI_API_KEY": "your-api-key",
  "AI_MODEL": "gpt-4o-mini",
  "AI_BASE_URL": "",
  "AI_PROXY": "",
  "AI_TEMPERATURE": 0.2,
  "AI_MAX_TOKENS": 2000,
  "USER_DEFAULT_PROMPT_MODE": "exec",
  "USER_PREFERENCES": "",
  "USER_ALLOW_SUDO": false,
  "USER_AGENT_AUTO_EXECUTE": false,
  "USER_PERMISSION_MODE": "workspace-write"
}
```

Provider-specific API keys via env vars: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`, `MINIMAX_API_KEY`.

Project-level overrides: `.helm/settings.json` and `.helm/settings.local.json`.

### Permission Modes

| Mode | What's allowed |
|---|---|
| `read-only` | Read files, search, list |
| `workspace-write` (default) | Write/edit files, run commands, create skills/agents, delegate |
| `full-access` | Everything including `restart_helm` |

## Data Storage

```
~/.config/helm.json          # main config
~/.config/helm/
  skills/{name}/             # skill manifests + scripts
  agents/{id}.json           # agent profiles
  sessions/{id}.json         # conversation sessions
  goals/{id}.json            # self-improvement goals
  memory.db                  # vector memory (sqlite-vec)
  backups/                   # source backups for self-improvement
    manifest.json            # backup tracking
    restart.sh               # auto-generated restart script
    {timestamp}/             # timestamped source snapshots
```

## Building

Requires CGO for sqlite-vec:

```shell
sudo apt-get install -y libsqlite3-dev  # Debian/Ubuntu
CGO_ENABLED=1 go build -o helm .
```

## Thanks

Originally forked from [ekkinox/yai](https://github.com/ekkinox/yai). Thanks to [@K-arch27](https://github.com/K-arch27) for the original name suggestion.
