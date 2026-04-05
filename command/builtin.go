package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/bearstonem/helm/config"
	"github.com/bearstonem/helm/cron"
	"github.com/bearstonem/helm/skill"
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
		Name:        "model",
		Description: "Show or switch model/provider at runtime",
		Handler:     cmdModel,
	})

	r.Register(&Command{
		Name:        "yolo",
		Aliases:     []string{"auto"},
		Description: "Toggle yolo mode (auto-execute agent tool calls)",
		Handler:     cmdYolo,
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

	r.Register(&Command{
		Name:        "memory",
		Aliases:     []string{"mem"},
		Description: "Show vector memory stats",
		Handler:     cmdMemory,
	})

	r.Register(&Command{
		Name:        "integrate",
		Aliases:     []string{"int"},
		Description: "Manage tool integrations (ComfyUI, webhooks, etc.)",
		Handler:     cmdIntegrate,
	})

	r.Register(&Command{
		Name:        "skill",
		Aliases:     []string{"skills"},
		Description: "List or remove agent-created skills",
		Handler:     cmdSkill,
	})

	r.Register(&Command{
		Name:        "agent",
		Aliases:     []string{"agents"},
		Description: "List, select, or clear agent profiles",
		Handler:     cmdAgent,
	})

	r.Register(&Command{
		Name:        "goals",
		Aliases:     []string{"goal"},
		Description: "List current self-improvement goals",
		Handler:     cmdGoals,
	})

	r.Register(&Command{
		Name:        "cron",
		Aliases:     []string{"jobs"},
		Description: "List scheduled cron jobs",
		Handler:     cmdCron,
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
		{"/session [load <id>]", "List or load a saved session"},
		{"/mode [exec|chat|agent]", "Show or switch mode"},
		{"/model [provider/model]", "Show or switch model (use --save or /model save)"},
		{"/yolo", "Toggle yolo mode (auto-execute agent tools)"},
		{"/integrate", "Manage tool integrations (add/remove/list)"},
		{"/skill", "List or remove agent-created skills"},
		{"/agent [select <id>|clear]", "List, select, or clear agent profile"},
		{"/goals", "List self-improvement goals"},
		{"/memory", "Show vector memory stats"},
		{"/cron", "List scheduled cron jobs and their status"},
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

func cmdSession(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)

	// /session load <id>
	if strings.HasPrefix(args, "load ") {
		id := strings.TrimSpace(args[5:])
		if id == "" {
			return Result{Output: "Usage: `/session load <id>`", IsError: true}
		}
		if ctx.LoadSessionFn == nil {
			return Result{Output: "Session loading not available.", IsError: true}
		}
		if err := ctx.LoadSessionFn(id); err != nil {
			return Result{Output: fmt.Sprintf("Error: %s", err), IsError: true}
		}
		return Result{Output: fmt.Sprintf("Session `%s` loaded. Continue the conversation.", id)}
	}

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

func cmdModel(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)

	// Check for --save flag
	save := false
	if strings.HasSuffix(args, " --save") {
		save = true
		args = strings.TrimSpace(strings.TrimSuffix(args, "--save"))
	} else if strings.HasSuffix(args, " --default") {
		save = true
		args = strings.TrimSpace(strings.TrimSuffix(args, "--default"))
	}

	// /model save — persist current runtime model as default
	if args == "save" || args == "default" {
		provider := ""
		model := ""
		if ctx.GetModelFn != nil {
			model = ctx.GetModelFn()
		}
		if ctx.Config != nil {
			provider = ctx.Config.GetAiConfig().GetProvider()
		}
		if model == "" {
			return Result{Output: "No model is currently set.", IsError: true}
		}
		if err := config.SaveDefaultModel(provider, model); err != nil {
			return Result{Output: fmt.Sprintf("Failed to save default: %s", err), IsError: true}
		}
		out := fmt.Sprintf("Saved default model: `%s`", model)
		if provider != "" {
			out = fmt.Sprintf("Saved default: **%s** / `%s`", config.ProviderDisplayNames[provider], model)
		}
		return Result{Output: "model:" + out}
	}

	if args == "" {
		// Show current model info
		if ctx.GetModelFn == nil {
			return Result{Output: "Model info not available.", IsError: true}
		}
		model := ctx.GetModelFn()
		provider := ""
		if ctx.Config != nil {
			provider = ctx.Config.GetAiConfig().GetProvider()
		}
		out := fmt.Sprintf("**Provider:** `%s` | **Model:** `%s`\n\n", provider, model)
		out += "**Usage:** `/model <model-name>` or `/model <provider>/<model>`\n"
		out += "**Save as default:** `/model save` or `/model <model> --save`\n\n"
		out += "**Available providers:** "
		for i, p := range config.ProviderList() {
			if i > 0 {
				out += ", "
			}
			out += fmt.Sprintf("`%s`", p)
		}
		out += "\n"
		return Result{Output: out}
	}

	// Check for provider/model format
	if strings.Contains(args, "/") {
		parts := strings.SplitN(args, "/", 2)
		provider := strings.TrimSpace(parts[0])
		model := strings.TrimSpace(parts[1])

		if model == "" {
			return Result{Output: "Missing model name after `/`. Usage: `/model provider/model`", IsError: true}
		}

		// Validate provider
		valid := false
		for _, p := range config.ProviderList() {
			if p == provider {
				valid = true
				break
			}
		}
		if !valid {
			return Result{Output: fmt.Sprintf("Unknown provider: `%s`", provider), IsError: true}
		}

		// Switch provider
		if ctx.SwitchProvider != nil {
			fallbackKey := ""
			baseURL := ""
			if ctx.Config != nil {
				fallbackKey = ctx.Config.GetAiConfig().GetKey()
				if defaultURL, ok := config.ProviderBaseURLs[provider]; ok {
					baseURL = defaultURL
				}
			}
			apiKey := config.ResolveAPIKey(provider, fallbackKey)
			if apiKey == "" && config.ProviderNeedsAPIKey(provider) {
				envVar := config.ProviderEnvKeys[provider]
				return Result{Output: fmt.Sprintf("No API key for `%s`. Set `%s` env var or configure it in settings.", provider, envVar), IsError: true}
			}
			if err := ctx.SwitchProvider(provider, apiKey, baseURL); err != nil {
				return Result{Output: fmt.Sprintf("Failed to switch provider: %s", err), IsError: true}
			}
		}

		// Set model
		if ctx.SetModelFn != nil {
			ctx.SetModelFn(model)
		}

		// Save if requested
		if save {
			if err := config.SaveDefaultModel(provider, model); err != nil {
				name := config.ProviderDisplayNames[provider]
				if name == "" {
					name = provider
				}
				return Result{Output: fmt.Sprintf("model:switched to **%s** / `%s` (failed to save: %s)", name, model, err)}
			}
			name := config.ProviderDisplayNames[provider]
			if name == "" {
				name = provider
			}
			return Result{Output: fmt.Sprintf("model:switched to **%s** / `%s` (saved as default)", name, model)}
		}

		name := config.ProviderDisplayNames[provider]
		if name == "" {
			name = provider
		}
		return Result{Output: fmt.Sprintf("model:switched to **%s** / `%s`", name, model)}
	}

	// Just a model name — switch model only, keep current provider
	if ctx.SetModelFn != nil {
		ctx.SetModelFn(args)
	}

	if save {
		provider := ""
		if ctx.Config != nil {
			provider = ctx.Config.GetAiConfig().GetProvider()
		}
		if err := config.SaveDefaultModel(provider, args); err != nil {
			return Result{Output: fmt.Sprintf("model:switched to `%s` (failed to save: %s)", args, err)}
		}
		return Result{Output: fmt.Sprintf("model:switched to `%s` (saved as default)", args)}
	}

	return Result{Output: fmt.Sprintf("model:switched to `%s`", args)}
}

func cmdYolo(args string, ctx *Context) Result {
	args = strings.TrimSpace(strings.ToLower(args))

	var enable bool
	switch args {
	case "on", "true", "1", "yes":
		enable = true
	case "off", "false", "0", "no":
		enable = false
	case "":
		// Toggle
		enable = !ctx.YoloMode
	default:
		return Result{Output: "Usage: `/yolo` (toggle), `/yolo on`, `/yolo off`", IsError: true}
	}

	if ctx.SetYoloFn != nil {
		ctx.SetYoloFn(enable)
	}

	if enable {
		return Result{Output: "yolo:on"}
	}
	return Result{Output: "yolo:off"}
}

func cmdIntegrate(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)

	if args == "" || args == "list" || args == "ls" {
		integrations := config.LoadIntegrationsFromViper()
		if len(integrations) == 0 {
			return Result{Output: "No integrations configured.\n\nUse `/integrate add` to set one up."}
		}

		out := "**Tool Integrations**\n\n"
		out += "| Name | Type | Endpoint | Enabled |\n"
		out += "|---|---|---|---|\n"
		for _, ic := range integrations {
			enabled := "yes"
			if !ic.Enabled {
				enabled = "no"
			}
			out += fmt.Sprintf("| `%s` | %s | `%s` | %s |\n", ic.Name, string(ic.Type), ic.Endpoint, enabled)
		}
		out += "\n**Commands:** `/integrate add`, `/integrate remove <name>`, `/integrate toggle <name>`"
		return Result{Output: out}
	}

	parts := strings.SplitN(args, " ", 2)
	subCmd := strings.ToLower(parts[0])
	subArgs := ""
	if len(parts) > 1 {
		subArgs = strings.TrimSpace(parts[1])
	}

	switch subCmd {
	case "add":
		return Result{Output: "integrate:add"}

	case "remove", "rm", "delete":
		if subArgs == "" {
			return Result{Output: "Usage: `/integrate remove <name>`", IsError: true}
		}
		if config.RemoveIntegration(subArgs) {
			if ctx.ReloadIntegrationsFn != nil {
				ctx.ReloadIntegrationsFn()
			}
			return Result{Output: fmt.Sprintf("Removed integration `%s`.", subArgs)}
		}
		return Result{Output: fmt.Sprintf("Integration `%s` not found.", subArgs), IsError: true}

	case "toggle":
		if subArgs == "" {
			return Result{Output: "Usage: `/integrate toggle <name>`", IsError: true}
		}
		integrations := config.LoadIntegrationsFromViper()
		found := false
		for i := range integrations {
			if integrations[i].Name == subArgs {
				integrations[i].Enabled = !integrations[i].Enabled
				config.SaveIntegrationsToViper(integrations)
				status := "enabled"
				if !integrations[i].Enabled {
					status = "disabled"
				}
				if ctx.ReloadIntegrationsFn != nil {
					ctx.ReloadIntegrationsFn()
				}
				return Result{Output: fmt.Sprintf("Integration `%s` is now **%s**.", subArgs, status)}
			}
		}
		if !found {
			return Result{Output: fmt.Sprintf("Integration `%s` not found.", subArgs), IsError: true}
		}
		return Result{}

	default:
		return Result{Output: "Usage: `/integrate [add|remove|toggle|list]`", IsError: true}
	}
}

func cmdMemory(_ string, ctx *Context) Result {
	if ctx.MemoryStore == nil {
		return Result{Output: "Vector memory not available (no embedding provider configured).", IsError: true}
	}
	msgs, skills, sessions := ctx.MemoryStore.Stats()
	out := "**Vector Memory**\n\n"
	out += fmt.Sprintf("| Type | Indexed |\n")
	out += fmt.Sprintf("|---|---|\n")
	out += fmt.Sprintf("| Messages | %d |\n", msgs)
	out += fmt.Sprintf("| Skills | %d |\n", skills)
	out += fmt.Sprintf("| Sessions | %d |\n", sessions)
	out += fmt.Sprintf("\nMemory is used to recall relevant past conversations and skill context.\n")
	return Result{Output: out}
}

func cmdSkill(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)

	skills, err := skill.LoadAll(ctx.HomeDir)
	if err != nil {
		return Result{Output: fmt.Sprintf("Error loading skills: %s", err), IsError: true}
	}

	// /skill remove <name>
	if strings.HasPrefix(args, "remove ") || strings.HasPrefix(args, "rm ") || strings.HasPrefix(args, "delete ") {
		parts := strings.SplitN(args, " ", 2)
		name := strings.TrimSpace(parts[1])
		if name == "" {
			return Result{Output: "Usage: `/skill remove <name>`", IsError: true}
		}
		if err := skill.Remove(ctx.HomeDir, name); err != nil {
			return Result{Output: fmt.Sprintf("Error: %s", err), IsError: true}
		}
		return Result{Output: fmt.Sprintf("Skill `%s` removed.", name)}
	}

	// /skill (list)
	if len(skills) == 0 {
		return Result{Output: "No skills created yet.\n\nIn agent mode, ask the AI to create a skill for you — e.g. \"learn how to call the weather API\"."}
	}

	out := "**Agent Skills**\n\n"
	out += "| Name | Tool | Language | Description |\n"
	out += "|---|---|---|---|\n"
	for _, s := range skills {
		desc := s.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		out += fmt.Sprintf("| `%s` | `%s` | %s | %s |\n", s.Name, s.ToolName(), s.Language, desc)
	}
	out += "\n**Remove:** `/skill remove <name>`"
	return Result{Output: out}
}

func cmdAgent(args string, ctx *Context) Result {
	args = strings.TrimSpace(args)

	// /agent select <id>
	if strings.HasPrefix(args, "select ") || strings.HasPrefix(args, "use ") {
		parts := strings.SplitN(args, " ", 2)
		id := strings.TrimSpace(parts[1])
		if id == "" {
			return Result{Output: "Usage: `/agent select <id>`", IsError: true}
		}
		if ctx.SetAgentProfile == nil {
			return Result{Output: "Agent selection not available.", IsError: true}
		}
		if err := ctx.SetAgentProfile(id); err != nil {
			return Result{Output: fmt.Sprintf("Error: %s", err), IsError: true}
		}
		return Result{Output: fmt.Sprintf("Agent profile `%s` activated. Agent will use this profile's system prompt and tools.", id)}
	}

	// /agent clear
	if args == "clear" || args == "reset" {
		if ctx.SetAgentProfile != nil {
			ctx.SetAgentProfile("")
		}
		return Result{Output: "Agent profile cleared. Using default primary agent."}
	}

	// /agent (list)
	if ctx.ListAgents == nil {
		return Result{Output: "Agent listing not available.", IsError: true}
	}
	agents := ctx.ListAgents()
	if len(agents) == 0 {
		return Result{Output: "No agent profiles yet.\n\nIn agent mode, ask the AI to create one, or use the web GUI (`helm --gui`)."}
	}

	current := ""
	if ctx.CurrentAgent != "" {
		current = ctx.CurrentAgent
	}

	out := "**Agent Profiles**\n\n"
	out += "| ID | Name | Tools | Active |\n"
	out += "|---|---|---|---|\n"
	for _, a := range agents {
		tools := "all"
		if len(a.Tools) > 0 {
			tools = fmt.Sprintf("%d", len(a.Tools))
		}
		active := ""
		if a.ID == current {
			active = "**active**"
		}
		desc := a.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		out += fmt.Sprintf("| `%s` | %s | %s | %s |\n", a.ID, a.Name, tools, active)
	}
	out += "\n**Select:** `/agent select <id>` | **Clear:** `/agent clear`"
	return Result{Output: out}
}

func cmdGoals(_ string, ctx *Context) Result {
	if ctx.ListGoals == nil {
		return Result{Output: "Goals not available.", IsError: true}
	}
	goals := ctx.ListGoals()
	if len(goals) == 0 {
		return Result{Output: "No goals yet.\n\nStart the self-improvement loop from the web GUI (`helm --gui`), or ask the agent to create goals."}
	}

	out := "**Self-Improvement Goals**\n\n"
	out += "| ID | Title | Status | Priority | Progress |\n"
	out += "|---|---|---|---|---|\n"
	for _, g := range goals {
		progress := g.Progress
		if len(progress) > 40 {
			progress = progress[:37] + "..."
		}
		pri := "medium"
		if g.Priority == 1 {
			pri = "high"
		} else if g.Priority >= 3 {
			pri = "low"
		}
		out += fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n", g.ID, g.Title, g.Status, pri, progress)
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

func cmdCron(_ string, ctx *Context) Result {
	jobs, err := cron.LoadAll(ctx.HomeDir)
	if err != nil {
		return Result{Output: fmt.Sprintf("Error loading cron jobs: %s", err), IsError: true}
	}
	if len(jobs) == 0 {
		return Result{Output: "No cron jobs configured.\n\nUse the web GUI (`helm --gui`) to create cron jobs."}
	}

	out := "**Scheduled Cron Jobs**\n\n"
	out += "| Name | Schedule | Enabled | Last Run | Status |\n"
	out += "|---|---|---|---|---|\n"
	for _, j := range jobs {
		enabled := "yes"
		if !j.Enabled {
			enabled = "no"
		}
		lastRun := "never"
		if j.LastRunAt != nil {
			lastRun = j.LastRunAt.Format(time.RFC822)
		}
		status := j.LastStatus
		if status == "" {
			status = "pending"
		}
		out += fmt.Sprintf("| `%s` | `%s` | %s | %s | %s |\n", j.Name, j.Schedule, enabled, lastRun, status)
	}
	return Result{Output: out}
}
