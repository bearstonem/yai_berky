package hook

import (
	"testing"

	"github.com/ekkinox/yai/config"
	"github.com/stretchr/testify/assert"
)

func TestRunPreToolUse_NoHooks(t *testing.T) {
	r := NewRunner(nil, "/tmp")
	result := r.RunPreToolUse("run_command", `{"command":"echo hi"}`)
	assert.Equal(t, config.HookAllow, result.Action)
}

func TestRunPreToolUse_AllowHook(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "exit 0", Name: "allow-all"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("run_command", `{"command":"echo hi"}`)
	assert.Equal(t, config.HookAllow, result.Action)
}

func TestRunPreToolUse_DenyHook(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "echo 'blocked by policy' && exit 1", Name: "deny-all"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("run_command", `{"command":"rm -rf /"}`)
	assert.Equal(t, config.HookDeny, result.Action)
	assert.Contains(t, result.Message, "blocked by policy")
}

func TestRunPreToolUse_EnvVars(t *testing.T) {
	// Use a deny hook so the message is propagated
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "echo $YAI_TOOL_NAME:$YAI_HOOK_EVENT && exit 1"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("write_file", `{"path":"/tmp/test"}`)
	assert.Equal(t, config.HookDeny, result.Action)
	assert.Contains(t, result.Message, "write_file:pre_tool_use")
}

func TestRunPreToolUse_MultipleDenyFirst(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "echo denied && exit 1", Name: "blocker"},
		{Event: config.HookPreToolUse, Command: "echo should not run"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("run_command", `{}`)
	assert.Equal(t, config.HookDeny, result.Action)
	assert.Contains(t, result.Message, "denied")
}

func TestRunPostToolUse_NoHooks(t *testing.T) {
	r := NewRunner(nil, "/tmp")
	result := r.RunPostToolUse("run_command", `{}`, "output")
	assert.Equal(t, config.HookAllow, result.Action)
}

func TestRunPostToolUse_CapturesOutput(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPostToolUse, Command: "echo logged:$YAI_TOOL_NAME"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPostToolUse("edit_file", `{"path":"/tmp/x"}`, "ok")
	assert.Equal(t, config.HookAllow, result.Action)
	assert.Contains(t, result.Message, "logged:edit_file")
}

func TestRunPostToolUse_NeverDenies(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPostToolUse, Command: "exit 1", Name: "failing-post-hook"},
	}
	r := NewRunner(hooks, "/tmp")
	// PostToolUse hooks always return Allow regardless of exit code
	result := r.RunPostToolUse("run_command", `{}`, "done")
	assert.Equal(t, config.HookAllow, result.Action)
}

func TestRunPreToolUse_Timeout(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "sleep 30", Name: "slow-hook", Timeout: 1},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("run_command", `{}`)
	assert.Equal(t, config.HookDeny, result.Action)
	assert.Contains(t, result.Message, "timed out")
}

func TestRunPreToolUse_IgnoresPostHooks(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPostToolUse, Command: "exit 1", Name: "post-only"},
	}
	r := NewRunner(hooks, "/tmp")
	result := r.RunPreToolUse("run_command", `{}`)
	assert.Equal(t, config.HookAllow, result.Action)
}

func TestHooksForEvent(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "echo pre"},
		{Event: config.HookPostToolUse, Command: "echo post"},
		{Event: config.HookPreToolUse, Command: "echo pre2"},
	}
	pre := config.HooksForEvent(hooks, config.HookPreToolUse)
	assert.Len(t, pre, 2)
	post := config.HooksForEvent(hooks, config.HookPostToolUse)
	assert.Len(t, post, 1)
}
