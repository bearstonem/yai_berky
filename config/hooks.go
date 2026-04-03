package config

import (
	"encoding/json"

	"github.com/spf13/viper"
)

const (
	config_hooks = "HOOKS"
)

// HookEvent identifies when a hook fires.
type HookEvent string

const (
	HookPreToolUse  HookEvent = "pre_tool_use"
	HookPostToolUse HookEvent = "post_tool_use"
)

// HookAction tells the system what to do after a hook runs.
type HookAction string

const (
	HookAllow HookAction = "allow"  // continue execution (default)
	HookDeny  HookAction = "deny"   // block tool execution, return hook message
	HookLog   HookAction = "log"    // continue execution, capture output for logging
)

// HookConfig describes a single hook entry from settings.
type HookConfig struct {
	Event   HookEvent `json:"event"`
	Command string    `json:"command"`
	Name    string    `json:"name,omitempty"`
	Timeout int       `json:"timeout,omitempty"` // seconds, default 10
}

// LoadHooks reads the HOOKS config key and returns parsed hook configs.
func LoadHooks() []HookConfig {
	raw := viper.GetString(config_hooks)
	if raw == "" {
		return nil
	}

	var hooks []HookConfig
	if err := json.Unmarshal([]byte(raw), &hooks); err != nil {
		return nil
	}
	return hooks
}

// LoadHooksFromRaw parses hook configs from a raw JSON array (used when viper
// stores the value as interface{} from overlay config).
func LoadHooksFromViper() []HookConfig {
	val := viper.Get(config_hooks)
	if val == nil {
		return nil
	}

	// If it's a string, parse directly
	if s, ok := val.(string); ok {
		var hooks []HookConfig
		if err := json.Unmarshal([]byte(s), &hooks); err != nil {
			return nil
		}
		return hooks
	}

	// If viper decoded it as []interface{} (from JSON overlay), re-marshal then unmarshal
	data, err := json.Marshal(val)
	if err != nil {
		return nil
	}
	var hooks []HookConfig
	if err := json.Unmarshal(data, &hooks); err != nil {
		return nil
	}
	return hooks
}

// HooksForEvent filters hooks by event type.
func HooksForEvent(hooks []HookConfig, event HookEvent) []HookConfig {
	var matched []HookConfig
	for _, h := range hooks {
		if h.Event == event {
			matched = append(matched, h)
		}
	}
	return matched
}
