# 🚀 Yai 💬 - AI powered terminal assistant

[![build](https://github.com/ekkinox/yai/actions/workflows/build.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/build.yml)
[![release](https://github.com/ekkinox/yai/actions/workflows/release.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/release.yml)
[![doc](https://github.com/ekkinox/yai/actions/workflows/doc.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/doc.yml)

> Unleash the power of artificial intelligence to streamline your command line experience.

![Intro](docs/_assets/intro.gif)

## What is Yai?

`Yai` (your AI) is an assistant for your terminal, using AI to build and run commands for you. You just need to describe them in your everyday language, it will take care of the rest.

You have any questions on random topics in mind? You can also ask `Yai`, and get the power of AI without leaving `/home`.

**Three modes:**
- **Exec** (`tab` to switch) -- describe what you want, get a single command, confirm with `y`
- **Chat** -- ask questions, get markdown answers
- **Agent** -- give a task, the AI autonomously runs commands, reads files, and iterates until it's done

It is already aware of your:
- operating system & distribution
- username, shell & home directory
- preferred editor

And you can also give any supplementary preferences to fine tune your experience.

## Supported Providers

Yai supports a wide range of AI providers and local LLM runtimes:

| Provider | Type | Default Model |
|---|---|---|
| [OpenAI](https://platform.openai.com/) | Cloud | `gpt-4o-mini` |
| [Anthropic Claude](https://console.anthropic.com/) | Cloud | `claude-sonnet-4-6` |
| [OpenRouter](https://openrouter.ai/) | Cloud (multi-model) | `openai/gpt-4o-mini` |
| [MiniMax](https://platform.minimax.io/) | Cloud | `MiniMax-M2` |
| [Ollama](https://ollama.com/) | Local | `llama3.2` |
| [llama.cpp](https://github.com/ggerganov/llama.cpp) | Local | `default` |
| [LM Studio](https://lmstudio.ai/) | Local | `default` |
| Custom (OpenAI-compatible) | Any | `default` |

Local providers (Ollama, llama.cpp, LM Studio) do not require an API key.

## Documentation

A complete documentation is available at [https://ekkinox.github.io/yai/](https://ekkinox.github.io/yai/).

## Quick start

### Install from source (recommended for development)

Clone the repo and run the local install script. Requires [Go](https://go.dev/dl/) 1.21+.

```shell
git clone https://github.com/ekkinox/yai.git && cd yai
./install-local.sh
```

This builds the binary, installs it to `~/.local/bin`, and adds it to your PATH automatically. You can customize the install directory with `INSTALL_DIR`:

```shell
INSTALL_DIR=/usr/local/bin sudo ./install-local.sh
```

### Install from release

To install a pre-built release binary:

```shell
curl -sS https://raw.githubusercontent.com/ekkinox/yai/main/install.sh | bash
```

At first run, it will ask you to choose a provider and enter your API key (if needed), then create the configuration file in `~/.config/yai.json`.

See [documentation](https://ekkinox.github.io/yai/getting-started/#configuration) for more information.

### Configuration

The configuration file lives at `~/.config/yai.json`. You can edit it directly or press `ctrl+s` inside Yai.

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
  "USER_AGENT_AUTO_EXECUTE": false
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

Existing configs using the legacy `OPENAI_*` keys continue to work and are read as fallback values.

### Agent Mode

Agent mode lets the AI autonomously complete multi-step tasks. Press `tab` to cycle to the `🤖 agent` prompt, describe your task, and the AI will:

1. Plan what needs to be done
2. Run shell commands, read/write files as needed
3. Observe the output and iterate
4. Provide a summary when finished

**Available tools:** `run_command`, `read_file`, `write_file`, `list_directory`

By default, each tool call requires your confirmation (`y/N`). Set `USER_AGENT_AUTO_EXECUTE` to `true` to let the agent run without asking.

You can also use agent mode from the command line:

```shell
yai -a find all TODO comments in this project
```

#### Remote Mode

Use `--remote` to run agent mode on a remote machine via SSH. Yai stays on your local machine -- all commands, file reads, and writes tunnel through SSH automatically. No install needed on the remote host.

```shell
# One-shot task on a remote host
yai --remote user@192.168.1.81 check disk usage

# Interactive REPL on a remote host
yai --remote user@192.168.1.81
```

The `--remote` flag implies agent mode, so `-a` is not required. On startup, Yai probes the remote system (OS, shell, home directory) and includes this context in the prompt so the AI generates correct commands for the target.

**Requirements:** Key-based SSH authentication must be configured for the target host (Yai uses `BatchMode=yes` and will not prompt for passwords).

**Safety:** Commands time out after 60 seconds. Output is capped at 50KB to avoid flooding the conversation. Press `ctrl+c` at any time to interrupt the agent. Sudo rules from `USER_ALLOW_SUDO` are enforced.

### Sudo Support

By default, Yai will not generate commands that use `sudo`. To enable elevated-privilege commands, set `USER_ALLOW_SUDO` to `true` in your config (`ctrl+s` inside Yai).

When enabled:
- The AI will use `sudo` when a task requires root access (installing packages, managing services, editing system files, etc.)
- A `[sudo]` warning is shown before the confirmation prompt so you always know before executing
- Sudo credentials are validated upfront via `sudo -v` before the actual command runs

## Thanks

Thanks to [@K-arch27](https://github.com/K-arch27) for the `Yai` name suggestion.
