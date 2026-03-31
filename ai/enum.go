package ai

type EngineMode int

const (
	ExecEngineMode EngineMode = iota
	ChatEngineMode
	AgentEngineMode
)

func (m EngineMode) String() string {
	switch m {
	case ExecEngineMode:
		return "exec"
	case AgentEngineMode:
		return "agent"
	default:
		return "chat"
	}
}
