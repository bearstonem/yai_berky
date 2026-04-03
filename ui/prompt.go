package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	exec_icon          = "🚀 > "
	exec_placeholder   = "Execute something..."
	config_icon        = "🔒 > "
	config_placeholder = "Enter your API key..."
	chat_icon          = "💬 > "
	chat_placeholder   = "Ask me something..."
	agent_icon         = "🤖 > "
	agent_placeholder  = "Give me a task..."
)

type Prompt struct {
	mode       PromptMode
	input      textinput.Model
	remoteHost string
	modelLabel string
}

func NewPrompt(mode PromptMode) *Prompt {
	input := textinput.New()
	input.Placeholder = getPromptPlaceholder(mode)
	input.TextStyle = getPromptStyle(mode)
	input.Prompt = getPromptIcon(mode)

	if mode == ConfigPromptMode {
		input.EchoMode = textinput.EchoPassword
	}

	input.Focus()

	return &Prompt{
		mode:  mode,
		input: input,
	}
}

func (p *Prompt) GetMode() PromptMode {
	return p.mode
}

func (p *Prompt) SetMode(mode PromptMode) *Prompt {
	p.mode = mode

	p.input.TextStyle = getPromptStyle(mode)
	p.input.Placeholder = getPromptPlaceholder(mode)

	if mode == ConfigPromptMode {
		p.input.EchoMode = textinput.EchoPassword
	} else {
		p.input.EchoMode = textinput.EchoNormal
	}

	p.updatePromptIcon()

	return p
}

func (p *Prompt) SetRemoteHost(host string) *Prompt {
	p.remoteHost = host
	if host != "" && p.mode == AgentPromptMode {
		style := getPromptStyle(AgentPromptMode)
		p.input.Prompt = style.Render(fmt.Sprintf("🤖 %s > ", host))
		p.input.Placeholder = fmt.Sprintf("Task for %s...", host)
	}
	return p
}

func (p *Prompt) SetModelLabel(model string) *Prompt {
	p.modelLabel = model
	p.updatePromptIcon()
	return p
}

func (p *Prompt) updatePromptIcon() {
	style := getPromptStyle(p.mode)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(dim_color))

	icon := getPromptIconRaw(p.mode)

	if p.remoteHost != "" && p.mode == AgentPromptMode {
		icon = fmt.Sprintf("🤖 %s > ", p.remoteHost)
	}

	if p.modelLabel != "" {
		p.input.Prompt = style.Render(icon) + dimStyle.Render(fmt.Sprintf("[%s] ", p.modelLabel))
	} else {
		p.input.Prompt = style.Render(icon)
	}
}

func (p *Prompt) SetPlaceholder(text string) *Prompt {
	p.input.Placeholder = text
	return p
}

func (p *Prompt) SetEchoMode(mode textinput.EchoMode) *Prompt {
	p.input.EchoMode = mode
	return p
}

func (p *Prompt) SetValue(value string) *Prompt {
	p.input.SetValue(value)
	return p
}

func (p *Prompt) GetValue() string {
	return p.input.Value()
}

func (p *Prompt) Blur() *Prompt {
	p.input.Blur()
	return p
}

func (p *Prompt) Focus() *Prompt {
	p.input.Focus()
	return p
}

func (p *Prompt) Update(msg tea.Msg) (*Prompt, tea.Cmd) {
	var updateCmd tea.Cmd
	p.input, updateCmd = p.input.Update(msg)
	return p, updateCmd
}

func (p *Prompt) View() string {
	return p.input.View()
}

func (p *Prompt) AsString() string {
	style := getPromptStyle(p.mode)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(dim_color))
	icon := getPromptIconRaw(p.mode)
	if p.remoteHost != "" && p.mode == AgentPromptMode {
		icon = fmt.Sprintf("🤖 %s > ", p.remoteHost)
	}
	prefix := style.Render(icon)
	if p.modelLabel != "" {
		prefix += dimStyle.Render(fmt.Sprintf("[%s] ", p.modelLabel))
	}
	return fmt.Sprintf("%s%s", prefix, style.Render(p.input.Value()))
}

func getPromptStyle(mode PromptMode) lipgloss.Style {
	switch mode {
	case ExecPromptMode:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(exec_color))
	case ConfigPromptMode:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(config_color))
	case AgentPromptMode:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(agent_color))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(chat_color))
	}
}

func getPromptIconRaw(mode PromptMode) string {
	switch mode {
	case ExecPromptMode:
		return exec_icon
	case ConfigPromptMode:
		return config_icon
	case AgentPromptMode:
		return agent_icon
	default:
		return chat_icon
	}
}

func getPromptIcon(mode PromptMode) string {
	style := getPromptStyle(mode)
	return style.Render(getPromptIconRaw(mode))
}

func getPromptPlaceholder(mode PromptMode) string {
	switch mode {
	case ExecPromptMode:
		return exec_placeholder
	case ConfigPromptMode:
		return config_placeholder
	case AgentPromptMode:
		return agent_placeholder
	default:
		return chat_placeholder
	}
}
