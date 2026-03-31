package ui

type PromptMode int

const (
	ExecPromptMode PromptMode = iota
	ConfigPromptMode
	ChatPromptMode
	AgentPromptMode
	DefaultPromptMode
)

func (m PromptMode) String() string {
	switch m {
	case ExecPromptMode:
		return "exec"
	case ConfigPromptMode:
		return "config"
	case ChatPromptMode:
		return "chat"
	case AgentPromptMode:
		return "agent"
	default:
		return "default"
	}
}

func GetPromptModeFromString(s string) PromptMode {
	switch s {
	case "exec":
		return ExecPromptMode
	case "config":
		return ConfigPromptMode
	case "chat":
		return ChatPromptMode
	case "agent":
		return AgentPromptMode
	default:
		return DefaultPromptMode
	}
}

type RunMode int

const (
	CliMode RunMode = iota
	ReplMode
)

func (m RunMode) String() string {
	if m == CliMode {
		return "cli"
	} else {
		return "repl"
	}
}
