package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"os"
	"os/exec"
	"path/filepath"

	"github.com/bearstonem/helm/agent"
	"github.com/bearstonem/helm/backup"
	"github.com/bearstonem/helm/config"
	"github.com/bearstonem/helm/hook"
	"github.com/bearstonem/helm/integration"
	"github.com/bearstonem/helm/memory"
	"github.com/bearstonem/helm/run"
	"github.com/bearstonem/helm/session"
	"github.com/bearstonem/helm/system"
)

const noexec = "[noexec]"

type RemoteSystemInfo struct {
	OS            string
	Hostname      string
	Shell         string
	HomeDir       string
	Username      string
	CurrentDir    string
	WorkspaceRoot string
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
	session       *session.Session
	onUsage       func(inputTokens, outputTokens int)
	modelOverride   string // runtime model override; empty = use config
	agentProfile    *agent.Profile // custom agent profile; nil = default
	memoryStore     *memory.Store
	embedder        memory.EmbeddingProvider
	homeDir         string
	delegationChain []string // agent IDs in the delegation path; empty = primary
	escalationChan  chan string // receives user response to escalation
}

func NewEngine(mode EngineMode, cfg *config.Config) (*Engine, error) {
	provider, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}

	homeDir := cfg.GetSystemConfig().GetHomeDirectory()
	workDir := cfg.GetSystemConfig().GetWorkspaceRoot()
	if workDir == "" {
		workDir = cfg.GetSystemConfig().GetCurrentDirectory()
	}
	allowSudo := cfg.GetUserConfig().GetAllowSudo()
	permMode := cfg.GetUserConfig().GetPermissionMode()

	// Initialize memory store (non-fatal if it fails)
	var memStore *memory.Store
	var embedder memory.EmbeddingProvider
	te := newToolExecutorWithHooksAndIntegrations(allowSudo, homeDir, workDir, permMode, cfg.GetUserConfig().GetHooks(), cfg.GetUserConfig().GetIntegrations())
	if store, err := memory.Open(homeDir); err == nil {
		memStore = store
		embedder = buildEmbedder(cfg)
		// Wire skill indexing
		if embedder != nil {
			s, emb := store, embedder
			te.SetOnSkillChange(func(action, name, description string) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				switch action {
				case "create":
					s.IndexSkill(ctx, emb, name, description)
				case "remove":
					s.RemoveSkill(name)
				}
			})
		}
	}

	e := &Engine{
		mode:          mode,
		config:        cfg,
		provider:      provider,
		execMessages:  make([]Message, 0),
		chatMessages:  make([]Message, 0),
		agentMessages: make([]Message, 0),
		channel:       make(chan EngineChatStreamOutput),
		agentChannel:  make(chan AgentEvent),
		approvalChan:  make(chan bool),
		toolExecutor:  te,
		pipe:          "",
		running:       false,
		memoryStore:    memStore,
		embedder:       embedder,
		homeDir:        homeDir,
		escalationChan: make(chan string),
	}

	// Wire agent management callbacks
	te.SetOnCreateAgent(e.executeCreateAgent)
	te.SetOnDelegateTask(e.executeDelegation)
	te.SetOnEscalateToUser(e.executeEscalation)
	te.SetOnRestartHelm(e.executeRestart)

	return e, nil
}

func newToolExecutorWithHooksAndIntegrations(allowSudo bool, homeDir, workDir string, permMode config.PermissionMode, hooks []config.HookConfig, integrations []config.IntegrationConfig) *ToolExecutor {
	te := NewToolExecutor(allowSudo, homeDir, workDir, permMode)
	if len(hooks) > 0 {
		te.SetHookRunner(hook.NewRunner(hooks, workDir))
	}
	if len(integrations) > 0 {
		te.SetIntegrations(integration.BuildTools(integrations))
	}
	te.LoadSkills()
	return te
}

func buildProvider(cfg *config.Config) (Provider, error) {
	aiCfg := cfg.GetAiConfig()
	provider := aiCfg.GetProvider()
	apiKey := config.ResolveAPIKey(provider, aiCfg.GetKey())

	switch provider {
	case config.ProviderAnthropic:
		return NewAnthropicProvider(apiKey), nil

	default:
		if apiKey == "" {
			apiKey = "no-key"
		}
		return NewOpenAIProvider(OpenAIProviderConfig{
			APIKey:  apiKey,
			BaseURL: aiCfg.GetEffectiveBaseURL(),
			Proxy:   aiCfg.GetProxy(),
			Name:    provider,
		})
	}
}

func buildEmbedder(cfg *config.Config) memory.EmbeddingProvider {
	aiCfg := cfg.GetAiConfig()
	provider := aiCfg.GetProvider()

	// If using OpenAI or OpenRouter, use OpenAI embeddings
	switch provider {
	case config.ProviderOpenAI:
		key := config.ResolveAPIKey(provider, aiCfg.GetKey())
		if key != "" {
			return memory.NewOpenAIEmbedder(key)
		}
	case config.ProviderOpenRouter:
		// OpenRouter doesn't support embeddings; try OpenAI key
		if key := config.ResolveAPIKey(config.ProviderOpenAI, ""); key != "" {
			return memory.NewOpenAIEmbedder(key)
		}
	case config.ProviderAnthropic:
		// Anthropic doesn't have embeddings API; try OpenAI key
		if key := config.ResolveAPIKey(config.ProviderOpenAI, ""); key != "" {
			return memory.NewOpenAIEmbedder(key)
		}
	}

	// Fallback: try Ollama locally
	return memory.NewOllamaEmbedder("", "")
}

// GetToolExecutor returns the tool executor.
func (e *Engine) GetToolExecutor() *ToolExecutor {
	return e.toolExecutor
}

// GetMemoryStore returns the memory store (may be nil).
func (e *Engine) GetMemoryStore() *memory.Store {
	return e.memoryStore
}

// GetEmbedder returns the embedding provider (may be nil).
func (e *Engine) GetEmbedder() memory.EmbeddingProvider {
	return e.embedder
}

// IndexMessage indexes a message into the memory store in the background.
func (e *Engine) IndexMessage(sessionID, role, content string) {
	if e.memoryStore == nil || e.embedder == nil || content == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e.memoryStore.IndexMessage(ctx, e.embedder, sessionID, role, content)
	}()
}

// IndexSession indexes a session summary into the memory store.
func (e *Engine) IndexSession(sessionID, summary, mode string) {
	if e.memoryStore == nil || e.embedder == nil || summary == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e.memoryStore.IndexSession(ctx, e.embedder, sessionID, summary, mode)
	}()
}

// RecallContext retrieves relevant past messages for the current query.
func (e *Engine) RecallContext(query string, k int) string {
	if e.memoryStore == nil || e.embedder == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := e.memoryStore.SearchMessages(ctx, e.embedder, query, k)
	if err != nil || len(messages) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Relevant past conversations\n")
	b.WriteString("The following are excerpts from past interactions that may be relevant:\n\n")
	for _, m := range messages {
		if m.Distance > 0.7 { // skip low-relevance results
			continue
		}
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("- [%s] %s\n", m.Role, content))
	}

	result := b.String()
	if result == "# Relevant past conversations\nThe following are excerpts from past interactions that may be relevant:\n\n" {
		return "" // nothing relevant
	}
	return result
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
	e.toolExecutor.SetRemoteHost(host, info.HomeDir, info.WorkspaceRoot)

	return nil
}

func probeRemoteSystem(host string) (*RemoteSystemInfo, error) {
	// Keep this lightweight: gather basic info + current dir + best-effort workspace root.
	output, err := run.CaptureSSHCommand(host, "uname -s; echo $SHELL; echo $HOME; hostname; whoami; pwd; (git rev-parse --show-toplevel 2>/dev/null || pwd)", 15*time.Second)
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
	if len(lines) > 5 {
		info.CurrentDir = strings.TrimSpace(lines[5])
	}
	if len(lines) > 6 {
		info.WorkspaceRoot = strings.TrimSpace(lines[6])
	}

	return info, nil
}

func (e *Engine) GetModel() string {
	if e.modelOverride != "" {
		return e.modelOverride
	}
	return e.config.GetAiConfig().GetModel()
}

func (e *Engine) SetModel(model string) {
	e.modelOverride = model
}

func (e *Engine) GetProvider() Provider {
	return e.provider
}

func (e *Engine) SwitchProvider(provider string, apiKey string, baseURL string) error {
	cfg := OpenAIProviderConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Name:    provider,
	}
	if apiKey == "" {
		cfg.APIKey = "no-key"
	}

	if provider == config.ProviderAnthropic {
		e.provider = NewAnthropicProvider(apiKey)
		return nil
	}

	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		return err
	}
	e.provider = p
	return nil
}

func (e *Engine) ReloadIntegrations(integrations []config.IntegrationConfig) {
	if len(integrations) > 0 {
		e.toolExecutor.SetIntegrations(integration.BuildTools(integrations))
	} else {
		e.toolExecutor.SetIntegrations(nil)
	}
}

func (e *Engine) SetOnUsage(fn func(inputTokens, outputTokens int)) {
	e.onUsage = fn
}

func (e *Engine) reportUsage(input, output int) {
	if e.onUsage != nil && (input > 0 || output > 0) {
		e.onUsage(input, output)
	}
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

// Session management

func (e *Engine) GetSession() *session.Session {
	return e.session
}

func (e *Engine) SetSession(s *session.Session) {
	e.session = s
}

// SetAgentProfile configures a custom agent profile that overrides
// the system prompt and optionally restricts the available tools.
func (e *Engine) SetAgentProfile(p *agent.Profile) {
	e.agentProfile = p
	if p != nil && p.Model != "" {
		e.modelOverride = p.Model
	} else if p == nil {
		e.modelOverride = ""
	}
}

func (e *Engine) StartNewSession() {
	mode := "exec"
	switch e.mode {
	case ChatEngineMode:
		mode = "chat"
	case AgentEngineMode:
		mode = "agent"
	}
	e.session = session.NewSession(mode)
}

// SaveSession persists the current session to disk. Call after each interaction.
func (e *Engine) SaveSession(homeDir string) error {
	if e.session == nil {
		return nil
	}
	e.session.Messages = e.exportMessages()
	e.session.Mode = e.modeString()
	err := e.session.Save(homeDir)
	if err == nil {
		e.IndexSession(e.session.ID, e.session.Summary, e.session.Mode)
	}
	return err
}

// LoadSession restores messages from a saved session.
func (e *Engine) LoadSession(homeDir, id string) error {
	s, err := session.Load(homeDir, id)
	if err != nil {
		return err
	}
	e.session = s
	e.importMessages(s.Messages)
	return nil
}

func (e *Engine) modeString() string {
	switch e.mode {
	case ChatEngineMode:
		return "chat"
	case AgentEngineMode:
		return "agent"
	default:
		return "exec"
	}
}

func (e *Engine) exportMessages() []session.Message {
	var msgs []Message
	switch e.mode {
	case ExecEngineMode:
		msgs = e.execMessages
	case AgentEngineMode:
		msgs = e.agentMessages
	default:
		msgs = e.chatMessages
	}

	out := make([]session.Message, len(msgs))
	for i, m := range msgs {
		out[i] = session.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			out[i].ToolCalls = append(out[i].ToolCalls, session.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
	}
	return out
}

func (e *Engine) importMessages(msgs []session.Message) {
	converted := make([]Message, len(msgs))
	for i, m := range msgs {
		converted[i] = Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			converted[i].ToolCalls = append(converted[i].ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
	}

	switch e.mode {
	case ExecEngineMode:
		e.execMessages = converted
	case AgentEngineMode:
		e.agentMessages = converted
	default:
		e.chatMessages = converted
	}
}

func (e *Engine) ExecCompletion(input string) (*EngineExecOutput, error) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	defer func() { e.cancelFn = nil }()

	e.running = true
	e.appendUserMessage(input)

	req := CompletionRequest{
		Model:       e.GetModel(),
		MaxTokens:   e.config.GetAiConfig().GetMaxTokens(),
		Temperature: e.config.GetAiConfig().GetTemperature(),
		Messages:    e.prepareCompletionMessages(),
	}

	content, err := e.provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	u := e.provider.LastUsage()
	e.reportUsage(u.InputTokens, u.OutputTokens)

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
		Model:       e.GetModel(),
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
	// Index for memory recall
	sid := ""
	if e.session != nil {
		sid = e.session.ID
	}
	e.IndexMessage(sid, "user", content)
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
	// Index for memory recall
	sid := ""
	if e.session != nil {
		sid = e.session.ID
	}
	e.IndexMessage(sid, "assistant", content)
	return e
}

func (e *Engine) appendAgentMessage(msg Message) {
	e.agentMessages = append(e.agentMessages, msg)
}

func sanitizeToolCalls(msg Message) (Message, int) {
	if len(msg.ToolCalls) == 0 {
		return msg, 0
	}
	valid := make([]ToolCall, 0, len(msg.ToolCalls))
	dropped := 0
	for _, tc := range msg.ToolCalls {
		// Provider APIs may reject tool calls whose arguments aren't valid JSON.
		if tc.Arguments != "" && !json.Valid([]byte(tc.Arguments)) {
			dropped++
			continue
		}
		valid = append(valid, tc)
	}
	msg.ToolCalls = valid
	return msg, dropped
}

func (e *Engine) prepareCompletionMessages() []Message {
	systemPrompt := e.prepareSystemPrompt()

	// Inject memory recall for agent/chat modes
	if e.mode == AgentEngineMode || e.mode == ChatEngineMode {
		var lastUserMsg string
		msgs := e.agentMessages
		if e.mode == ChatEngineMode {
			msgs = e.chatMessages
		}
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" && msgs[i].Content != "" {
				lastUserMsg = msgs[i].Content
				break
			}
		}
		if lastUserMsg != "" {
			recalled := e.RecallContext(lastUserMsg, 5)
			if recalled != "" {
				systemPrompt += "\n" + recalled
			}
		}
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
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
		// Avoid sending invalid tool calls back to the provider (can 400 the request).
		for _, m := range e.agentMessages {
			clean, _ := sanitizeToolCalls(m)
			messages = append(messages, clean)
		}
	default:
		messages = append(messages, e.chatMessages...)
	}

	return messages
}

func (e *Engine) preparePipePrompt() string {
	return fmt.Sprintf("I will work on the following input: %s", e.pipe)
}

func (e *Engine) prepareSystemPrompt() string {
	// If a custom agent profile is set, use its system prompt
	if e.agentProfile != nil && e.agentProfile.SystemPrompt != "" {
		prompt := e.agentProfile.SystemPrompt
		prompt += "\n" + e.prepareSystemPromptContextPart()
		return prompt
	}

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
	prompt := "You are Helm, a powerful terminal assistant generating a JSON containing a command line for my input.\n" +
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
		"Helm: {\"cmd\":\"ls ~\", \"exp\": \"list all files in your home dir\", \"exec\": true}\n" +
		"Me: list all pods of all namespaces\n" +
		"Helm: {\"cmd\":\"kubectl get pods --all-namespaces\", \"exp\": \"list pods form all k8s namespaces\", \"exec\": true}\n" +
		"Me: how are you ?\n" +
		"Helm: {\"cmd\":\"\", \"exp\": \"I'm good thanks but I cannot generate a command for this. Use the chat mode to discuss.\", \"exec\": false}"

	return prompt
}

func (e *Engine) prepareSystemPromptChatPart() string {
	return "You are Helm a powerful terminal assistant created by github.com/bearstonem.\n" +
		"You will answer in the most helpful possible way.\n" +
		"Always format your answer in markdown format.\n\n" +
		"For example:\n" +
		"Me: What is 2+2 ?\n" +
		"Helm: The answer for `2+2` is `4`\n" +
		"Me: +2 again ?\n" +
		"Helm: The answer is `6`\n"
}

func (e *Engine) prepareSystemPromptAgentPart() string {
	prompt := "You are Helm, an autonomous terminal agent.\n\n"

	// DELEGATION FIRST — this is the primary agent's most important instruction
	if len(e.delegationChain) == 0 {
		delegationBlock := e.prepareAvailableAgents()
		if delegationBlock != "" {
			prompt += delegationBlock + "\n"
		}
	} else {
		// Sub-agent: collaborative instructions
		prompt += e.prepareAvailableAgents() + "\n"
	}

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

	prompt += `# Tools
You have access to tools for shell commands, file operations, and search:
- run_command: Execute shell commands. Use for running programs, builds, tests, git operations, etc.
- read_file: Read file contents. Always read a file before editing it.
- edit_file: Make targeted search-and-replace edits to existing files. Preferred over write_file for modifications.
- write_file: Create new files or completely overwrite existing ones.
- list_directory: List directory contents with metadata.
- search_files: Search file contents using regex patterns (like grep). Use instead of run_command with grep.
- find_files: Find files by name pattern using glob matching. Use instead of run_command with find.

# Skills (self-created tools)
You can create, list, and remove persistent skills — reusable tools that you build yourself:
- create_skill: Create a new skill from a script. The script receives JSON arguments via stdin and prints output to stdout. Skills persist across sessions and become part of your tool inventory.
- list_skills: Show all user-created skills.
- remove_skill: Delete a skill.
When the user asks you to "learn" an API, "add a capability", or "create a tool for X", use create_skill. Write a robust script that handles errors and edge cases. Test it after creation.
Skills prefixed with skill_ appear as regular tools you can call.

# Approach
- Understand before acting: read relevant files and explore the codebase before making changes.
- Break tasks into small steps and verify each step before proceeding.
- After each tool call, observe the result and decide what to do next.
- If a command fails, read the error carefully and try a different approach. Don't retry the same thing blindly.
- Continue until the task is fully complete, then provide a brief summary.

# Editing files
- Always read_file before editing to understand the current content.
- Prefer edit_file (search-and-replace) over write_file for modifying existing files — it's safer and shows intent.
- When using edit_file, include enough context in old_string to match uniquely.
- Only use write_file for creating new files or when the entire file needs to be rewritten.

# Code quality
- Don't add features, refactor code, or make improvements beyond what was asked.
- Don't add error handling, comments, or type annotations to code you didn't change.
- Be careful not to introduce security vulnerabilities (command injection, XSS, SQL injection, etc.).
- Prefer simple, direct solutions over clever abstractions.

# Long-running processes
- NEVER start dev servers, file watchers, or daemons with run_command — they will hang until timeout (60s).
- Build and test only. If the user wants to run a server, tell them to do it manually.
- Use background processes (&) only if you don't need the output.

# Safety
- Be careful with destructive operations (rm -rf, git reset --hard, DROP TABLE). Explain the risk.
- Don't overwrite files without reading them first.
- Prefer creating new git commits over amending existing ones.
- When in doubt about a risky action, explain what you'd do and why before doing it.
`

	if !e.config.GetUserConfig().GetAllowSudo() {
		prompt += "- Do NOT use sudo in commands. If elevated privileges are needed, inform the user.\n"
	} else {
		prompt += "- You may use sudo when commands require elevated privileges.\n"
	}

	return prompt
}

// agentTools returns the tool set for the current agent, filtered by profile if set.
// Sub-agents can delegate to other agents but not create new ones.
func (e *Engine) agentTools() []Tool {
	all := e.toolExecutor.AllTools()

	// Determine self agent_* tool name to exclude
	selfToolName := ""
	if e.agentProfile != nil {
		selfToolName = "agent_" + e.agentProfile.ID
	}

	var tools []Tool
	if e.agentProfile != nil && len(e.agentProfile.Tools) > 0 {
		allowed := make(map[string]bool, len(e.agentProfile.Tools))
		for _, name := range e.agentProfile.Tools {
			allowed[name] = true
		}
		for _, t := range all {
			if allowed[t.Name] && t.Name != selfToolName {
				tools = append(tools, t)
			}
		}
	} else {
		for _, t := range all {
			if t.Name != selfToolName {
				tools = append(tools, t)
			}
		}
	}

	// Sub-agents can delegate but not create new agents
	if len(e.delegationChain) > 0 {
		var filtered []Tool
		for _, t := range tools {
			if t.Name != "create_agent" {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}

	return tools
}

// executeCreateAgent creates or updates a persistent sub-agent profile.
// If an agent with the same name already exists, it is updated in place
// while preserving its CreatedAt timestamp.
func (e *Engine) executeCreateAgent(name, description, systemPrompt string, tools []string) (string, error) {
	p := &agent.Profile{
		Name:         name,
		Description:  description,
		SystemPrompt: systemPrompt,
		Tools:        tools,
	}

	// Preserve CreatedAt if updating an existing agent
	id := agent.SanitizeID(name)
	if existing, err := agent.Load(e.homeDir, id); err == nil {
		p.CreatedAt = existing.CreatedAt
	}

	if err := agent.Save(e.homeDir, p); err != nil {
		return "", err
	}
	return fmt.Sprintf("Agent %q created (id: %s). You can now delegate tasks to it with delegate_task.", name, p.ID), nil
}

// executeDelegation runs a sub-agent with the given profile and task.
// It forwards sub-agent events to the parent's channel and returns the final answer.
// Uses cycle detection instead of depth limits to allow collaborative multi-agent workflows.
func (e *Engine) executeDelegation(agentID, task, sharedContext string) (string, error) {
	// Cycle detection: check if this agent is already in the chain
	for _, id := range e.delegationChain {
		if id == agentID {
			return "", fmt.Errorf("cycle detected: agent %q is already in the delegation chain %v", agentID, e.delegationChain)
		}
	}
	// Safety cap on chain length
	if len(e.delegationChain) >= 6 {
		return "", fmt.Errorf("delegation chain too deep (%d agents): %v", len(e.delegationChain), e.delegationChain)
	}

	profile, err := agent.Load(e.homeDir, agentID)
	if err != nil {
		return "", fmt.Errorf("agent %q not found: %w", agentID, err)
	}

	// Notify: sub-agent starting
	e.agentChannel <- AgentEvent{
		Type:      AgentEventSubAgentStart,
		AgentID:   agentID,
		AgentName: profile.Name,
		Content:   task,
	}

	// Create independent sub-engine
	subEngine, err := NewEngine(AgentEngineMode, e.config)
	if err != nil {
		return "", fmt.Errorf("creating sub-engine: %w", err)
	}
	subEngine.SetAgentProfile(profile)

	// Build the delegation chain for the sub-engine
	myID := "__primary__"
	if e.agentProfile != nil {
		myID = e.agentProfile.ID
	}
	subEngine.delegationChain = append(append([]string{}, e.delegationChain...), myID)
	subEngine.StartNewSession()

	// Inject shared context if provided
	if sharedContext != "" {
		subEngine.appendUserMessage("## Context from delegating agent\n" + sharedContext)
		subEngine.appendAgentMessage(Message{Role: "assistant", Content: "Understood. I have the shared context. Proceeding with the task."})
	}

	// Run sub-agent in background; close channel when done so forwarding loop exits
	done := make(chan error, 1)
	go func() {
		done <- subEngine.AgentCompletion(task, true)
		close(subEngine.agentChannel)
	}()

	// Forward sub-agent events to parent channel, tagged with agent identity
	var finalAnswer string
	subCh := subEngine.GetAgentChannel()
	for event := range subCh {
		event.AgentID = agentID
		event.AgentName = profile.Name

		switch event.Type {
		case AgentEventAnswer:
			finalAnswer = event.Content
			e.agentChannel <- event
		case AgentEventDone:
			// Don't forward Done — sub-agent done != parent done
		case AgentEventEscalation:
			// Forward escalation to parent's channel (bubbles up to user)
			e.agentChannel <- event
			// Relay user's response back down to the sub-agent
			go func() {
				response := <-e.escalationChan
				subEngine.RespondToEscalation(response)
			}()
		default:
			e.agentChannel <- event
		}
	}

	// Wait for completion
	if err := <-done; err != nil {
		e.agentChannel <- AgentEvent{
			Type:      AgentEventSubAgentDone,
			AgentID:   agentID,
			AgentName: profile.Name,
			Content:   "failed: " + err.Error(),
		}
		return "", err
	}

	e.agentChannel <- AgentEvent{
		Type:      AgentEventSubAgentDone,
		AgentID:   agentID,
		AgentName: profile.Name,
		Content:   "completed",
	}

	if finalAnswer == "" {
		finalAnswer = "(sub-agent completed without a final answer)"
	}
	return finalAnswer, nil
}

// executeEscalation pauses the agent and asks the user a question.
func (e *Engine) executeEscalation(question, context string) (string, error) {
	content := question
	if context != "" {
		content = question + "\n\nContext: " + context
	}

	agentID := ""
	agentName := ""
	if e.agentProfile != nil {
		agentID = e.agentProfile.ID
		agentName = e.agentProfile.Name
	}

	e.agentChannel <- AgentEvent{
		Type:      AgentEventEscalation,
		Content:   content,
		AgentID:   agentID,
		AgentName: agentName,
	}

	// Block until the user responds
	response := <-e.escalationChan
	return response, nil
}

// RespondToEscalation sends the user's response to a blocked escalation.
func (e *Engine) RespondToEscalation(response string) {
	e.escalationChan <- response
}

// executeRestart triggers the restart script to rebuild and relaunch Helm.
func (e *Engine) executeRestart(reason string) (string, error) {
	scriptPath := filepath.Join(backup.BackupsDir(e.homeDir), "restart.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", fmt.Errorf("restart script not found at %s — run from GUI mode to generate it", scriptPath)
	}

	// Launch restart script in background (it will kill this process)
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to launch restart script: %w", err)
	}

	return fmt.Sprintf("Restart initiated (reason: %s). The application will stop, rebuild, and relaunch. If the build fails, the latest backup will be restored automatically.", reason), nil
}

// prepareAvailableAgents returns a prompt section listing available agents for collaboration.
func (e *Engine) prepareAvailableAgents() string {
	var b strings.Builder
	isPrimary := len(e.delegationChain) == 0

	agents, _ := agent.LoadAll(e.homeDir)

	// Filter out self
	myID := ""
	if e.agentProfile != nil {
		myID = e.agentProfile.ID
	}
	var otherAgents []agent.Profile
	for _, a := range agents {
		if a.ID != myID {
			otherAgents = append(otherAgents, a)
		}
	}

	// List available skills (skill_* tools)
	var skillNames []string
	for _, t := range e.toolExecutor.AllTools() {
		if len(t.Name) > 6 && t.Name[:6] == "skill_" {
			skillNames = append(skillNames, t.Name)
		}
	}

	if isPrimary {
		b.WriteString("\n# DELEGATION — IMPORTANT\n")
		b.WriteString("You are an orchestrator. Your PRIMARY job is to delegate work to specialized sub-agents, NOT to do work yourself.\n\n")

		if len(otherAgents) > 0 {
			b.WriteString("## Available Sub-Agents:\n")
			b.WriteString("Each agent is also available as a direct tool (`agent_{id}`) that you can assign to other agents.\n\n")
			for _, a := range otherAgents {
				tools := "all tools"
				if len(a.Tools) > 0 {
					tools = fmt.Sprintf("%d tools: %s", len(a.Tools), strings.Join(a.Tools, ", "))
				}
				b.WriteString(fmt.Sprintf("- **%s** (id: `%s`, %s): %s\n", a.Name, a.ID, tools, a.Description))
			}
			b.WriteString("\n")
		} else {
			b.WriteString("No sub-agents exist yet. Create one before doing any substantial work.\n\n")
		}

		if len(skillNames) > 0 {
			b.WriteString("## Available Skills:\n")
			b.WriteString("These skill_* tools can be assigned to agents when creating them: ")
			b.WriteString(strings.Join(skillNames, ", "))
			b.WriteString("\n\n")
		}

		b.WriteString("## Rules:\n")
		b.WriteString("1. If a sub-agent matches the request, delegate IMMEDIATELY with `delegate_task`.\n")
		b.WriteString("2. For complex tasks, delegate to MULTIPLE agents CONCURRENTLY (multiple `delegate_task` calls in one response).\n")
		b.WriteString("3. If NO sub-agent matches, CREATE one first with `create_agent` (give it a good system prompt and assign relevant tools including skill_* tools), then delegate to it.\n")
		b.WriteString("4. If a task needs a capability that doesn't exist as a skill, create it with `create_skill` first, then assign it to the agent.\n")
		b.WriteString("5. Use `escalate_to_user` to ask the user for information you need (API keys, preferences, credentials, clarifications).\n")
		b.WriteString("6. Only do work yourself for simple questions, clarifications, or trivial tasks.\n")
		b.WriteString("7. When creating agents, include relevant skill_* tools in the tools list so the agent can use them.\n")
		b.WriteString("8. Pass relevant context via the `context` parameter when delegating.\n")
	} else {
		b.WriteString("\n# COLLABORATION\n")
		b.WriteString("You can collaborate with other agents and the user:\n\n")

		if len(otherAgents) > 0 {
			b.WriteString("## Other Agents:\n")
			for _, a := range otherAgents {
				b.WriteString(fmt.Sprintf("- **%s** (id: `%s`): %s\n", a.Name, a.ID, a.Description))
			}
			b.WriteString("\n")
		}

		if len(skillNames) > 0 {
			b.WriteString("## Available Skills: ")
			b.WriteString(strings.Join(skillNames, ", "))
			b.WriteString("\n\n")
		}

		b.WriteString("## Rules:\n")
		b.WriteString("- Use `delegate_task` to ask another agent for help when needed.\n")
		b.WriteString("- Use `escalate_to_user` when you need clarification, approval, or information from the user.\n")
		b.WriteString("- If you need a skill that doesn't exist, use `create_skill` to build it.\n")
		b.WriteString("- Pass context when delegating so the other agent understands the situation.\n")
	}

	return b.String()
}

func (e *Engine) AgentCompletion(input string, autoExecute bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	defer func() { e.cancelFn = nil }()

	e.running = true
	e.appendUserMessage(input)

	maxIterations := 50
	if len(e.delegationChain) > 0 {
		maxIterations = 200 // sub-agents get more room to work on complex tasks
	}

	for i := 0; i < maxIterations; i++ {
		if !e.running {
			return nil
		}

		req := CompletionRequest{
			Model:       e.GetModel(),
			MaxTokens:   e.config.GetAiConfig().GetMaxTokens(),
			Temperature: e.config.GetAiConfig().GetTemperature(),
			Messages:    e.prepareCompletionMessages(),
			Tools:       e.agentTools(),
		}

		resp, err := e.provider.CompleteWithTools(ctx, req)
		if err != nil {
			e.running = false
			e.agentChannel <- AgentEvent{Type: AgentEventError, Error: err}
			e.agentChannel <- AgentEvent{Type: AgentEventDone}
			return err
		}

		u := e.provider.LastUsage()
		e.reportUsage(u.InputTokens, u.OutputTokens)

		// If the model produced malformed tool-call JSON, don't persist it in history.
		// Instead, ask it to retry with a valid JSON tool call.
		cleanResp, dropped := sanitizeToolCalls(resp)
		if dropped > 0 {
			notice := fmt.Sprintf("Your last tool call had invalid JSON arguments and was rejected. Please retry the tool call with valid JSON arguments only (no truncation). If writing a large file, prefer write_file with content_lines and split into multiple calls if needed.")
			e.appendAgentMessage(Message{Role: "assistant", Content: notice})
			e.appendUserMessage(notice)
			e.agentChannel <- AgentEvent{Type: AgentEventThinking, Content: notice}
			continue
		}

		if len(cleanResp.ToolCalls) == 0 {
			e.appendAgentMessage(cleanResp)
			e.agentChannel <- AgentEvent{Type: AgentEventAnswer, Content: cleanResp.Content}
			e.agentChannel <- AgentEvent{Type: AgentEventDone}
			e.running = false
			return nil
		}

		if cleanResp.Content != "" {
			e.agentChannel <- AgentEvent{Type: AgentEventThinking, Content: cleanResp.Content}
		}

		e.appendAgentMessage(cleanResp)

		// Separate delegation calls from regular tool calls
		var delegations, regular []ToolCall
		for _, tc := range cleanResp.ToolCalls {
			if tc.Name == "delegate_task" {
				delegations = append(delegations, tc)
			} else {
				regular = append(regular, tc)
			}
		}

		// Execute regular tool calls sequentially
		for _, tc := range regular {
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

		// Execute delegations in parallel
		if len(delegations) > 0 {
			type delegResult struct {
				tc     ToolCall
				result ToolResult
			}
			resultsCh := make(chan delegResult, len(delegations))

			for _, tc := range delegations {
				tc := tc // capture loop var
				e.agentChannel <- AgentEvent{Type: AgentEventToolCall, ToolCall: &tc}
				go func() {
					result := e.toolExecutor.Execute(tc)
					resultsCh <- delegResult{tc: tc, result: result}
				}()
			}

			// Collect all delegation results
			for range delegations {
				dr := <-resultsCh
				e.appendAgentMessage(Message{
					Role:       "tool",
					Content:    dr.result.Content,
					ToolCallID: dr.result.ToolCallID,
				})
				e.agentChannel <- AgentEvent{Type: AgentEventToolResult, ToolResult: &dr.result}
			}
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
		if e.remoteInfo.CurrentDir != "" {
			part += fmt.Sprintf("current directory is %s, ", e.remoteInfo.CurrentDir)
		}
		if e.remoteInfo.WorkspaceRoot != "" {
			part += fmt.Sprintf("workspace root is %s, ", e.remoteInfo.WorkspaceRoot)
		}
		part += "take this into account. "
		if e.config.GetUserConfig().GetPreferences() != "" {
			part += fmt.Sprintf("Also, %s.", e.config.GetUserConfig().GetPreferences())
		}
		return part
	}

	sys := e.config.GetSystemConfig()

	// Workspace is the primary working directory — emphasize it first.
	workDir := sys.GetWorkspaceRoot()
	if workDir == "" {
		workDir = sys.GetCurrentDirectory()
	}

	part := ""
	if workDir != "" {
		part += fmt.Sprintf("Primary working directory: %s — this is your workspace. Default all commands, searches, and file operations here unless told otherwise. ", workDir)
	}

	part += "System context: "
	if sys.GetOperatingSystem() != system.UnknownOperatingSystem {
		part += fmt.Sprintf("OS is %s, ", sys.GetOperatingSystem().String())
	}
	if sys.GetDistribution() != "" {
		part += fmt.Sprintf("distribution is %s, ", sys.GetDistribution())
	}
	if sys.GetHomeDirectory() != "" {
		part += fmt.Sprintf("home directory is %s, ", sys.GetHomeDirectory())
	}
	if sys.GetShell() != "" {
		part += fmt.Sprintf("shell is %s, ", sys.GetShell())
	}
	if sys.GetEditor() != "" {
		part += fmt.Sprintf("editor is %s, ", sys.GetEditor())
	}
	if sys.GetCurrentDirectory() != "" && sys.GetCurrentDirectory() != workDir {
		part += fmt.Sprintf("current directory is %s, ", sys.GetCurrentDirectory())
	}
	if sys.GetWorkspaceRoot() != "" && sys.GetWorkspaceRoot() != workDir {
		part += fmt.Sprintf("workspace root is %s, ", sys.GetWorkspaceRoot())
	}
	part += "take this into account. "

	if e.config.GetUserConfig().GetPreferences() != "" {
		part += fmt.Sprintf("Also, %s.", e.config.GetUserConfig().GetPreferences())
	}

	// Inject instruction files (HELM.md) discovered from the workspace.
	instructions := system.DiscoverInstructions(workDir)
	if instructions != "" {
		part += "\n\n# Project Instructions (from HELM.md)\n\n" + instructions
	}

	return part
}
