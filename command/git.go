package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/ekkinox/yai/run"
)

func cmdDiff(_ string, ctx *Context) Result {
	output, err := run.CaptureCommand("git diff", ctx.WorkDir, 15*time.Second)
	if err != nil {
		return Result{Output: fmt.Sprintf("git diff failed: %s", err), IsError: true}
	}

	diff := output.Stdout
	if diff == "" {
		// Try staged diff
		output, err = run.CaptureCommand("git diff --staged", ctx.WorkDir, 15*time.Second)
		if err != nil {
			return Result{Output: fmt.Sprintf("git diff --staged failed: %s", err), IsError: true}
		}
		diff = output.Stdout
		if diff == "" {
			return Result{Output: "No changes (working tree clean)."}
		}
		return Result{Output: fmt.Sprintf("**Staged changes:**\n```diff\n%s\n```", truncateOutput(diff, 8000))}
	}

	return Result{Output: fmt.Sprintf("```diff\n%s\n```", truncateOutput(diff, 8000))}
}

func cmdCommit(args string, ctx *Context) Result {
	msg := strings.TrimSpace(args)
	if msg == "" {
		return Result{Output: "Usage: `/commit <message>`", IsError: true}
	}

	// Stage all changes
	stageOut, err := run.CaptureCommand("git add -A", ctx.WorkDir, 15*time.Second)
	if err != nil {
		return Result{Output: fmt.Sprintf("git add failed: %s", err), IsError: true}
	}
	if stageOut.ExitCode != 0 {
		return Result{Output: fmt.Sprintf("git add failed: %s", stageOut.Stderr), IsError: true}
	}

	// Check there's something to commit
	statusOut, err := run.CaptureCommand("git diff --cached --quiet", ctx.WorkDir, 10*time.Second)
	if err == nil && statusOut.ExitCode == 0 {
		return Result{Output: "Nothing to commit (no staged changes)."}
	}

	// Commit
	commitCmd := fmt.Sprintf("git commit -m %s", shellQuote(msg))
	commitOut, err := run.CaptureCommand(commitCmd, ctx.WorkDir, 30*time.Second)
	if err != nil {
		return Result{Output: fmt.Sprintf("git commit failed: %s", err), IsError: true}
	}
	if commitOut.ExitCode != 0 {
		return Result{Output: fmt.Sprintf("git commit failed:\n```\n%s\n```", commitOut.Stderr), IsError: true}
	}

	return Result{Output: fmt.Sprintf("[committed] %s\n```\n%s\n```", msg, strings.TrimSpace(commitOut.Stdout))}
}

func cmdStatus(_ string, ctx *Context) Result {
	output, err := run.CaptureCommand("git status --short", ctx.WorkDir, 10*time.Second)
	if err != nil {
		return Result{Output: fmt.Sprintf("git status failed: %s", err), IsError: true}
	}

	status := strings.TrimSpace(output.Stdout)
	if status == "" {
		return Result{Output: "Working tree clean."}
	}

	return Result{Output: fmt.Sprintf("```\n%s\n```", status)}
}

func cmdLog(_ string, ctx *Context) Result {
	output, err := run.CaptureCommand("git log --oneline -20", ctx.WorkDir, 10*time.Second)
	if err != nil {
		return Result{Output: fmt.Sprintf("git log failed: %s", err), IsError: true}
	}

	log := strings.TrimSpace(output.Stdout)
	if log == "" {
		return Result{Output: "No commits yet."}
	}

	return Result{Output: fmt.Sprintf("```\n%s\n```", log)}
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
