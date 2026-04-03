package ui

import (
	"fmt"
	"strings"

	"github.com/ekkinox/yai/config"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

const (
	exec_color    = "#ffa657"
	config_color  = "#ffffff"
	chat_color    = "#66b3ff"
	agent_color   = "#bb86fc"
	help_color    = "#aaaaaa"
	error_color   = "#cc3333"
	warning_color = "#ffcc00"
	success_color = "#46b946"
	dim_color     = "#888888"
)

type Renderer struct {
	contentRenderer *glamour.TermRenderer
	successRenderer lipgloss.Style
	warningRenderer lipgloss.Style
	errorRenderer   lipgloss.Style
	helpRenderer    lipgloss.Style
	agentRenderer   lipgloss.Style
	dimRenderer     lipgloss.Style
}

func NewRenderer(options ...glamour.TermRendererOption) *Renderer {
	contentRenderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return nil
	}

	successRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(success_color))
	warningRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(warning_color))
	errorRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(error_color))
	helpRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(help_color)).Italic(true)
	agentRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(agent_color))
	dimRenderer := lipgloss.NewStyle().Foreground(lipgloss.Color(dim_color)).Italic(true)

	return &Renderer{
		contentRenderer: contentRenderer,
		successRenderer: successRenderer,
		warningRenderer: warningRenderer,
		errorRenderer:   errorRenderer,
		helpRenderer:    helpRenderer,
		agentRenderer:   agentRenderer,
		dimRenderer:     dimRenderer,
	}
}

func (r *Renderer) RenderContent(in string) string {
	out, _ := r.contentRenderer.Render(in)
	return out
}

func (r *Renderer) RenderSuccess(in string) string {
	return r.successRenderer.Render(in)
}

func (r *Renderer) RenderWarning(in string) string {
	return r.warningRenderer.Render(in)
}

func (r *Renderer) RenderError(in string) string {
	return r.errorRenderer.Render(in)
}

func (r *Renderer) RenderHelp(in string) string {
	return r.helpRenderer.Render(in)
}

func (r *Renderer) RenderConfigMessage() string {
	welcome := "Welcome! 👋  \n\n"
	welcome += "I cannot find a configuration file. Let's set one up.\n\n"
	welcome += "**Choose a provider** (enter number):\n\n"

	providers := config.ProviderList()
	for i, p := range providers {
		name := config.ProviderDisplayNames[p]
		welcome += fmt.Sprintf("  `%d` - %s\n", i+1, name)
	}

	return welcome
}

func (r *Renderer) RenderAPIKeyMessage(provider string) string {
	name := config.ProviderDisplayNames[provider]
	needsKey := config.ProviderNeedsAPIKey(provider)

	if !needsKey {
		return fmt.Sprintf("**%s** selected (no API key needed).\n\n", name)
	}

	var urlHint string
	switch provider {
	case config.ProviderOpenAI:
		urlHint = "Get one at https://platform.openai.com/api-keys"
	case config.ProviderAnthropic:
		urlHint = "Get one at https://console.anthropic.com/settings/keys"
	case config.ProviderOpenRouter:
		urlHint = "Get one at https://openrouter.ai/settings/keys"
	case config.ProviderMiniMax:
		urlHint = "Get one at https://platform.minimax.io/"
	default:
		urlHint = "Enter your API key"
	}

	return fmt.Sprintf("**%s** selected.\n\nPlease enter your API key. %s\n", name, urlHint)
}

func (r *Renderer) RenderBaseURLMessage(provider string) string {
	return fmt.Sprintf("Enter base URL for **%s** (or press Enter to skip):\n",
		config.ProviderDisplayNames[provider])
}

func (r *Renderer) RenderToolCall(name string, args string) string {
	return r.agentRenderer.Render(fmt.Sprintf("  [tool] %s", name)) + "\n" +
		r.dimRenderer.Render(fmt.Sprintf("  %s", args)) + "\n"
}

func (r *Renderer) RenderToolResult(output string, exitCode int) string {
	maxPreview := 500
	preview := output
	if len(preview) > maxPreview {
		preview = preview[:maxPreview] + "\n  ... [truncated]"
	}

	var badge string
	if strings.HasPrefix(output, "error:") || strings.HasPrefix(output, "error ") {
		badge = r.errorRenderer.Render("[error]")
	} else if strings.Contains(output, "exit_code: ") {
		badge = r.successRenderer.Render(fmt.Sprintf("[exit %d]", exitCode))
		if exitCode != 0 {
			badge = r.errorRenderer.Render(fmt.Sprintf("[exit %d]", exitCode))
		}
	} else {
		badge = r.successRenderer.Render("[ok]")
	}

	return fmt.Sprintf("  %s\n%s\n", badge, r.dimRenderer.Render(preview))
}

func (r *Renderer) RenderAgentThinking(text string) string {
	return r.dimRenderer.Render(fmt.Sprintf("  %s", text)) + "\n"
}

func (r *Renderer) RenderRemoteInfo(host string, hostname string, os string) string {
	info := fmt.Sprintf("**Remote:** `%s`", host)
	if hostname != "" {
		info += fmt.Sprintf(" (%s", hostname)
		if os != "" {
			info += fmt.Sprintf(", %s", os)
		}
		info += ")"
	}
	return info
}

func (r *Renderer) RenderHelpMessage() string {
	help := "**Help**\n"
	help += "- `↑`/`↓` : navigate in history\n"
	help += "- `tab`   : switch between `🚀 exec`, `💬 chat`, and `🤖 agent` prompt modes\n"
	help += "- `ctrl+h`: show help\n"
	help += "- `ctrl+s`: edit settings\n"
	help += "- `ctrl+r`: clear terminal and reset discussion history\n"
	help += "- `ctrl+l`: clear terminal but keep discussion history\n"
	help += "- `ctrl+c`: exit or interrupt command execution\n\n"
	help += "**Slash commands:** `/help`, `/clear`, `/reset`, `/compact`, `/cost`, `/session`, `/mode`, `/model`, `/yolo`, `/diff`, `/commit`, `/status`, `/log`\n\n"
	help += "**Agent mode:** the AI autonomously runs commands to complete tasks.\n"
	help += "Use `/yolo` to toggle auto-execution at runtime, or set `USER_AGENT_AUTO_EXECUTE` in settings (`ctrl+s`).\n\n"
	help += "**Permissions:** set `USER_PERMISSION_MODE` to `read-only`, `workspace-write` (default), or `full-access`.\n"

	return help
}

func (r *Renderer) RenderProviderInfo(cfg config.AiConfig) string {
	name := config.ProviderDisplayNames[cfg.GetProvider()]
	if name == "" {
		name = cfg.GetProvider()
	}

	info := fmt.Sprintf("**Provider:** %s | **Model:** `%s`", name, cfg.GetModel())

	baseURL := cfg.GetEffectiveBaseURL()
	if baseURL != "" && !strings.HasPrefix(baseURL, "https://api.openai.com") {
		info += fmt.Sprintf(" | **URL:** `%s`", baseURL)
	}

	return info
}
