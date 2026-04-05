---
title: "Usage"
classes: wide
permalink: /usage/
---

## Interfaces

Helm has three interfaces:

### TUI (Terminal)

Interactive REPL with three modes — press `tab` to cycle:

- `▶ exec` — describe what you want, get a command, confirm with `y`
- `📡 chat` — ask questions, get markdown answers
- `🖖 agent` — give a task, the AI runs commands autonomously

```shell
helm                            # interactive REPL
helm list docker containers     # exec mode (one-shot)
helm -e show disk usage         # force exec mode
helm -c explain goroutines      # force chat mode
helm -a refactor the logging    # force agent mode
```

Pipe input is supported:

```shell
cat error.log | helm -c explain what is wrong
cat script.go | helm -c generate unit tests
```

### Web GUI

Full dashboard with agent management, skills, themes, and self-improvement:

```shell
helm --gui                      # default port 6900
helm --gui --port 8080          # custom port
```

Pages: Chat, Primary Agent, Sub-Agents, Skills, Sessions, Settings

### Pipe Mode (Headless)

For scripts, CI, and other AI agents. No TUI, plain text output:

```shell
helm --pipe -a "find all TODOs"           # agent
helm --pipe -c "explain what a mutex is"  # chat
helm --pipe -e "compress all PNGs"        # exec (just the command)
echo "deploy this" | helm --pipe -a       # stdin
```

### Remote Mode (SSH)

Run agent mode on a remote machine via SSH:

```shell
helm --remote user@192.168.1.81 check disk usage
helm --remote user@host                    # interactive
```

## Keyboard Shortcuts (TUI)

| Key | Action |
|---|---|
| `tab` | Switch modes (▶ exec / 📡 chat / 🖖 agent) |
| `↑` / `↓` | Navigate input history |
| `enter` | Submit |
| `alt+enter` / `ctrl+j` | Insert newline |
| `ctrl+v` | Paste |
| `ctrl+h` | Help |
| `ctrl+s` | Edit settings |
| `ctrl+r` | Clear + reset history |
| `ctrl+l` | Clear (keep history) |
| `ctrl+c` | Exit or interrupt |

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show help |
| `/clear` | Clear terminal |
| `/reset` | Clear + reset conversation |
| `/compact` | Compact history to save tokens |
| `/cost` | Token usage and cost estimate |
| `/session` | Manage sessions (save/load/list) |
| `/mode` | Switch prompt mode |
| `/model` | Switch provider/model at runtime |
| `/yolo` | Toggle auto-execute for agent |
| `/integrate` | Manage tool integrations |
| `/skill` | Manage agent-created skills |
| `/agent` | List, select, or clear agent profiles |
| `/goals` | List self-improvement goals |
| `/memory` | Vector memory stats |
| `/diff` `/commit` `/status` `/log` | Git operations |

## Agent Mode

The agent autonomously completes multi-step tasks using tools:

- `run_command` — shell commands (60s timeout)
- `read_file` / `write_file` / `edit_file` — file operations
- `list_directory` / `search_files` / `find_files` — exploration
- `create_skill` / `create_agent` — self-extending capabilities
- `delegate_task` — delegate to specialized sub-agents
- `escalate_to_user` — pause and ask you a question
- `list_goals` / `create_goal` / `update_goal` — self-improvement
- `restart_helm` — rebuild after code changes (full-access mode)

Sub-agents run in parallel and report back. Any agent can delegate to any other agent. The primary agent creates new agents and skills on the fly when needed.
