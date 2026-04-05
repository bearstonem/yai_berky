package command

import (
	"github.com/bearstonem/helm/agent"
	"github.com/bearstonem/helm/config"
	"github.com/bearstonem/helm/goal"
	"github.com/bearstonem/helm/memory"
	"github.com/bearstonem/helm/session"
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
	// Integration management
	ReloadIntegrationsFn func()
	// Memory store
	MemoryStore *memory.Store
	// Agent profile management
	ListAgents      func() []agent.Profile
	SetAgentProfile func(id string) error
	CurrentAgent    string // currently selected agent profile ID
	// Goal management
	ListGoals func() []goal.Goal
	// Session loading
	LoadSessionFn func(id string) error
}
