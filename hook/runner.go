package hook

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bearstonem/helm/config"
)

// Result is what a hook execution returns.
type Result struct {
	Action  config.HookAction
	Message string // output from the hook command
}

// Runner executes hooks for tool calls.
type Runner struct {
	hooks   []config.HookConfig
	workDir string
}

// NewRunner creates a hook runner with the given hook configs.
func NewRunner(hooks []config.HookConfig, workDir string) *Runner {
	return &Runner{
		hooks:   hooks,
		workDir: workDir,
	}
}

// RunPreToolUse runs all PreToolUse hooks for the given tool call.
// If any hook exits non-zero, it returns Deny with the hook's stderr/stdout as message.
// Otherwise returns Allow.
func (r *Runner) RunPreToolUse(toolName string, toolArgs string) Result {
	hooks := config.HooksForEvent(r.hooks, config.HookPreToolUse)
	if len(hooks) == 0 {
		return Result{Action: config.HookAllow}
	}

	for _, h := range hooks {
		result := r.execute(h, map[string]string{
			"HELM_HOOK_EVENT": string(config.HookPreToolUse),
			"HELM_TOOL_NAME":  toolName,
			"HELM_TOOL_ARGS":  toolArgs,
		})
		if result.Action == config.HookDeny {
			return result
		}
	}

	return Result{Action: config.HookAllow}
}

// RunPostToolUse runs all PostToolUse hooks after a tool completes.
// PostToolUse hooks are informational — they cannot deny (always returns Allow).
func (r *Runner) RunPostToolUse(toolName string, toolArgs string, toolResult string) Result {
	hooks := config.HooksForEvent(r.hooks, config.HookPostToolUse)
	if len(hooks) == 0 {
		return Result{Action: config.HookAllow}
	}

	var messages []string
	for _, h := range hooks {
		result := r.execute(h, map[string]string{
			"HELM_HOOK_EVENT":  string(config.HookPostToolUse),
			"HELM_TOOL_NAME":   toolName,
			"HELM_TOOL_ARGS":   toolArgs,
			"HELM_TOOL_RESULT": toolResult,
		})
		if result.Message != "" {
			messages = append(messages, result.Message)
		}
	}

	return Result{
		Action:  config.HookAllow,
		Message: strings.Join(messages, "\n"),
	}
}

// execute runs a single hook command with the given environment variables.
func (r *Runner) execute(h config.HookConfig, env map[string]string) Result {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	cmd := exec.Command("bash", "-c", h.Command)
	cmd.Dir = r.workDir

	// Inherit current environment and add hook-specific vars
	cmd.Env = append(os.Environ(), formatEnv(env)...)

	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		msg := strings.TrimSpace(output.String())
		if err != nil {
			// Non-zero exit = deny for pre hooks
			if msg == "" {
				msg = fmt.Sprintf("hook %q failed: %s", hookName(h), err)
			}
			return Result{Action: config.HookDeny, Message: msg}
		}
		return Result{Action: config.HookAllow, Message: msg}

	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return Result{
			Action:  config.HookDeny,
			Message: fmt.Sprintf("hook %q timed out after %s", hookName(h), timeout),
		}
	}
}

func hookName(h config.HookConfig) string {
	if h.Name != "" {
		return h.Name
	}
	return h.Command
}

func formatEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
