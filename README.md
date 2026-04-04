# Helm - AI Agent Platform

> Unleash the power of artificial intelligence to streamline your command line experience.

![Intro](docs/_assets/intro.gif)

## What is Helm?

`Helm` is an AI agent platform for your terminal. It builds and runs commands, manages autonomous agents, creates reusable skills, and remembers across sessions. Describe what you need in everyday language, and it takes care of the rest.

You have any questions on random topics in mind? You can also ask `Helm`, and get the power of AI without leaving `/home`.

**Three modes** (press `tab` to cycle):
- **Exec** `⚓` -- describe what you want, get a single command, confirm with `y`
- **Chat** `🧭` -- ask questions, get markdown-rendered answers
- **Agent** `⎈` -- give a task, the AI autonomously runs commands, reads/writes files, creates tools, and iterates until it's done

It is already aware of your:
- operating system & distribution
- username, shell & home directory
- preferred editor

And you can also give any supplementary preferences to fine tune your experience.

## Features

### Multi-Provider Support

Helm works with a wide range of cloud AI providers and local LLM runtimes through a unified OpenAI-compatible interface:

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

Local providers (Ollama, llama.cpp, LM Studio) do not require an API key.

Providers that don't support OpenAI-style function calling (Ollama, llama.cpp, LM Studio) automatically use a prompt-based tool calling fallback -- the agent describes tools in the system prompt and parses JSON responses from the model.

### Agent Mode

Agent mode lets the AI autonomously complete multi-step tasks. Press `tab` to cycle to the `⎈ agent` prompt, describe your task, and the AI will:

1. Plan what needs to be done
2. Run shell commands, read/write/edit files, search the codebase
3. Observe the output and iterate
4. Provide a summary when finished

**Agent tools:**

| Tool | Description |
|---|---|
| `run_command` | Execute shell commands via `bash -c` |
| `read_file` | Read file contents |
| `write_file` | Create or overwrite files (supports plain text, line arrays, and base64) |
| `edit_file` | Make targeted edits to existing files using search/replace |
| `list_directory` | List directory contents |
| `search_files` | Search file contents with regex patterns (ripgrep-style) |
| `find_files` | Find files by name/glob pattern |
| `create_skill` | Create a reusable skill (see Skills below) |
| `list_skills` | List all available skills |
| `remove_skill` | Remove a skill |

By default, each tool call requires your confirmation (`y/N`). Toggle auto-execution with `/yolo` at runtime, or set `USER_AGENT_AUTO_EXECUTE` to `true` in settings.

### Skills -- Agent-Created Tools

The agent can create its own reusable tools called **skills**. Ask the agent to build an integration -- like "add a skill to query the GitHub API" or "create a tool that converts images with ImageMagick" -- and it will:

1. Write an executable script (bash, python, node, or ruby)
2. Define the tool schema (name, description, parameters)
3. Save it as a persistent skill available in all future sessions

Skills are stored in `~/.config/helm/skills/` as a manifest + script pair. On startup, Helm loads all skills and displays them alongside the built-in tools.

Manage skills with the `/skill` slash command or let the agent handle it via `create_skill` / `remove_skill`.

### Vector Memory

Helm includes a local vector memory system powered by **SQLite + sqlite-vec** that gives the AI cross-session context recall.

**What gets indexed:**
- Conversation messages (user and assistant)
- Skills (name and description)
- Session summaries

**How it works:**
- Messages are embedded as 512-dimensional vectors using OpenAI `text-embedding-3-small` (with Ollama `nomic-embed-text` as a local fallback)
- When you start a new conversation, relevant past messages are automatically retrieved via KNN search and injected into the system prompt
- All indexing happens in the background -- no latency impact on your interactions

Check memory stats with `/memory`.

### Tool Integrations

Helm supports external tool integrations that extend the agent's capabilities:

- **ComfyUI** -- Generate images via a local or remote ComfyUI server
- **Webhook** -- Call arbitrary HTTP endpoints as agent tools

Set up integrations with `/integrate` and manage them interactively.

### Multiline Input

The input field supports multiline text:

- **Enter** -- submit your message
- **Alt+Enter** or **Ctrl+J** -- insert a newline
- **Ctrl+V** -- paste from clipboard (multiline supported)

### Pipe Mode (Non-Interactive)

Use `--pipe` for headless, non-interactive operation. This bypasses the TUI entirely -- input comes from command-line args, output goes to stdout as plain text. Agent mode auto-executes all tools without confirmation.

This makes helm usable as a tool by other AI agents, scripts, and CI pipelines.

```shell
# Agent mode -- auto-executes tools, prints final answer to stdout
helm --pipe -a "find all TODO comments in this project"

# Chat mode -- plain text response to stdout
helm --pipe -c "explain what a goroutine is"

# Exec mode -- prints just the command
helm --pipe -e "list docker containers"

# Pipe stdin
echo "refactor this function" | helm --pipe -a

# Use in scripts
COMMAND=$(helm --pipe -e "compress all png files in current dir")
echo "Would run: $COMMAND"
```

Diagnostic output (thinking, tool calls, results) goes to stderr, so stdout contains only the final answer. This means you can safely pipe or capture the output.

### Remote Mode

Use `--remote` to run agent mode on a remote machine via SSH. Helm stays on your local machine -- all commands, file reads, and writes tunnel through SSH automatically. No install needed on the remote host.

```shell
# One-shot task on a remote host
helm --remote user@192.168.1.81 check disk usage

# Interactive REPL on a remote host
helm --remote user@192.168.1.81
```

The `--remote` flag implies agent mode. On startup, Helm probes the remote system (OS, shell, home directory) and includes this context in the prompt so the AI generates correct commands for the target.

**Requirements:** Key-based SSH authentication must be configured for the target host (Helm uses `BatchMode=yes` and will not prompt for passwords).

### Permission Modes

Control what the agent is allowed to do with `USER_PERMISSION_MODE`:

| Mode | Description |
|---|---|
| `read-only` | Agent can only run commands and read files |
| `workspace-write` (default) | Agent can also write/edit files in the workspace and create skills |
| `full-access` | No restrictions on tool execution |

### Sudo Support

By default, Helm will not generate commands that use `sudo`. To enable elevated-privilege commands, set `USER_ALLOW_SUDO` to `true` in your config (`ctrl+s` inside Helm).

When enabled:
- The AI will use `sudo` when a task requires root access (installing packages, managing services, editing system files, etc.)
- A `[sudo]` warning is shown before the confirmation prompt so you always know before executing
- Sudo credentials are validated upfront via `sudo -v` before the actual command runs

## Quick Start

### Install from source

Clone the repo and build. Requires [Go](https://go.dev/dl/) 1.21+ and a C compiler (CGO is required for sqlite-vec).

```shell
git clone https://github.com/bearstonem/helm.git && cd helm
go build -o helm .
mv helm ~/.local/bin/   # or anywhere on your PATH
```

Or use the install script:

```shell
./install-local.sh
```

This builds the binary, installs it to `~/.local/bin`, and adds it to your PATH automatically. You can customize the install directory with `INSTALL_DIR`:

```shell
INSTALL_DIR=/usr/local/bin sudo ./install-local.sh
```

At first run, it will ask you to choose a provider and enter your API key (if needed), then create the configuration file in `~/.config/helm.json`.

### Usage

```shell
# Interactive REPL (default)
helm

# One-shot exec mode
helm list all docker containers

# One-shot chat mode
helm -c explain what a goroutine is

# One-shot agent mode
helm -a refactor the logging in this project

# Pipe mode (no TUI, plain text I/O, agent auto-executes)
helm --pipe -a "create a hello world web server"
helm --pipe -c "what is a mutex" > answer.txt

# Agent on a remote host
helm --remote user@host deploy the latest build
```

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `tab` | Switch between exec, chat, and agent modes |
| `↑` / `↓` | Navigate input history |
| `enter` | Submit input |
| `alt+enter` / `ctrl+j` | Insert newline (multiline input) |
| `ctrl+v` | Paste from clipboard |
| `ctrl+h` | Show help |
| `ctrl+s` | Edit settings |
| `ctrl+r` | Clear terminal and reset discussion history |
| `ctrl+l` | Clear terminal but keep discussion history |
| `ctrl+c` | Exit or interrupt agent |

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show help |
| `/clear` | Clear terminal |
| `/reset` | Clear terminal and reset conversation |
| `/compact` | Compact conversation history to save tokens |
| `/cost` | Show token usage and estimated cost |
| `/session [save\|load\|list]` | Manage conversation sessions |
| `/mode [exec\|chat\|agent]` | Switch prompt mode |
| `/model [provider/model] [--save]` | Switch model at runtime, optionally save as default |
| `/yolo` | Toggle auto-execute for agent tool calls |
| `/integrate` | Manage tool integrations |
| `/skill [list\|remove <name>]` | Manage agent-created skills |
| `/memory` | Show vector memory stats |
| `/diff` | Show git diff of working tree |
| `/commit <message>` | Stage all and commit |
| `/status` | Show git status |
| `/log` | Show recent git log |

## Configuration

The configuration file lives at `~/.config/helm.json`. You can edit it directly or press `ctrl+s` inside Helm.

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

| Key | Description |
|---|---|
| `AI_PROVIDER` | One of: `openai`, `anthropic`, `openrouter`, `minimax`, `ollama`, `llamacpp`, `lmstudio`, `custom` |
| `AI_API_KEY` | Your API key (not required for local providers) |
| `AI_MODEL` | Model name to use |
| `AI_BASE_URL` | Custom API base URL (auto-set for known providers, override for custom setups) |
| `AI_PROXY` | HTTP proxy URL |
| `AI_TEMPERATURE` | Sampling temperature (0-2) |
| `AI_MAX_TOKENS` | Maximum tokens in the response |
| `USER_DEFAULT_PROMPT_MODE` | Default mode: `exec`, `chat`, or `agent` |
| `USER_PREFERENCES` | Free-text preferences appended to the system prompt |
| `USER_ALLOW_SUDO` | Allow commands with `sudo` (default `false`) |
| `USER_AGENT_AUTO_EXECUTE` | Skip confirmation for each tool call in agent mode (default `false`) |
| `USER_PERMISSION_MODE` | Agent permission level: `read-only`, `workspace-write`, `full-access` |

**Environment variables:** Provider-specific API keys can be set via environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`, `MINIMAX_API_KEY`) and take precedence over the config file. Existing configs using legacy `OPENAI_*` keys continue to work.

## Building

Helm uses CGO for the sqlite-vec vector memory system. You'll need:

- Go 1.21+
- A C compiler (`gcc` or `clang`)
- SQLite development headers: `sudo apt-get install libsqlite3-dev` (Debian/Ubuntu)

```shell
CGO_ENABLED=1 go build -o helm .
```

## Thanks

Thanks to [@K-arch27](https://github.com/K-arch27) for the `Helm` name suggestion.
