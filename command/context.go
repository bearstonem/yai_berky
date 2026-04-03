package command

import (
	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/session"
)

// Context provides commands access to application state.
type Context struct {
	Config       *config.Config
	HomeDir      string
	WorkDir      string
	SessionID    string
	Mode         string // "exec", "chat", "agent"
	YoloMode     bool
	UsageTracker *UsageTracker
	// Callbacks for commands that need to mutate engine state
	ResetFn     func()
	CompactFn   func() string // returns summary of compacted messages
	SessionList func() []session.SessionInfo
	SetYoloFn      func(bool)
	GetModelFn     func() string
	SetModelFn     func(string)
	SwitchProvider func(provider, apiKey, baseURL string) error
}
