package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/ekkinox/yai/ai"
	"github.com/ekkinox/yai/config"
)

// RunPipe executes a non-interactive pipe mode that bypasses bubbletea.
// Input comes from args/stdin, output goes to stdout as plain text.
// Agent mode always auto-executes tools (no confirmation prompts).
func RunPipe(input *UiInput) error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	promptMode := input.GetPromptMode()
	if promptMode == DefaultPromptMode {
		promptMode = GetPromptModeFromString(cfg.GetUserConfig().GetDefaultPromptMode())
	}

	engineMode := promptModeToEngineMode(promptMode)
	engine, err := ai.NewEngine(engineMode, cfg)
	if err != nil {
		return fmt.Errorf("creating engine: %w", err)
	}

	if input.GetPipe() != "" {
		engine.SetPipe(input.GetPipe())
	}

	if input.GetRemote() != "" {
		if err := engine.SetRemoteHost(input.GetRemote()); err != nil {
			return fmt.Errorf("setting remote host: %w", err)
		}
	}

	engine.StartNewSession()

	query := input.GetArgs()
	if query == "" {
		return fmt.Errorf("no input provided (use: yai --pipe -a 'your task')")
	}

	switch promptMode {
	case AgentPromptMode:
		return runPipeAgent(engine, query)
	case ChatPromptMode:
		return runPipeChat(engine, query)
	default:
		return runPipeExec(engine, query)
	}
}

func runPipeAgent(engine *ai.Engine, query string) error {
	// Start agent completion in a goroutine (it sends events to the channel)
	go func() {
		engine.AgentCompletion(query, true) // always auto-execute
	}()

	ch := engine.GetAgentChannel()
	for event := range ch {
		switch event.Type {
		case ai.AgentEventThinking:
			if event.Content != "" {
				// Strip <think> tags for clean output
				content := event.Content
				content = stripThinkTags(content)
				if content != "" {
					fmt.Fprintf(os.Stderr, "[thinking] %s\n", content)
				}
			}
		case ai.AgentEventToolCall:
			if event.ToolCall != nil {
				fmt.Fprintf(os.Stderr, "[tool] %s %s\n", event.ToolCall.Name, event.ToolCall.Arguments)
			}
		case ai.AgentEventToolResult:
			if event.ToolResult != nil {
				content := event.ToolResult.Content
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				fmt.Fprintf(os.Stderr, "[result] %s\n", content)
			}
		case ai.AgentEventAnswer:
			if event.Content != "" {
				content := stripThinkTags(event.Content)
				fmt.Println(content)
			}
		case ai.AgentEventError:
			if event.Error != nil {
				fmt.Fprintf(os.Stderr, "[error] %s\n", event.Error.Error())
			}
		case ai.AgentEventDone:
			return nil
		}
	}
	return nil
}

func runPipeChat(engine *ai.Engine, query string) error {
	// Use streaming for chat mode
	err := engine.ChatStreamCompletion(query)
	if err != nil {
		return err
	}

	ch := engine.GetChannel()
	for output := range ch {
		if output.GetContent() != "" {
			fmt.Print(output.GetContent())
		}
		if output.IsLast() {
			fmt.Println()
			return nil
		}
	}
	return nil
}

func runPipeExec(engine *ai.Engine, query string) error {
	output, err := engine.ExecCompletion(query)
	if err != nil {
		return err
	}
	if output.IsExecutable() {
		fmt.Println(output.GetCommand())
	} else {
		fmt.Println(output.GetExplanation())
	}
	return nil
}

// stripThinkTags removes <think>...</think> blocks from model output.
func stripThinkTags(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			// Unclosed think tag — strip everything from <think> onward
			s = strings.TrimSpace(s[:start])
			break
		}
		s = strings.TrimSpace(s[:start] + s[end+len("</think>"):])
	}
	return s
}
