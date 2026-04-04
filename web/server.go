package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"os/exec"
	"runtime"
	"time"

	"github.com/ekkinox/yai/agent"
	"github.com/ekkinox/yai/ai"
	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/memory"
	"github.com/ekkinox/yai/session"
	"github.com/ekkinox/yai/skill"
)

//go:embed static/*
var staticFiles embed.FS

// Server is the yai web GUI server.
type Server struct {
	homeDir string
	config  *config.Config
	engine  *ai.Engine
	addr    string
}

// NewServer creates a new web server.
func NewServer(cfg *config.Config, engine *ai.Engine, homeDir string, port int) *Server {
	return &Server{
		homeDir: homeDir,
		config:  cfg,
		engine:  engine,
		addr:    fmt.Sprintf("127.0.0.1:%d", port),
	}
}

// Start launches the HTTP server and opens the browser.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/skills", s.corsWrap(s.handleSkills))
	mux.HandleFunc("/api/skills/", s.corsWrap(s.handleSkillByName))
	mux.HandleFunc("/api/sessions", s.corsWrap(s.handleSessions))
	mux.HandleFunc("/api/sessions/", s.corsWrap(s.handleSessionByID))
	mux.HandleFunc("/api/config", s.corsWrap(s.handleConfig))
	mux.HandleFunc("/api/providers", s.corsWrap(s.handleProviders))
	mux.HandleFunc("/api/memory/stats", s.corsWrap(s.handleMemoryStats))
	mux.HandleFunc("/api/agents", s.corsWrap(s.handleAgents))
	mux.HandleFunc("/api/agents/", s.corsWrap(s.handleAgentByID))
	mux.HandleFunc("/api/tools", s.corsWrap(s.handleTools))
	mux.HandleFunc("/api/build/skill", s.corsWrap(s.handleBuildSkill))
	mux.HandleFunc("/api/build/agent", s.corsWrap(s.handleBuildAgent))
	mux.HandleFunc("/api/chat", s.corsWrap(s.handleChat))
	mux.HandleFunc("/api/agent", s.corsWrap(s.handleAgent))

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	url := fmt.Sprintf("http://%s", listener.Addr().String())
	fmt.Printf("yai GUI running at %s\n", url)

	// Open browser
	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser(url)
	}()

	return http.Serve(listener, mux)
}

func (s *Server) corsWrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}
}

func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Skills ---

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		skills, err := skill.LoadAll(s.homeDir)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		if skills == nil {
			skills = []skill.Manifest{}
		}
		s.jsonResponse(w, skills)

	case "POST":
		var req struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Language    string          `json:"language"`
			Script      string          `json:"script"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.Parameters == nil {
			req.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		m, err := skill.Create(s.homeDir, req.Name, req.Description, req.Language, req.Script, req.Parameters)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		w.WriteHeader(201)
		s.jsonResponse(w, m)

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleSkillByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/skills/"):]
	if name == "" {
		s.jsonError(w, "missing skill name", 400)
		return
	}

	switch r.Method {
	case "GET":
		skills, err := skill.LoadAll(s.homeDir)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		for _, sk := range skills {
			if sk.Name == name {
				// Include script content in the response
				script, _ := skill.ReadScript(s.homeDir, sk)
				s.jsonResponse(w, map[string]interface{}{
					"name":        sk.Name,
					"description": sk.Description,
					"language":    sk.Language,
					"parameters":  sk.Parameters,
					"script_file": sk.ScriptFile,
					"script":      script,
					"created_at":  sk.CreatedAt,
				})
				return
			}
		}
		s.jsonError(w, "skill not found", 404)

	case "PUT":
		var req struct {
			Description string          `json:"description"`
			Language    string          `json:"language"`
			Script      string          `json:"script"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.Parameters == nil {
			req.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		m, err := skill.Update(s.homeDir, name, req.Description, req.Language, req.Script, req.Parameters)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		s.jsonResponse(w, m)

	case "DELETE":
		if err := skill.Remove(s.homeDir, name); err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

// --- Sessions ---

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}
	sessions, err := session.List(s.homeDir)
	if err != nil {
		s.jsonError(w, err.Error(), 500)
		return
	}
	if sessions == nil {
		sessions = []session.SessionInfo{}
	}
	s.jsonResponse(w, sessions)
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/sessions/"):]
	if id == "" {
		s.jsonError(w, "missing session ID", 400)
		return
	}

	switch r.Method {
	case "GET":
		sess, err := session.Load(s.homeDir, id)
		if err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, sess)

	case "DELETE":
		if err := session.Delete(s.homeDir, id); err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

// --- Config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		aiCfg := s.config.GetAiConfig()
		userCfg := s.config.GetUserConfig()
		s.jsonResponse(w, map[string]interface{}{
			"provider":            aiCfg.GetProvider(),
			"api_key":             maskKey(aiCfg.GetKey()),
			"model":               aiCfg.GetModel(),
			"base_url":            aiCfg.GetBaseURL(),
			"proxy":               aiCfg.GetProxy(),
			"temperature":         aiCfg.GetTemperature(),
			"max_tokens":          aiCfg.GetMaxTokens(),
			"default_prompt_mode": userCfg.GetDefaultPromptMode(),
			"preferences":         userCfg.GetPreferences(),
			"allow_sudo":          userCfg.GetAllowSudo(),
			"auto_execute":        userCfg.GetAgentAutoExecute(),
			"permission_mode":     userCfg.GetPermissionMode().String(),
		})

	case "PUT":
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}

		// Map frontend field names to viper config keys
		keyMap := map[string]string{
			"provider":            "AI_PROVIDER",
			"api_key":             "AI_API_KEY",
			"model":               "AI_MODEL",
			"base_url":            "AI_BASE_URL",
			"proxy":               "AI_PROXY",
			"temperature":         "AI_TEMPERATURE",
			"max_tokens":          "AI_MAX_TOKENS",
			"default_prompt_mode": "USER_DEFAULT_PROMPT_MODE",
			"preferences":         "USER_PREFERENCES",
			"allow_sudo":          "USER_ALLOW_SUDO",
			"auto_execute":        "USER_AGENT_AUTO_EXECUTE",
			"permission_mode":     "USER_PERMISSION_MODE",
		}

		settings := make(map[string]interface{})
		for frontendKey, viperKey := range keyMap {
			if val, ok := req[frontendKey]; ok {
				// Don't overwrite key with masked value
				if frontendKey == "api_key" {
					str, isStr := val.(string)
					if isStr && (str == "" || strings.Contains(str, "***")) {
						continue
					}
				}
				settings[viperKey] = val
			}
		}

		newCfg, err := config.SaveAllSettings(settings)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		s.config = newCfg

		s.jsonResponse(w, map[string]string{"status": "saved"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	type providerInfo struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		DefaultModel string `json:"default_model"`
		NeedsAPIKey  bool   `json:"needs_api_key"`
	}

	var providers []providerInfo
	for _, id := range config.ProviderList() {
		providers = append(providers, providerInfo{
			ID:           id,
			Name:         config.ProviderDisplayNames[id],
			DefaultModel: config.ProviderDefaultModels[id],
			NeedsAPIKey:  config.ProviderNeedsAPIKey(id),
		})
	}
	s.jsonResponse(w, providers)
}

// --- Memory ---

func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	store := s.engine.GetMemoryStore()
	if store == nil {
		s.jsonResponse(w, map[string]interface{}{
			"available": false,
			"messages":  0,
			"skills":    0,
			"sessions":  0,
		})
		return
	}

	msgs, skills, sessions := store.Stats()
	s.jsonResponse(w, map[string]interface{}{
		"available": true,
		"messages":  msgs,
		"skills":    skills,
		"sessions":  sessions,
	})
}

// --- Agents ---

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		agents, err := agent.LoadAll(s.homeDir)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		if agents == nil {
			agents = []agent.Profile{}
		}
		s.jsonResponse(w, agents)

	case "POST":
		var p agent.Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if p.Name == "" {
			s.jsonError(w, "name is required", 400)
			return
		}
		if err := agent.Save(s.homeDir, &p); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		w.WriteHeader(201)
		s.jsonResponse(w, p)

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/agents/"):]
	if id == "" {
		s.jsonError(w, "missing agent ID", 400)
		return
	}

	switch r.Method {
	case "GET":
		p, err := agent.Load(s.homeDir, id)
		if err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, p)

	case "PUT":
		var p agent.Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		p.ID = id
		if err := agent.Save(s.homeDir, &p); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		s.jsonResponse(w, p)

	case "DELETE":
		if err := agent.Delete(s.homeDir, id); err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

// --- Tools (list available tools for assignment) ---

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	// Create a temp engine to get the full tool list
	engine, err := ai.NewEngine(ai.AgentEngineMode, s.config)
	if err != nil {
		s.jsonError(w, err.Error(), 500)
		return
	}

	type toolInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	var tools []toolInfo
	for _, t := range engine.GetToolExecutor().AllTools() {
		tools = append(tools, toolInfo{Name: t.Name, Description: t.Description})
	}
	s.jsonResponse(w, tools)
}

// --- Chat (SSE streaming) ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", 400)
		return
	}
	if req.Message == "" {
		s.jsonError(w, "message required", 400)
		return
	}

	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, "streaming not supported", 500)
		return
	}

	// Create a fresh engine for this request in chat mode
	engine, err := ai.NewEngine(ai.ChatEngineMode, s.config)
	if err != nil {
		sseEvent(w, flusher, "error", err.Error())
		return
	}
	engine.StartNewSession()

	// Start streaming
	go func() {
		engine.ChatStreamCompletion(req.Message)
	}()

	ch := engine.GetChannel()
	for output := range ch {
		if output.GetContent() != "" {
			sseEvent(w, flusher, "content", output.GetContent())
		}
		if output.IsLast() {
			sseEvent(w, flusher, "done", "")
			return
		}
	}
}

// --- Agent (SSE streaming) ---

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	var req struct {
		Message string `json:"message"`
		AgentID string `json:"agent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", 400)
		return
	}
	if req.Message == "" {
		s.jsonError(w, "message required", 400)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, "streaming not supported", 500)
		return
	}

	engine, err := ai.NewEngine(ai.AgentEngineMode, s.config)
	if err != nil {
		sseEvent(w, flusher, "error", err.Error())
		return
	}

	// Apply agent profile if specified
	if req.AgentID != "" {
		profile, err := agent.Load(s.homeDir, req.AgentID)
		if err != nil {
			sseEvent(w, flusher, "error", "agent not found: "+err.Error())
			return
		}
		engine.SetAgentProfile(profile)
	}

	engine.StartNewSession()

	// Auto-execute all tools in GUI mode
	go func() {
		engine.AgentCompletion(req.Message, true)
	}()

	ch := engine.GetAgentChannel()
	for event := range ch {
		switch event.Type {
		case ai.AgentEventThinking:
			sseEvent(w, flusher, "thinking", event.Content)
		case ai.AgentEventToolCall:
			if event.ToolCall != nil {
				data, _ := json.Marshal(map[string]string{
					"name":      event.ToolCall.Name,
					"arguments": event.ToolCall.Arguments,
				})
				sseEvent(w, flusher, "tool_call", string(data))
			}
		case ai.AgentEventToolResult:
			if event.ToolResult != nil {
				content := event.ToolResult.Content
				if len(content) > 2000 {
					content = content[:2000] + "..."
				}
				sseEvent(w, flusher, "tool_result", content)
			}
		case ai.AgentEventAnswer:
			sseEvent(w, flusher, "answer", event.Content)
		case ai.AgentEventError:
			if event.Error != nil {
				sseEvent(w, flusher, "error", event.Error.Error())
			}
		case ai.AgentEventDone:
			sseEvent(w, flusher, "done", "")
			return
		}
	}
}

// --- Builders (AI-assisted creation) ---

var skillBuilderPrompt = "You are a skill-builder assistant for Yai, an AI terminal tool. Your job is to help the user create a new \"skill\" — a reusable tool that the Yai agent can invoke.\n\n" +
	"A skill consists of:\n" +
	"- **name**: short snake_case identifier (e.g. \"fetch_github_issues\")\n" +
	"- **description**: what the tool does (shown to the AI agent)\n" +
	"- **language**: one of \"bash\", \"python\", \"node\", or \"ruby\"\n" +
	"- **script**: the executable script content. It receives JSON arguments on stdin and should print its output to stdout.\n" +
	"- **parameters**: JSON Schema describing the tool's input parameters\n\n" +
	"Guide the user through defining their skill step by step. Ask clarifying questions about what the tool should do, what inputs it needs, what language makes sense, etc.\n\n" +
	"When you have enough information and the user is satisfied, output the final skill definition as a JSON block wrapped in triple backticks with the label \"skill_definition\", like this:\n\n" +
	"```skill_definition\n" +
	"{\n  \"name\": \"example_tool\",\n  \"description\": \"Does something useful\",\n  \"language\": \"bash\",\n  \"script\": \"#!/bin/bash\\nread input\\necho hello\",\n  \"parameters\": {\"type\": \"object\", \"properties\": {\"arg1\": {\"type\": \"string\"}}, \"required\": [\"arg1\"]}\n}\n" +
	"```\n\n" +
	"IMPORTANT rules:\n" +
	"- Only output the skill_definition block when you and the user are both ready. Don't output it prematurely.\n" +
	"- The script must be complete and functional. Include proper error handling.\n" +
	"- The script receives arguments as a single JSON string on stdin (read from stdin).\n" +
	"- Escape newlines and special characters properly in the JSON \"script\" field.\n" +
	"- Keep descriptions concise but informative — they're shown to the AI agent.\n" +
	"- Be conversational and helpful. If the user is vague, ask questions."

var agentBuilderPrompt = "You are an agent-builder assistant for Yai, an AI terminal tool. Your job is to help the user create a custom \"agent profile\" — a preconfigured AI persona with a specific system prompt and tool set.\n\n" +
	"An agent profile consists of:\n" +
	"- **name**: human-readable name (e.g. \"Code Reviewer\", \"DevOps Bot\")\n" +
	"- **description**: short description of what this agent specializes in\n" +
	"- **system_prompt**: the full system prompt that defines the agent's behavior, personality, and instructions\n" +
	"- **tools**: list of tool names this agent is allowed to use (empty = all tools). Available tools: run_command, read_file, write_file, edit_file, list_directory, search_files, find_files, create_skill, list_skills, remove_skill, plus any skill_* tools.\n" +
	"- **model**: (optional) model override, leave empty for default\n\n" +
	"Guide the user through defining their agent step by step. Ask about:\n" +
	"- What domain/task should the agent specialize in?\n" +
	"- What tone/personality should it have?\n" +
	"- Should it be restricted to certain tools? (e.g. a read-only reviewer shouldn't have write_file)\n" +
	"- Any specific instructions or constraints?\n\n" +
	"Write a thorough, well-crafted system prompt for them. When ready, output the final definition as a JSON block:\n\n" +
	"```agent_definition\n" +
	"{\n  \"name\": \"Example Agent\",\n  \"description\": \"Specializes in X\",\n  \"system_prompt\": \"You are an expert...\",\n  \"tools\": [\"read_file\", \"search_files\", \"find_files\"],\n  \"model\": \"\"\n}\n" +
	"```\n\n" +
	"IMPORTANT rules:\n" +
	"- Only output the agent_definition block when you and the user are both ready.\n" +
	"- The system prompt should be detailed and well-structured. Include role definition, approach guidelines, constraints, and output format preferences.\n" +
	"- If restricting tools, explain to the user what each tool does so they can make informed choices.\n" +
	"- Be conversational and helpful. Ask questions to understand their needs."

func (s *Server) handleBuildSkill(w http.ResponseWriter, r *http.Request) {
	s.handleBuilder(w, r, skillBuilderPrompt)
}

func (s *Server) handleBuildAgent(w http.ResponseWriter, r *http.Request) {
	s.handleBuilder(w, r, agentBuilderPrompt)
}

type builderMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type builderRequest struct {
	Messages []builderMessage `json:"messages"`
}

func (s *Server) handleBuilder(w http.ResponseWriter, r *http.Request, systemPrompt string) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	var req builderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", 400)
		return
	}
	if len(req.Messages) == 0 {
		s.jsonError(w, "messages required", 400)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, "streaming not supported", 500)
		return
	}

	// Build messages with system prompt
	messages := []ai.Message{
		{Role: "system", Content: systemPrompt},
	}
	for _, m := range req.Messages {
		messages = append(messages, ai.Message{Role: m.Role, Content: m.Content})
	}

	// Create a fresh engine for this request
	engine, err := ai.NewEngine(ai.ChatEngineMode, s.config)
	if err != nil {
		sseEvent(w, flusher, "error", err.Error())
		return
	}

	// Use the engine's provider directly for a streaming completion
	ch := make(chan ai.StreamChunk)
	go func() {
		engine.GetProvider().StreamComplete(r.Context(), ai.CompletionRequest{
			Model:       engine.GetModel(),
			MaxTokens:   s.config.GetAiConfig().GetMaxTokens(),
			Temperature: s.config.GetAiConfig().GetTemperature(),
			Messages:    messages,
		}, ch)
	}()

	for chunk := range ch {
		if chunk.Err != nil {
			sseEvent(w, flusher, "error", chunk.Err.Error())
			return
		}
		if chunk.Content != "" {
			sseEvent(w, flusher, "content", chunk.Content)
		}
		if chunk.Done {
			sseEvent(w, flusher, "done", "")
			return
		}
	}
}

func sseEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	flusher.Flush()
}

// GetMemoryStore returns the memory store from the engine (may be nil).
// This is a convenience to avoid importing memory in server consumers.
func (s *Server) GetMemoryStore() *memory.Store {
	return s.engine.GetMemoryStore()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser: %s (visit %s manually)\n", err, url)
	}
}
