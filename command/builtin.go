package command

import (
	"fmt"
	"strings"
)

// RegisterBuiltins adds all built-in slash commands to the registry.
func RegisterBuiltins(r *Registry) {
	r.Register(&Command{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands",
		Handler:     cmdHelp,
	})

	r.Register(&Command{
		Name:        "clear",
		Aliases:     []string{"cls"},
		Description: "Clear the terminal (keep history)",
		Handler:     cmdClear,
	})

	r.Register(&Command{
		Name:        "reset",
		Description: "Clear terminal and reset conversation",
		Handler:     cmdReset,
	})

	r.Register(&Command{
		Name:        "compact",
		Description: "Summarize old messages to save context",
		Handler:     cmdCompact,
	})

	r.Register(&Command{
		Name:        "cost",
		Aliases:     []string{"usage"},
		Description: "Show token usage and estimated cost",
		Handler:     cmdCost,
	})

	r.Register(&Command{
		Name:        "session",
		Aliases:     []string{"sessions"},
		Description: "List saved sessions",
		Handler:     cmdSession,
	})

	r.Register(&Command{
		Name:        "mode",
		Description: "Show or switch mode (exec/chat/agent)",
		Handler:     cmdMode,
	})

	r.Register(&Command{
		Name:        "diff",
		Description: "Show git diff of working tree",
		Handler:     cmdDiff,
	})

	r.Register(&Command{
		Name:        "commit",
		Description: "Stage all changes and commit with a message",
		Handler:     cmdCommit,
	})

	r.Register(&Command{
		Name:        "status",
		Aliases:     []string{"st"},
		Description: "Show git status",
		Handler:     cmdStatus,
	})

	r.Register(&Command{
		Name:        "log",
		Description: "Show recent git log",
		Handler:     cmdLog,
	})
}

func cmdHelp(_ string, ctx *Context) Result {
	// Build help from the global registry — we just list known commands here.
	help := "**Slash Commands**\n\n"
	help += "| Command | Description |\n"
	help += "|---|---|\n"

	commands := []struct{ name, desc string }{
		{"/help", "Show available commands"},
		{"/clear", "Clear the terminal (keep history)"},
		{"/reset", "Clear terminal and reset conversation"},
		{"/compact", "Summarize old messages to save context"},
		{"/cost", "Show token usage and estimated cost"},
		{"/session", "List saved sessions"},
		{"/mode [exec|chat|agent]", "Show or switch mode"},
		{"/diff", "Show git diff of working tree"},
		{"/commit <message>", "Stage all and commit"},
		{"/status", "Show git status"},
		{"/log", "Show recent git log"},
	}

	for _, c := range commands {
		help += fmt.Sprintf("| `%s` | %s |\n", c.name, c.desc)
	}

	return Result{Output: help}
}

func cmdClear(_ string, _ *Context) Result {
	return Result{Clear: true}
}

func cmdReset(_ string, ctx *Context) Result {
	if ctx.ResetFn != nil {
		ctx.ResetFn()
	}
	return Result{Clear: true, Reset: true, Output: "[conversation reset]\n"}
}

func cmdCompact(_ string, ctx *Context) Result {
	if ctx.CompactFn == nil {
		return Result{Output: "Compact not available in this mode.", IsError: true}
	}
	summary := ctx.CompactFn()
	if summary == "" {
		return Result{Output: "Nothing to compact — conversation is short enough."}
	}
	return Result{Output: fmt.Sprintf("[compacted] Kept summary:\n\n%s", summary)}
}

func cmdCost(_ string, ctx *Context) Result {
	if ctx.UsageTracker == nil {
		return Result{Output: "Usage tracking not available.", IsError: true}
	}
	return Result{Output: ctx.UsageTracker.Summary()}
}

func cmdSession(_ string, ctx *Context) Result {
	if ctx.SessionList == nil {
		return Result{Output: "Session listing not available.", IsError: true}
	}
	sessions := ctx.SessionList()
	if len(sessions) == 0 {
		return Result{Output: "No saved sessions found."}
	}

	out := "**Saved Sessions**\n\n"
	out += "| ID | Mode | Messages | Summary | Updated |\n"
	out += "|---|---|---|---|---|\n"

	max := 20
	if len(sessions) < max {
		max = len(sessions)
	}
	for _, s := range sessions[:max] {
		summary := s.Summary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		out += fmt.Sprintf("| `%s` | %s | %d | %s | %s |\n",
			s.ID, s.Mode, s.Messages, summary, s.UpdatedAt.Format("Jan 02 15:04"))
	}

	if len(sessions) > max {
		out += fmt.Sprintf("\n... and %d more\n", len(sessions)-max)
	}

	return Result{Output: out}
}

func cmdMode(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)
	if args == "" {
		return Result{Output: fmt.Sprintf("Current mode: **%s**", ctx.Mode)}
	}
	switch strings.ToLower(args) {
	case "exec", "chat", "agent":
		return Result{Output: fmt.Sprintf("switch:%s", args)}
	default:
		return Result{Output: fmt.Sprintf("Unknown mode: %s. Use exec, chat, or agent.", args), IsError: true}
	}
}
