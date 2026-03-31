package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/run"
	"github.com/ekkinox/yai/system"
)

const noexec = "[noexec]"

type RemoteSystemInfo struct {
	OS       string
	Hostname string
	Shell    string
	HomeDir  string
	Username string
}

type Engine struct {
	mode          EngineMode
	config        *config.Config
	provider      Provider
	execMessages  []Message
	chatMessages  []Message
	agentMessages []Message
	channel       chan EngineChatStreamOutput
	agentChannel  chan AgentEvent
	approvalChan  chan bool
	toolExecutor  *ToolExecutor
	pipe          string
	running       bool
	cancelFn      context.CancelFunc
	remoteHost    string
	remoteInfo    *RemoteSystemInfo
}

func NewEngine(mode EngineMode, cfg *config.Config) (*Engine, error) {
	provider, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}

	homeDir := cfg.GetSystemConfig().GetHomeDirectory()
	allowSudo := cfg.GetUserConfig().GetAllowSudo()

	return &Engine{
		mode:          mode,
		config:        cfg,
		provider:      provider,
		execMessages:  make([]Message, 0),
		chatMessages:  make([]Message, 0),
		agentMessages: make([]Message, 0),
		channel:       make(chan EngineChatStreamOutput),
		agentChannel:  make(chan AgentEvent),
		approvalChan:  make(chan bool),
		toolExecutor:  NewToolExecutor(allowSudo, homeDir),
		pipe:          "",
		running:       false,
	}, nil
}

func buildProvider(cfg *config.Config) (Provider, error) {
	aiCfg := cfg.GetAiConfig()

	switch aiCfg.GetProvider() {
	case config.ProviderAnthropic:
		return NewAnthropicProvider(aiCfg.GetKey()), nil

	case config.ProviderOpenAI, config.ProviderOpenRouter, config.ProviderMiniMax,
		config.ProviderOllama, config.ProviderLlamaCpp, config.ProviderLMStudio, config.ProviderCustom:

		apiKey := aiCfg.GetKey()
		if apiKey == "" {
			apiKey = "no-key"
		}

		return NewOpenAIProvider(OpenAIProviderConfig{
			APIKey:  apiKey,
			BaseURL: aiCfg.GetEffectiveBaseURL(),
			Proxy:   aiCfg.GetProxy(),
			Name:    aiCfg.GetProvider(),
		})

	default:
		apiKey := aiCfg.GetKey()
		if apiKey == "" {
			apiKey = "no-key"
		}
		return NewOpenAIProvider(OpenAIProviderConfig{
			APIKey:  apiKey,
			BaseURL: aiCfg.GetEffectiveBaseURL(),
			Proxy:   aiCfg.GetProxy(),
			Name:    aiCfg.GetProvider(),
		})
	}
}

func (e *Engine) SetMode(mode EngineMode) *Engine {
	e.mode = mode
	return e
}

func (e *Engine) GetMode() EngineMode {
	return e.mode
}

func (e *Engine) GetChannel() chan EngineChatStreamOutput {
	return e.channel
}

func (e *Engine) GetAgentChannel() chan AgentEvent {
	return e.agentChannel
}

func (e *Engine) SendApproval(approved bool) {
	e.approvalChan <- approved
}

func (e *Engine) GetRemoteHost() string {
	return e.remoteHost
}

func (e *Engine) GetRemoteInfo() *RemoteSystemInfo {
	return e.remoteInfo
}

func (e *Engine) SetRemoteHost(host string) error {
	e.remoteHost = host

	info, err := probeRemoteSystem(host)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", host, err)
	}

	e.remoteInfo = info
	e.toolExecutor.SetRemoteHost(host, info.HomeDir)

	return nil
}

func probeRemoteSystem(host string) (*RemoteSystemInfo, error) {
	output, err := run.CaptureSSHCommand(host, "uname -s; echo $SHELL; echo $HOME; hostname; whoami", 15*time.Second)
	if err != nil {
		return nil, err
	}
	if output.ExitCode != 0 {
		errMsg := output.Stderr
		if errMsg == "" {
			errMsg = "SSH connection failed"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	lines := strings.Split(strings.TrimSpace(output.Stdout), "\n")
	info := &RemoteSystemInfo{}
	if len(lines) > 0 {
		info.OS = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		info.Shell = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		info.HomeDir = strings.TrimSpace(lines[2])
	}
	if len(lines) > 3 {
		info.Hostname = strings.TrimSpace(lines[3])
	}
	if len(lines) > 4 {
		info.Username = strings.TrimSpace(lines[4])
	}

	return info, nil
}

func (e *Engine) SetPipe(pipe string) *Engine {
	e.pipe = pipe
	return e
}

func (e *Engine) Interrupt() *Engine {
	if e.cancelFn != nil {
		e.cancelFn()
	}

	if e.mode == AgentEngineMode {
		e.agentChannel <- AgentEvent{
			Type:    AgentEventDone,
			Content: "[Interrupt]",
		}
	} else {
		e.channel <- EngineChatStreamOutput{
			content:   "[Interrupt]",
			last:      true,
			interrupt: true,
		}
	}

	e.running = false
	return e
}

func (e *Engine) Clear() *Engine {
	switch e.mode {
	case ExecEngineMode:
		e.execMessages = []Message{}
	case AgentEngineMode:
		e.agentMessages = []Message{}
	default:
		e.chatMessages = []Message{}
	}
	return e
}

func (e *Engine) Reset() *Engine {
	e.execMessages = []Message{}
	e.chatMessages = []Message{}
	e.agentMessages = []Message{}
	return e
}

func (e *Engine) ExecCompletion(input string) (*EngineExecOutput, error) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	defer func() { e.cancelFn = nil }()

	e.running = true
	e.appendUserMessage(input)

	req := CompletionRequest{
		Model:       e.config.GetAiConfig().GetModel(),
		MaxTokens:   e.config.GetAiConfig().GetMaxTokens(),
		Temperature: e.config.GetAiConfig().GetTemperature(),
		Messages:    e.prepareCompletionMessages(),
	}

	content, err := e.provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	e.appendAssistantMessage(content)

	var output EngineExecOutput
	err = json.Unmarshal([]byte(content), &output)
	if err != nil {
		re := regexp.MustCompile(`\{.*?\}`)
		match := re.FindString(content)
		if match != "" {
			err = json.Unmarshal([]byte(match), &output)
			if err != nil {
				return nil, err
			}
		} else {
			output = EngineExecOutput{
				Command:     "",
				Explanation: content,
				Executable:  false,
			}
		}
	}

	return &output, nil
}

func (e *Engine) ChatStreamCompletion(input string) error {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	defer func() { e.cancelFn = nil }()

	e.running = true
	e.appendUserMessage(input)

	req := CompletionRequest{
		Model:       e.config.GetAiConfig().GetModel(),
		MaxTokens:   e.config.GetAiConfig().GetMaxTokens(),
		Temperature: e.config.GetAiConfig().GetTemperature(),
		Messages:    e.prepareCompletionMessages(),
	}

	streamCh := make(chan StreamChunk, 64)
	go e.provider.StreamComplete(ctx, req, streamCh)

	var output string

	for chunk := range streamCh {
		if !e.running {
			cancel()
			return nil
		}

		if chunk.Err != nil {
			e.running = false
			return chunk.Err
		}

		if chunk.Done {
			executable := false
			if e.mode == ExecEngineMode {
				if !strings.HasPrefix(output, noexec) && !strings.Contains(output, "\n") {
					executable = true
				}
			}

			e.channel <- EngineChatStreamOutput{
				content:    "",
				last:       true,
				executable: executable,
			}
			e.running = false
			e.appendAssistantMessage(output)
			return nil
		}

		output += chunk.Content
		e.channel <- EngineChatStreamOutput{
			content: chunk.Content,
			last:    false,
		}
	}

	return nil
}

func (e *Engine) appendUserMessage(content string) *Engine {
	msg := Message{Role: "user", Content: content}
	switch e.mode {
	case ExecEngineMode:
		e.execMessages = append(e.execMessages, msg)
	case AgentEngineMode:
		e.agentMessages = append(e.agentMessages, msg)
	default:
		e.chatMessages = append(e.chatMessages, msg)
	}
	return e
}

func (e *Engine) appendAssistantMessage(content string) *Engine {
	msg := Message{Role: "assistant", Content: content}
	switch e.mode {
	case ExecEngineMode:
		e.execMessages = append(e.execMessages, msg)
	case AgentEngineMode:
		e.agentMessages = append(e.agentMessages, msg)
	default:
		e.chatMessages = append(e.chatMessages, msg)
	}
	return e
}

func (e *Engine) appendAgentMessage(msg Message) {
	e.agentMessages = append(e.agentMessages, msg)
}

func (e *Engine) prepareCompletionMessages() []Message {
	messages := []Message{
		{Role: "system", Content: e.prepareSystemPrompt()},
	}

	if e.pipe != "" {
		messages = append(messages, Message{
			Role:    "user",
			Content: e.preparePipePrompt(),
		})
	}

	switch e.mode {
	case ExecEngineMode:
		messages = append(messages, e.execMessages...)
	case AgentEngineMode:
		messages = append(messages, e.agentMessages...)
	default:
		messages = append(messages, e.chatMessages...)
	}

	return messages
}

func (e *Engine) preparePipePrompt() string {
	return fmt.Sprintf("I will work on the following input: %s", e.pipe)
}

func (e *Engine) prepareSystemPrompt() string {
	var bodyPart string
	switch e.mode {
	case ExecEngineMode:
		bodyPart = e.prepareSystemPromptExecPart()
	case AgentEngineMode:
		bodyPart = e.prepareSystemPromptAgentPart()
	default:
		bodyPart = e.prepareSystemPromptChatPart()
	}

	return fmt.Sprintf("%s\n%s", bodyPart, e.prepareSystemPromptContextPart())
}

func (e *Engine) prepareSystemPromptExecPart() string {
	prompt := "Your are Yai, a powerful terminal assistant generating a JSON containing a command line for my input.\n" +
		"You will always reply using the following json structure: {\"cmd\":\"the command\", \"exp\": \"some explanation\", \"exec\": true}.\n" +
		"Your answer will always only contain the json structure, never add any advice or supplementary detail or information, even if I asked the same question before.\n" +
		"The field cmd will contain a single line command (don't use new lines, use separators like && and ; instead).\n" +
		"The field exp will contain an short explanation of the command if you managed to generate an executable command, otherwise it will contain the reason of your failure.\n" +
		"The field exec will contain true if you managed to generate an executable command, false otherwise.\n"

	if e.config.GetUserConfig().GetAllowSudo() {
		prompt += "You are allowed to use sudo when a command requires elevated privileges. " +
			"If a task clearly requires root access (installing packages, editing system files, managing services, etc.), " +
			"include sudo in the command. Always mention in the explanation that the command requires elevated privileges.\n"
	} else {
		prompt += "Do NOT use sudo in commands. If a task requires elevated privileges, " +
			"set exec to false and explain that the user needs to enable sudo in settings (ctrl+s, set USER_ALLOW_SUDO to true).\n"
	}

	prompt += "\n" +
		"Examples:\n" +
		"Me: list all files in my home dir\n" +
		"Yai: {\"cmd\":\"ls ~\", \"exp\": \"list all files in your home dir\", \"exec\": true}\n" +
		"Me: list all pods of all namespaces\n" +
		"Yai: {\"cmd\":\"kubectl get pods --all-namespaces\", \"exp\": \"list pods form all k8s namespaces\", \"exec\": true}\n" +
		"Me: how are you ?\n" +
		"Yai: {\"cmd\":\"\", \"exp\": \"I'm good thanks but I cannot generate a command for this. Use the chat mode to discuss.\", \"exec\": false}"

	return prompt
}

func (e *Engine) prepareSystemPromptChatPart() string {
	return "You are Yai a powerful terminal assistant created by github.com/ekkinox.\n" +
		"You will answer in the most helpful possible way.\n" +
		"Always format your answer in markdown format.\n\n" +
		"For example:\n" +
		"Me: What is 2+2 ?\n" +
		"Yai: The answer for `2+2` is `4`\n" +
		"Me: +2 again ?\n" +
		"Yai: The answer is `6`\n"
}

func (e *Engine) prepareSystemPromptAgentPart() string {
	prompt := "You are Yai, an autonomous terminal agent. You help the user accomplish tasks by using the tools available to you.\n\n"

	if e.remoteHost != "" {
		prompt += fmt.Sprintf("IMPORTANT: You are operating on a REMOTE host via SSH (%s).\n", e.remoteHost)
		prompt += "All commands, file reads, file writes, and directory listings execute on the remote system, NOT the local machine.\n"
		if e.remoteInfo != nil {
			prompt += fmt.Sprintf("Remote system: %s", e.remoteInfo.Hostname)
			if e.remoteInfo.OS != "" {
				prompt += fmt.Sprintf(", OS: %s", e.remoteInfo.OS)
			}
			if e.remoteInfo.Username != "" {
				prompt += fmt.Sprintf(", user: %s", e.remoteInfo.Username)
			}
			prompt += "\n"
		}
		prompt += "\n"
	}

	prompt += "You have access to tools that let you run shell commands, read and write files, and list directories.\n" +
		"When given a task, break it down into steps and use your tools to complete it.\n" +
		"After each tool call, observe the result and decide what to do next.\n" +
		"Continue until the task is fully complete, then provide a brief summary of what you did.\n\n" +
		"Guidelines:\n" +
		"- Prefer small, incremental commands so you can observe results and adjust.\n" +
		"- If a command fails, read the error output and try a different approach.\n" +
		"- Always explain your reasoning briefly before using a tool.\n" +
		"- When the task is complete, respond with a text summary (no tool calls).\n" +
		"- Be careful with destructive operations (rm -rf, overwriting files). Explain the risk when relevant.\n"

	if !e.config.GetUserConfig().GetAllowSudo() {
		prompt += "- Do NOT use sudo in commands. If elevated privileges are needed, inform the user.\n"
	} else {
		prompt += "- You may use sudo when commands require elevated privileges.\n"
	}

	return prompt
}

func (e *Engine) AgentCompletion(input string, autoExecute bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	defer func() { e.cancelFn = nil }()

	e.running = true
	e.appendUserMessage(input)

	const maxIterations = 50

	for i := 0; i < maxIterations; i++ {
		if !e.running {
			return nil
		}

		req := CompletionRequest{
			Model:       e.config.GetAiConfig().GetModel(),
			MaxTokens:   e.config.GetAiConfig().GetMaxTokens(),
			Temperature: e.config.GetAiConfig().GetTemperature(),
			Messages:    e.prepareCompletionMessages(),
			Tools:       AgentTools(),
		}

		resp, err := e.provider.CompleteWithTools(ctx, req)
		if err != nil {
			e.running = false
			e.agentChannel <- AgentEvent{Type: AgentEventError, Error: err}
			e.agentChannel <- AgentEvent{Type: AgentEventDone}
			return err
		}

		if len(resp.ToolCalls) == 0 {
			e.appendAgentMessage(resp)
			e.agentChannel <- AgentEvent{Type: AgentEventAnswer, Content: resp.Content}
			e.agentChannel <- AgentEvent{Type: AgentEventDone}
			e.running = false
			return nil
		}

		if resp.Content != "" {
			e.agentChannel <- AgentEvent{Type: AgentEventThinking, Content: resp.Content}
		}

		e.appendAgentMessage(resp)

		for _, tc := range resp.ToolCalls {
			if !e.running {
				return nil
			}

			e.agentChannel <- AgentEvent{Type: AgentEventToolCall, ToolCall: &tc}

			if !autoExecute {
				e.agentChannel <- AgentEvent{Type: AgentEventApprovalRequired, ToolCall: &tc}
				approved := <-e.approvalChan
				if !approved {
					result := ToolResult{
						ToolCallID: tc.ID,
						Content:    "The user declined to execute this tool call.",
					}
					e.appendAgentMessage(Message{
						Role:       "tool",
						Content:    result.Content,
						ToolCallID: result.ToolCallID,
					})
					e.agentChannel <- AgentEvent{Type: AgentEventToolResult, ToolResult: &result}
					continue
				}
			}

			result := e.toolExecutor.Execute(tc)
			e.appendAgentMessage(Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
			e.agentChannel <- AgentEvent{Type: AgentEventToolResult, ToolResult: &result}
		}
	}

	e.running = false
	e.agentChannel <- AgentEvent{
		Type:    AgentEventAnswer,
		Content: "Reached the maximum number of iterations. Please provide further instructions if the task is not complete.",
	}
	e.agentChannel <- AgentEvent{Type: AgentEventDone}
	return nil
}

func (e *Engine) prepareSystemPromptContextPart() string {
	if e.remoteHost != "" && e.remoteInfo != nil {
		part := fmt.Sprintf("Remote context (SSH target: %s): ", e.remoteHost)
		if e.remoteInfo.OS != "" {
			part += fmt.Sprintf("operating system is %s, ", e.remoteInfo.OS)
		}
		if e.remoteInfo.Hostname != "" {
			part += fmt.Sprintf("hostname is %s, ", e.remoteInfo.Hostname)
		}
		if e.remoteInfo.HomeDir != "" {
			part += fmt.Sprintf("home directory is %s, ", e.remoteInfo.HomeDir)
		}
		if e.remoteInfo.Shell != "" {
			part += fmt.Sprintf("shell is %s, ", e.remoteInfo.Shell)
		}
		if e.remoteInfo.Username != "" {
			part += fmt.Sprintf("user is %s, ", e.remoteInfo.Username)
		}
		part += "take this into account. "
		if e.config.GetUserConfig().GetPreferences() != "" {
			part += fmt.Sprintf("Also, %s.", e.config.GetUserConfig().GetPreferences())
		}
		return part
	}

	part := "My context: "

	if e.config.GetSystemConfig().GetOperatingSystem() != system.UnknownOperatingSystem {
		part += fmt.Sprintf("my operating system is %s, ", e.config.GetSystemConfig().GetOperatingSystem().String())
	}
	if e.config.GetSystemConfig().GetDistribution() != "" {
		part += fmt.Sprintf("my distribution is %s, ", e.config.GetSystemConfig().GetDistribution())
	}
	if e.config.GetSystemConfig().GetHomeDirectory() != "" {
		part += fmt.Sprintf("my home directory is %s, ", e.config.GetSystemConfig().GetHomeDirectory())
	}
	if e.config.GetSystemConfig().GetShell() != "" {
		part += fmt.Sprintf("my shell is %s, ", e.config.GetSystemConfig().GetShell())
	}
	if e.config.GetSystemConfig().GetShell() != "" {
		part += fmt.Sprintf("my editor is %s, ", e.config.GetSystemConfig().GetEditor())
	}
	part += "take this into account. "

	if e.config.GetUserConfig().GetPreferences() != "" {
		part += fmt.Sprintf("Also, %s.", e.config.GetUserConfig().GetPreferences())
	}

	return part
}
