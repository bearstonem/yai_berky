# 🚀 Yai 💬 - AI powered terminal assistant

[![build](https://github.com/ekkinox/yai/actions/workflows/build.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/build.yml)
[![release](https://github.com/ekkinox/yai/actions/workflows/release.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/release.yml)
[![doc](https://github.com/ekkinox/yai/actions/workflows/doc.yml/badge.svg)](https://github.com/ekkinox/yai/actions/workflows/doc.yml)

> Unleash the power of artificial intelligence to streamline your command line experience.

![Intro](docs/_assets/intro.gif)

## What is Yai?

`Yai` (your AI) is an assistant for your terminal, using AI to build and run commands for you. You just need to describe them in your everyday language, it will take care of the rest.

You have any questions on random topics in mind? You can also ask `Yai`, and get the power of AI without leaving `/home`.

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

To install `Yai`, simply run:

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
  "USER_PREFERENCES": ""
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
| `USER_DEFAULT_PROMPT_MODE` | Default mode: `exec` or `chat` |
| `USER_PREFERENCES` | Free-text preferences appended to the system prompt |

Existing configs using the legacy `OPENAI_*` keys continue to work and are read as fallback values.

## Thanks

Thanks to [@K-arch27](https://github.com/K-arch27) for the `Yai` name suggestion.
