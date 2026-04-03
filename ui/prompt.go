package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
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
	area       textarea.Model   // multiline input for normal modes
	input      textinput.Model  // single-line input for config (password) mode
	remoteHost string
	modelLabel string
}

func NewPrompt(mode PromptMode) *Prompt {
	// textarea for normal multiline input
	area := textarea.New()
	area.Placeholder = getPromptPlaceholder(mode)
	area.ShowLineNumbers = false
	area.CharLimit = 4096
	area.MaxHeight = 20
	area.SetWidth(120)
	area.SetHeight(1) // start as single line, grows with content
	area.FocusedStyle.CursorLine = lipgloss.NewStyle() // no line highlight
	area.FocusedStyle.Base = lipgloss.NewStyle()
	area.BlurredStyle.Base = lipgloss.NewStyle()
	area.Prompt = getPromptIcon(mode)
	// Enter submits; Alt+Enter inserts newline
	area.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	area.Focus()

	// textinput for config/password mode
	input := textinput.New()
	input.Placeholder = getPromptPlaceholder(mode)
	input.TextStyle = getPromptStyle(mode)
	input.Prompt = getPromptIcon(mode)
	if mode == ConfigPromptMode {
		input.EchoMode = textinput.EchoPassword
	}

	return &Prompt{
		mode:  mode,
		area:  area,
		input: input,
	}
}

func (p *Prompt) isConfigMode() bool {
	return p.mode == ConfigPromptMode
}

func (p *Prompt) GetMode() PromptMode {
	return p.mode
}

func (p *Prompt) SetMode(mode PromptMode) *Prompt {
	p.mode = mode

	if mode == ConfigPromptMode {
		p.input.EchoMode = textinput.EchoPassword
		p.input.TextStyle = getPromptStyle(mode)
		p.input.Placeholder = getPromptPlaceholder(mode)
		p.input.Prompt = getPromptIcon(mode)
		p.input.Focus()
	} else {
		p.input.EchoMode = textinput.EchoNormal
		p.area.Placeholder = getPromptPlaceholder(mode)
		p.updatePromptIcon()
	}

	return p
}

func (p *Prompt) SetRemoteHost(host string) *Prompt {
	p.remoteHost = host
	if host != "" && p.mode == AgentPromptMode {
		style := getPromptStyle(AgentPromptMode)
		prompt := style.Render(fmt.Sprintf("🤖 %s > ", host))
		p.area.Prompt = prompt
		p.area.Placeholder = fmt.Sprintf("Task for %s...", host)
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
		prompt := style.Render(icon) + dimStyle.Render(fmt.Sprintf("[%s] ", p.modelLabel))
		p.area.Prompt = prompt
		p.input.Prompt = prompt
	} else {
		prompt := style.Render(icon)
		p.area.Prompt = prompt
		p.input.Prompt = prompt
	}
}

func (p *Prompt) SetWidth(w int) *Prompt {
	p.area.SetWidth(w)
	return p
}

func (p *Prompt) SetPlaceholder(text string) *Prompt {
	if p.isConfigMode() {
		p.input.Placeholder = text
	} else {
		p.area.Placeholder = text
	}
	return p
}

func (p *Prompt) SetEchoMode(mode textinput.EchoMode) *Prompt {
	p.input.EchoMode = mode
	return p
}

func (p *Prompt) SetValue(value string) *Prompt {
	if p.isConfigMode() {
		p.input.SetValue(value)
	} else {
		p.area.SetValue(value)
		// Auto-resize height based on content
		p.autoResize()
	}
	return p
}

func (p *Prompt) GetValue() string {
	if p.isConfigMode() {
		return p.input.Value()
	}
	return p.area.Value()
}

func (p *Prompt) Blur() *Prompt {
	if p.isConfigMode() {
		p.input.Blur()
	} else {
		p.area.Blur()
	}
	return p
}

func (p *Prompt) Focus() *Prompt {
	if p.isConfigMode() {
		p.input.Focus()
	} else {
		p.area.Focus()
	}
	return p
}

func (p *Prompt) Update(msg tea.Msg) (*Prompt, tea.Cmd) {
	if p.isConfigMode() {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return p, cmd
	}
	var cmd tea.Cmd
	p.area, cmd = p.area.Update(msg)
	p.autoResize()
	return p, cmd
}

func (p *Prompt) View() string {
	if p.isConfigMode() {
		return p.input.View()
	}
	return p.area.View()
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
	value := p.GetValue()
	// For multiline, show first line with prefix, indent continuation lines
	lines := strings.Split(value, "\n")
	if len(lines) <= 1 {
		return fmt.Sprintf("%s%s", prefix, style.Render(value))
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s%s", prefix, style.Render(lines[0])))
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	for _, line := range lines[1:] {
		b.WriteString(fmt.Sprintf("\n%s%s", indent, style.Render(line)))
	}
	return b.String()
}

// autoResize adjusts the textarea height based on content line count.
func (p *Prompt) autoResize() {
	lines := strings.Count(p.area.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 20 {
		lines = 20
	}
	p.area.SetHeight(lines)
}

// InputBlink returns the appropriate blink command for the current mode.
func InputBlink() tea.Msg {
	return textarea.Blink()
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
