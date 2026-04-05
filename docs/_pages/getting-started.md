---
title: "Getting started"
classes: wide
permalink: /getting-started/
---

## What is Helm?

`Helm` is an AI agent platform for your terminal. It manages autonomous agents, creates reusable tools, remembers across sessions, and provides a web GUI for managing everything.

It is already aware of your operating system, shell, home directory, and preferred editor.

## Installation

### From source (recommended)

Requires [Go](https://go.dev/dl/) 1.21+ and a C compiler (CGO for sqlite-vec).

```shell
git clone https://github.com/bearstonem/helm.git && cd helm
./install-local.sh
```

Or manually:

```shell
sudo apt-get install -y libsqlite3-dev  # Debian/Ubuntu
CGO_ENABLED=1 go build -o helm .
mv helm ~/.local/bin/
```

## Configuration

At first run, Helm will ask you to choose a provider and enter your API key. The config is saved to `~/.config/helm.json`:

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

### Supported Providers

| Provider | `AI_PROVIDER` | Needs API Key |
|---|---|---|
| OpenAI | `openai` | Yes (`OPENAI_API_KEY`) |
| Anthropic Claude | `anthropic` | Yes (`ANTHROPIC_API_KEY`) |
| OpenRouter | `openrouter` | Yes (`OPENROUTER_API_KEY`) |
| MiniMax | `minimax` | Yes (`MINIMAX_API_KEY`) |
| Ollama | `ollama` | No (local) |
| llama.cpp | `llamacpp` | No (local) |
| LM Studio | `lmstudio` | No (local) |
| Custom | `custom` | Depends |

### Editing Settings

- In the TUI: press `ctrl+s` to open settings in your editor (hot-reloads on save)
- In the Web GUI: Settings page with editable form
- At runtime: `/model provider/model --save` to switch and persist

### Preferences

Use `USER_PREFERENCES` for natural-language customization:

```json
{
  "USER_PREFERENCES": "Always explain what commands do before running them. Prefer Python over Bash for scripting."
}
```

### Permission Modes

`USER_PERMISSION_MODE` controls what agents can do:

| Mode | Allowed |
|---|---|
| `read-only` | Read files, search, list directories |
| `workspace-write` (default) | Write files, run commands, create skills/agents, delegate |
| `full-access` | Everything including `restart_helm` |

## Quick Start

```shell
helm                           # interactive TUI
helm --gui                     # web GUI at localhost:6900
helm -a refactor the logging   # agent mode (one-shot)
helm -c what is a mutex        # chat mode (one-shot)
helm --pipe -a "deploy it"     # headless for scripts
helm --remote user@host task   # remote SSH agent
```
