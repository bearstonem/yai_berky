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
	"sync"
	"os/exec"
	"runtime"
	"time"

	"context"
	"os"
	"path/filepath"

	"github.com/bearstonem/helm/command"
	"github.com/bearstonem/helm/cron"
	"github.com/bearstonem/helm/agent"
	"github.com/bearstonem/helm/backup"
	"github.com/bearstonem/helm/ai"
	"github.com/bearstonem/helm/config"
	"github.com/bearstonem/helm/goal"
	"github.com/bearstonem/helm/memory"
	"github.com/bearstonem/helm/session"
	"github.com/bearstonem/helm/skill"
	"github.com/spf13/viper"
)

type cronEvent struct {
	Type string // "thinking", "tool_call", "answer", "error", "done"
	Data string
}

//go:embed static/*
var staticFiles embed.FS

// Server is the Helm web GUI server.
type Server struct {
	homeDir      string
	sourceDir    string // app source directory (where go.mod lives)
	config       *config.Config
	engine       *ai.Engine
	addr         string
	activeEngine        *ai.Engine // currently running agent engine (for escalation responses)
	mu                  sync.Mutex
	selfImproveRunning  bool
	selfImproveCycle    int
	selfImproveCancel   context.CancelFunc
	selfImproveEngine   *ai.Engine
	selfImproveChan     chan ai.AgentEvent
	selfImproveInterval  time.Duration
	selfImproveDirective string // user's prime directive
	cronScheduler        *cron.Scheduler
	cronEventChans       map[string][]chan cronEvent // job ID → listeners
}

// NewServer creates a new web server.
func NewServer(cfg *config.Config, engine *ai.Engine, homeDir string, sourceDir string, port int) *Server {
	s := &Server{
		homeDir:        homeDir,
		sourceDir:      sourceDir,
		config:         cfg,
		engine:         engine,
		addr:           fmt.Sprintf("127.0.0.1:%d", port),
		cronEventChans: make(map[string][]chan cronEvent),
	}
	s.cronScheduler = cron.NewScheduler(homeDir, s.executeCronJob)
	return s
}

// Start launches the HTTP server and opens the browser.
func (s *Server) Start() error {
	// Sync tool API keys from config to environment on startup
	if braveKey := viper.GetString("BRAVE_API_KEY"); braveKey != "" && os.Getenv("BRAVE_API_KEY") == "" {
		os.Setenv("BRAVE_API_KEY", braveKey)
	}

	// Start cron scheduler
	s.cronScheduler.Start()

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
	mux.HandleFunc("/api/agent/respond", s.corsWrap(s.handleAgentRespond))
	mux.HandleFunc("/api/goals", s.corsWrap(s.handleGoals))
	mux.HandleFunc("/api/goals/", s.corsWrap(s.handleGoalByID))
	mux.HandleFunc("/api/self-improve/start", s.corsWrap(s.handleSelfImproveStart))
	mux.HandleFunc("/api/self-improve/stop", s.corsWrap(s.handleSelfImproveStop))
	mux.HandleFunc("/api/self-improve/status", s.corsWrap(s.handleSelfImproveStatus))
	mux.HandleFunc("/api/self-improve/stream", s.corsWrap(s.handleSelfImproveStream))
	mux.HandleFunc("/api/self-improve/reviews", s.corsWrap(s.handleEvolutionReviews))
	mux.HandleFunc("/api/command", s.corsWrap(s.handleCommand))
	mux.HandleFunc("/api/workspace", s.corsWrap(s.handleWorkspace))
	mux.HandleFunc("/api/workspace/browse", s.corsWrap(s.handleWorkspaceBrowse))
	mux.HandleFunc("/api/cron", s.corsWrap(s.handleCronJobs))
	mux.HandleFunc("/api/cron/", s.corsWrap(s.handleCronJobByID))
	mux.HandleFunc("/api/cron/scheduler", s.corsWrap(s.handleCronScheduler))
	mux.HandleFunc("/api/cron/stream/", s.corsWrap(s.handleCronStream))

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
	fmt.Printf("Helm GUI running at %s\n", url)

	// Write PID file for restart script
	pidFile := filepath.Join(backup.BackupsDir(s.homeDir), "helm.pid")
	os.MkdirAll(backup.BackupsDir(s.homeDir), 0755)
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	// Generate restart script
	port := listener.Addr().(*net.TCPAddr).Port
	binaryPath, _ := os.Executable()
	backup.GenerateRestartScript(s.homeDir, s.sourceDir, binaryPath, port)

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
		braveKey := os.Getenv("BRAVE_API_KEY")
		if braveKey == "" {
			braveKey = viper.GetString("BRAVE_API_KEY")
		}
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
			"brave_api_key":       maskKey(braveKey),
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
			"brave_api_key":       "BRAVE_API_KEY",
		}

		settings := make(map[string]interface{})
		for frontendKey, viperKey := range keyMap {
			if val, ok := req[frontendKey]; ok {
				// Don't overwrite keys with masked values
				if frontendKey == "api_key" || frontendKey == "brave_api_key" {
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

		// Sync tool API keys to environment so child processes can access them
		if braveVal, ok := settings["BRAVE_API_KEY"]; ok {
			if s, ok := braveVal.(string); ok && s != "" {
				os.Setenv("BRAVE_API_KEY", s)
			}
		}

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
		Message   string `json:"message"`
		SessionID string `json:"session_id,omitempty"`
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

	// Reuse the server's engine for session continuity
	engine := s.engine

	if req.SessionID != "" {
		if err := engine.LoadSession(s.homeDir, req.SessionID); err != nil {
			sseEvent(w, flusher, "error", "session not found: "+err.Error())
			return
		}
	}

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
			engine.SaveSession(s.homeDir)
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
		Message   string `json:"message"`
		AgentID   string `json:"agent_id,omitempty"`
		SessionID string `json:"session_id,omitempty"`
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

	// Reuse the server's engine for session continuity
	engine := s.engine

	// Apply agent profile if specified (or clear if switching back to primary)
	if req.AgentID != "" {
		profile, err := agent.Load(s.homeDir, req.AgentID)
		if err != nil {
			sseEvent(w, flusher, "error", "agent not found: "+err.Error())
			return
		}
		engine.SetAgentProfile(profile)
	} else {
		engine.SetAgentProfile(nil)
	}

	// Resume existing session or start new
	if req.SessionID != "" {
		if err := engine.LoadSession(s.homeDir, req.SessionID); err != nil {
			sseEvent(w, flusher, "error", "session not found: "+err.Error())
			return
		}
	} else {
		engine.StartNewSession()
	}

	// Track active engine for escalation responses
	s.mu.Lock()
	s.activeEngine = engine
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.activeEngine = nil
		s.mu.Unlock()
	}()

	// Respect auto-execute config — same behavior as terminal REPL
	autoExec := s.config.GetUserConfig().GetAgentAutoExecute()
	go func() {
		engine.AgentCompletion(req.Message, autoExec)
	}()

	ch := engine.GetAgentChannel()
	for event := range ch {
		switch event.Type {
		case ai.AgentEventThinking:
			payload := map[string]string{"content": event.Content}
			if event.AgentID != "" {
				payload["agent_id"] = event.AgentID
				payload["agent_name"] = event.AgentName
			}
			data, _ := json.Marshal(payload)
			sseEvent(w, flusher, "thinking", string(data))
		case ai.AgentEventToolCall:
			if event.ToolCall != nil {
				payload := map[string]string{
					"name":      event.ToolCall.Name,
					"arguments": event.ToolCall.Arguments,
				}
				if event.AgentID != "" {
					payload["agent_id"] = event.AgentID
					payload["agent_name"] = event.AgentName
				}
				data, _ := json.Marshal(payload)
				sseEvent(w, flusher, "tool_call", string(data))
			}
		case ai.AgentEventToolResult:
			if event.ToolResult != nil {
				content := event.ToolResult.Content
				if len(content) > 2000 {
					content = content[:2000] + "..."
				}
				payload := map[string]string{"content": content}
				if event.AgentID != "" {
					payload["agent_id"] = event.AgentID
					payload["agent_name"] = event.AgentName
				}
				data, _ := json.Marshal(payload)
				sseEvent(w, flusher, "tool_result", string(data))
			}
		case ai.AgentEventAnswer:
			payload := map[string]string{"content": event.Content}
			if event.AgentID != "" {
				payload["agent_id"] = event.AgentID
				payload["agent_name"] = event.AgentName
			}
			data, _ := json.Marshal(payload)
			sseEvent(w, flusher, "answer", string(data))
		case ai.AgentEventError:
			if event.Error != nil {
				sseEvent(w, flusher, "error", event.Error.Error())
			}
		case ai.AgentEventSubAgentStart:
			data, _ := json.Marshal(map[string]string{
				"agent_id":   event.AgentID,
				"agent_name": event.AgentName,
				"task":       event.Content,
			})
			sseEvent(w, flusher, "sub_agent_start", string(data))
		case ai.AgentEventSubAgentDone:
			data, _ := json.Marshal(map[string]string{
				"agent_id":   event.AgentID,
				"agent_name": event.AgentName,
				"status":     event.Content,
			})
			sseEvent(w, flusher, "sub_agent_done", string(data))
		case ai.AgentEventEscalation:
			data, _ := json.Marshal(map[string]string{
				"agent_id":   event.AgentID,
				"agent_name": event.AgentName,
				"question":   event.Content,
			})
			sseEvent(w, flusher, "escalation", string(data))
		case ai.AgentEventDone:
			// Save session to disk
			engine.SaveSession(s.homeDir)
			sseEvent(w, flusher, "done", "")
			return
		}
	}
}

// --- Builders (AI-assisted creation) ---

var skillBuilderPrompt = "You are a skill-builder assistant for Helm, an AI terminal tool. Your job is to help the user create a new \"skill\" — a reusable tool that the Helm agent can invoke.\n\n" +
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

var agentBuilderPrompt = "You are an agent-builder assistant for Helm, an AI terminal tool. Your job is to help the user create a custom \"agent profile\" — a preconfigured AI persona with a specific system prompt and tool set.\n\n" +
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

	// Build messages with system prompt
	messages := []ai.Message{
		{Role: "system", Content: systemPrompt},
	}
	for _, m := range req.Messages {
		messages = append(messages, ai.Message{Role: m.Role, Content: m.Content})
	}

	// Non-streaming completion — avoids think tag splitting issues
	engine, err := ai.NewEngine(ai.ChatEngineMode, s.config)
	if err != nil {
		s.jsonError(w, err.Error(), 500)
		return
	}

	content, err := engine.GetProvider().Complete(r.Context(), ai.CompletionRequest{
		Model:       engine.GetModel(),
		MaxTokens:   s.config.GetAiConfig().GetMaxTokens(),
		Temperature: s.config.GetAiConfig().GetTemperature(),
		Messages:    messages,
	})
	if err != nil {
		s.jsonError(w, err.Error(), 500)
		return
	}

	s.jsonResponse(w, map[string]string{"content": content})
}

// --- Goals ---

func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		goals, err := goal.LoadAll(s.homeDir)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		if goals == nil {
			goals = []goal.Goal{}
		}
		s.jsonResponse(w, goals)

	case "POST":
		var g goal.Goal
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if err := goal.Save(s.homeDir, &g); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		w.WriteHeader(201)
		s.jsonResponse(w, g)

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleGoalByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/goals/"):]
	if id == "" {
		s.jsonError(w, "missing goal ID", 400)
		return
	}

	switch r.Method {
	case "GET":
		g, err := goal.Load(s.homeDir, id)
		if err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, g)

	case "PUT":
		var g goal.Goal
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		g.ID = id
		if err := goal.Save(s.homeDir, &g); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		s.jsonResponse(w, g)

	case "DELETE":
		if err := goal.Delete(s.homeDir, id); err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

// --- Slash Commands ---

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}

	name, args := command.Parse(req.Input)
	if name == "" {
		s.jsonError(w, "not a slash command", 400)
		return
	}

	reg := command.NewRegistry()
	command.RegisterBuiltins(reg)

	cmd := reg.Get(name)
	if cmd == nil {
		s.jsonError(w, fmt.Sprintf("unknown command: /%s", name), 400)
		return
	}

	ctx := &command.Context{
		Config:  s.config,
		HomeDir: s.homeDir,
		WorkDir: s.engine.GetToolExecutor().GetWorkDir(),
		Mode:    "chat",
		ResetFn: func() {
			s.engine.StartNewSession()
		},
		SessionList: func() []session.SessionInfo {
			list, _ := session.List(s.homeDir)
			return list
		},
		GetModelFn: func() string {
			return s.config.GetAiConfig().GetModel()
		},
		MemoryStore: s.engine.GetMemoryStore(),
		ListAgents: func() []agent.Profile {
			agents, _ := agent.LoadAll(s.homeDir)
			return agents
		},
		ListGoals: func() []goal.Goal {
			goals, _ := goal.LoadAll(s.homeDir)
			return goals
		},
	}

	result := cmd.Handler(args, ctx)

	s.jsonResponse(w, map[string]interface{}{
		"output":   result.Output,
		"is_error": result.IsError,
		"clear":    result.Clear,
		"reset":    result.Reset,
	})
}

// --- Workspace ---

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		workDir := s.engine.GetToolExecutor().GetWorkDir()
		s.jsonResponse(w, map[string]string{
			"path": workDir,
			"name": filepath.Base(workDir),
		})

	case "PUT":
		var req struct {
			Path   string `json:"path"`
			Create bool   `json:"create"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.Path == "" {
			s.jsonError(w, "path is required", 400)
			return
		}

		// Create directory if requested
		if req.Create {
			if err := os.MkdirAll(req.Path, 0755); err != nil {
				s.jsonError(w, "failed to create directory: "+err.Error(), 400)
				return
			}
		}

		// Validate path exists and is a directory
		info, err := os.Stat(req.Path)
		if err != nil {
			s.jsonError(w, "path does not exist: "+err.Error(), 400)
			return
		}
		if !info.IsDir() {
			s.jsonError(w, "path is not a directory", 400)
			return
		}

		// Update workspace
		s.engine.GetToolExecutor().SetWorkDir(req.Path)

		// Save to recent workspaces
		s.saveRecentWorkspace(req.Path)

		s.jsonResponse(w, map[string]string{
			"path": req.Path,
			"name": filepath.Base(req.Path),
		})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleWorkspaceBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = s.homeDir
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.jsonError(w, err.Error(), 400)
		return
	}

	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		IsGit bool   `json:"is_git"`
	}

	var dirs []entry
	// Add parent directory
	parent := filepath.Dir(dir)
	if parent != dir {
		dirs = append(dirs, entry{Name: "..", Path: parent, IsDir: true})
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden dirs except .git
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		isGit := false
		if _, err := os.Stat(filepath.Join(fullPath, ".git")); err == nil {
			isGit = true
		}
		dirs = append(dirs, entry{Name: name, Path: fullPath, IsDir: true, IsGit: isGit})
	}

	// Load recent workspaces
	recents := s.loadRecentWorkspaces()

	s.jsonResponse(w, map[string]interface{}{
		"current": dir,
		"entries": dirs,
		"recents": recents,
	})
}

func (s *Server) recentWorkspacesPath() string {
	return filepath.Join(s.homeDir, ".config", "helm", "recent_workspaces.json")
}

func (s *Server) loadRecentWorkspaces() []string {
	data, err := os.ReadFile(s.recentWorkspacesPath())
	if err != nil {
		return nil
	}
	var recents []string
	json.Unmarshal(data, &recents)
	return recents
}

func (s *Server) saveRecentWorkspace(path string) {
	recents := s.loadRecentWorkspaces()

	// Remove if already present
	filtered := make([]string, 0, len(recents)+1)
	for _, r := range recents {
		if r != path {
			filtered = append(filtered, r)
		}
	}
	// Prepend
	filtered = append([]string{path}, filtered...)
	// Keep max 10
	if len(filtered) > 10 {
		filtered = filtered[:10]
	}

	data, _ := json.Marshal(filtered)
	os.MkdirAll(filepath.Dir(s.recentWorkspacesPath()), 0755)
	os.WriteFile(s.recentWorkspacesPath(), data, 0644)
}

// --- Cron Jobs ---

func (s *Server) handleCronJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		jobs, err := cron.LoadAll(s.homeDir)
		if err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		if jobs == nil {
			jobs = []cron.Job{}
		}
		s.jsonResponse(w, jobs)

	case "POST":
		var req struct {
			Name        string `json:"name"`
			Schedule    string `json:"schedule"`
			Instruction string `json:"instruction"`
			AgentID     string `json:"agent_id"`
			Enabled     bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.Name == "" || req.Schedule == "" || req.Instruction == "" {
			s.jsonError(w, "name, schedule, and instruction are required", 400)
			return
		}
		// Validate schedule
		if _, err := cron.NextRun(req.Schedule, time.Now()); err != nil {
			s.jsonError(w, "invalid schedule: "+err.Error(), 400)
			return
		}
		j := &cron.Job{
			Name:        req.Name,
			Schedule:    req.Schedule,
			Instruction: req.Instruction,
			AgentID:     req.AgentID,
			Output:      cron.ChannelChat,
			Enabled:     req.Enabled,
		}
		if err := cron.Save(s.homeDir, j); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		w.WriteHeader(201)
		s.jsonResponse(w, j)

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleCronJobByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/cron/"):]
	if id == "" || strings.Contains(id, "/") {
		s.jsonError(w, "missing job ID", 400)
		return
	}

	switch r.Method {
	case "GET":
		j, err := cron.Load(s.homeDir, id)
		if err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, j)

	case "PUT":
		var req struct {
			Name        string `json:"name"`
			Schedule    string `json:"schedule"`
			Instruction string `json:"instruction"`
			AgentID     string `json:"agent_id"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		existing, err := cron.Load(s.homeDir, id)
		if err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Schedule != "" {
			if _, err := cron.NextRun(req.Schedule, time.Now()); err != nil {
				s.jsonError(w, "invalid schedule: "+err.Error(), 400)
				return
			}
			existing.Schedule = req.Schedule
		}
		if req.Instruction != "" {
			existing.Instruction = req.Instruction
		}
		if req.AgentID != "" {
			existing.AgentID = req.AgentID
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if err := cron.Save(s.homeDir, existing); err != nil {
			s.jsonError(w, err.Error(), 500)
			return
		}
		s.jsonResponse(w, existing)

	case "DELETE":
		if err := cron.Delete(s.homeDir, id); err != nil {
			s.jsonError(w, err.Error(), 404)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

func (s *Server) handleCronScheduler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.jsonResponse(w, map[string]bool{"running": s.cronScheduler.IsRunning()})
	case "POST":
		var req struct {
			Action string `json:"action"` // "start" or "stop"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		switch req.Action {
		case "start":
			s.cronScheduler.Start()
		case "stop":
			s.cronScheduler.Stop()
		default:
			s.jsonError(w, "action must be 'start' or 'stop'", 400)
			return
		}
		s.jsonResponse(w, map[string]string{"status": "ok"})
	default:
		s.jsonError(w, "method not allowed", 405)
	}
}

// handleCronStream provides SSE stream for a running cron job.
func (s *Server) handleCronStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/cron/stream/"):]
	if id == "" {
		s.jsonError(w, "missing job ID", 400)
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

	ch := make(chan cronEvent, 64)
	s.mu.Lock()
	s.cronEventChans[id] = append(s.cronEventChans[id], ch)
	s.mu.Unlock()

	ctx := r.Context()
	defer func() {
		s.mu.Lock()
		chans := s.cronEventChans[id]
		for i, c := range chans {
			if c == ch {
				s.cronEventChans[id] = append(chans[:i], chans[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				sseEvent(w, flusher, "done", "")
				return
			}
			sseEvent(w, flusher, evt.Type, evt.Data)
		}
	}
}

func (s *Server) broadcastCronEvent(jobID string, evt cronEvent) {
	s.mu.Lock()
	chans := s.cronEventChans[jobID]
	s.mu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- evt:
		default:
		}
	}
}

// executeCronJob is called by the scheduler when a job fires.
func (s *Server) executeCronJob(job cron.Job) {
	cron.UpdateStatus(s.homeDir, job.ID, "running")
	s.broadcastCronEvent(job.ID, cronEvent{Type: "thinking", Data: fmt.Sprintf("Cron job %q started", job.Name)})

	engine, err := ai.NewEngine(ai.AgentEngineMode, s.config)
	if err != nil {
		cron.UpdateStatus(s.homeDir, job.ID, "error")
		s.broadcastCronEvent(job.ID, cronEvent{Type: "error", Data: err.Error()})
		return
	}
	engine.StartNewSession()

	done := make(chan struct{})
	go func() {
		engine.AgentCompletion(job.Instruction, true)
		close(engine.GetAgentChannel())
		close(done)
	}()

	agentCh := engine.GetAgentChannel()
	for event := range agentCh {
		evtType := "thinking"
		switch event.Type {
		case ai.AgentEventThinking:
			evtType = "thinking"
		case ai.AgentEventToolCall:
			evtType = "tool_call"
		case ai.AgentEventToolResult:
			evtType = "tool_result"
		case ai.AgentEventAnswer:
			evtType = "answer"
		case ai.AgentEventError:
			evtType = "error"
		}
		data := event.Content
		if event.Error != nil {
			data = event.Error.Error()
		}
		s.broadcastCronEvent(job.ID, cronEvent{Type: evtType, Data: data})
	}

	<-done
	engine.SaveSession(s.homeDir)
	cron.UpdateStatus(s.homeDir, job.ID, "success")
	s.broadcastCronEvent(job.ID, cronEvent{Type: "done", Data: ""})
}

// --- Self-Improvement Loop ---

var selfImprovePrompt = "You are Helm's self-improvement agent. You run periodically to make the platform better.\n\n" +
	"## Your Mission This Cycle\n\n" +
	"1. **REVIEW GOALS**: Use `list_goals` to check current goals. If none exist, create 2-3 foundational goals (e.g. expand skill set, improve agent coverage, ensure skill reliability).\n\n" +
	"2. **REVIEW CAPABILITIES**: Use `list_skills` to see current skills. Use `list_directory` to review agent profiles in ~/.config/helm/agents/.\n\n" +
	"3. **IDENTIFY GAPS**: Think about what skills and agents would make the platform more useful. Consider: API integrations, data processing, DevOps automation, code quality tools, monitoring, etc.\n\n" +
	"4. **IMPROVE OR CREATE**: Pick ONE actionable item and execute it well:\n" +
	"   - Create a new skill using `create_skill` (write the full script, test it with `run_command`)\n" +
	"   - Create a new agent using `create_agent` for an uncovered domain\n" +
	"   - Improve an existing skill by reading its script with `read_file` and rewriting it\n\n" +
	"5. **TEST**: After creating or modifying anything, test it to verify it works.\n\n" +
	"6. **UPDATE GOALS**: Use `update_goal` to log what you accomplished. Create new goals for future cycles.\n\n" +
	"7. **REPORT**: Summarize what you accomplished this cycle.\n\n" +
	"Be methodical. Do ONE thing well rather than many things poorly. Each cycle should make measurable progress.\n" +
	"NEVER start long-running processes (dev servers, watchers). Only use short-lived commands.\n\n" +
	"## PRESERVE EXISTING WORK\n" +
	"Agents and skills created in previous cycles are YOUR prior work. NEVER delete, overwrite, or recreate them.\n" +
	"- Before creating an agent, check if one already exists for that domain — if so, skip it or improve it in place.\n" +
	"- NEVER use `run_command` to delete files in ~/.config/helm/agents/ or ~/.config/helm/skills/.\n" +
	"- NEVER use `write_file` to overwrite agent JSON files — use `create_agent` which handles updates safely.\n" +
	"- If you want to improve an existing agent, read its file first, then create it again with the same name (it will update in place).\n" +
	"- Each cycle should ADD to the platform's capabilities, not rebuild from scratch.\n\n" +
	"## AUTONOMY — NEVER BLOCK ON USER INPUT\n" +
	"You run autonomously. NEVER use `escalate_to_user` to ask questions and wait for answers.\n" +
	"If you need information (API keys, preferences, credentials), save your question to a file\n" +
	"using `write_file` in ~/.config/helm/evolution-reviews/ and move on to something else.\n" +
	"Your cycle must NEVER stop or block — find an alternative approach or defer the task to a future cycle.\n\n" +
	"## CODE CHANGES & RESTART\n" +
	"Your application source is automatically backed up before each cycle.\n" +
	"If you modify Go source code (.go files), you MUST call `restart_helm` afterward to rebuild and relaunch.\n" +
	"The restart script will: kill current process → rebuild → relaunch.\n" +
	"If the build fails, the latest backup is automatically restored and relaunched.\n" +
	"This is safe — you can experiment with code changes knowing the backup protects against broken builds.\n"

func (s *Server) handleSelfImproveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	s.mu.Lock()
	if s.selfImproveRunning {
		s.mu.Unlock()
		s.jsonError(w, "self-improvement loop already running", 409)
		return
	}

	var req struct {
		IntervalMinutes int    `json:"interval_minutes"`
		PrimeDirective  string `json:"prime_directive"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.IntervalMinutes <= 0 {
		req.IntervalMinutes = 5
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.selfImproveRunning = true
	s.selfImproveCancel = cancel
	s.selfImproveCycle = 0
	s.selfImproveInterval = time.Duration(req.IntervalMinutes) * time.Minute
	s.selfImproveDirective = req.PrimeDirective
	s.selfImproveChan = make(chan ai.AgentEvent, 100)
	s.mu.Unlock()

	go s.selfImproveLoop(ctx)

	s.jsonResponse(w, map[string]interface{}{
		"status":           "started",
		"interval_minutes": req.IntervalMinutes,
	})
}

func (s *Server) handleSelfImproveStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	s.mu.Lock()
	if !s.selfImproveRunning {
		s.mu.Unlock()
		s.jsonError(w, "not running", 404)
		return
	}
	s.selfImproveCancel()
	s.selfImproveRunning = false
	s.mu.Unlock()

	s.jsonResponse(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleSelfImproveStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	s.mu.Lock()
	status := map[string]interface{}{
		"running":          s.selfImproveRunning,
		"cycle":            s.selfImproveCycle,
		"interval_minutes": int(s.selfImproveInterval.Minutes()),
		"prime_directive":  s.selfImproveDirective,
	}
	s.mu.Unlock()

	s.jsonResponse(w, status)
}

func (s *Server) handleSelfImproveStream(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ch := s.selfImproveChan
	s.mu.Unlock()

	if ch == nil {
		s.jsonError(w, "not running", 404)
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

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				sseEvent(w, flusher, "done", "loop stopped")
				return
			}
			switch event.Type {
			case ai.AgentEventThinking:
				payload := map[string]string{"content": event.Content}
				if event.AgentID != "" {
					payload["agent_id"] = event.AgentID
					payload["agent_name"] = event.AgentName
				}
				data, _ := json.Marshal(payload)
				sseEvent(w, flusher, "thinking", string(data))
			case ai.AgentEventToolCall:
				if event.ToolCall != nil {
					payload := map[string]string{"name": event.ToolCall.Name, "arguments": event.ToolCall.Arguments}
					if event.AgentID != "" {
						payload["agent_id"] = event.AgentID
						payload["agent_name"] = event.AgentName
					}
					data, _ := json.Marshal(payload)
					sseEvent(w, flusher, "tool_call", string(data))
				}
			case ai.AgentEventToolResult:
				if event.ToolResult != nil {
					content := event.ToolResult.Content
					if len(content) > 2000 {
						content = content[:2000] + "..."
					}
					payload := map[string]string{"content": content}
					data, _ := json.Marshal(payload)
					sseEvent(w, flusher, "tool_result", string(data))
				}
			case ai.AgentEventAnswer:
				payload := map[string]string{"content": event.Content}
				data, _ := json.Marshal(payload)
				sseEvent(w, flusher, "answer", string(data))
			case ai.AgentEventSubAgentStart:
				data, _ := json.Marshal(map[string]string{"agent_id": event.AgentID, "agent_name": event.AgentName, "task": event.Content})
				sseEvent(w, flusher, "sub_agent_start", string(data))
			case ai.AgentEventSubAgentDone:
				data, _ := json.Marshal(map[string]string{"agent_id": event.AgentID, "agent_name": event.AgentName, "status": event.Content})
				sseEvent(w, flusher, "sub_agent_done", string(data))
			case ai.AgentEventError:
				if event.Error != nil {
					sseEvent(w, flusher, "error", event.Error.Error())
				}
			case ai.AgentEventDone:
				sseEvent(w, flusher, "cycle_end", "")
			}
		}
	}
}

func (s *Server) selfImproveLoop(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.selfImproveRunning = false
		if s.selfImproveChan != nil {
			close(s.selfImproveChan)
			s.selfImproveChan = nil
		}
		s.mu.Unlock()
	}()

	// Run first cycle immediately
	s.runSelfImproveCycle(ctx)

	ticker := time.NewTicker(s.selfImproveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSelfImproveCycle(ctx)
		}
	}
}

// safeSend attempts to send on ch without panicking if it's closed.
func safeSend(ch chan ai.AgentEvent, event ai.AgentEvent) {
	defer func() { recover() }()
	select {
	case ch <- event:
	default:
	}
}

func (s *Server) runSelfImproveCycle(ctx context.Context) {
	s.mu.Lock()
	s.selfImproveCycle++
	cycle := s.selfImproveCycle
	ch := s.selfImproveChan
	s.mu.Unlock()

	if ch == nil {
		return
	}

	// Back up the application before making changes
	reason := fmt.Sprintf("self-improvement cycle %d", cycle)
	entry, backupErr := backup.Create(s.homeDir, s.sourceDir, reason)
	if backupErr != nil {
		safeSend(ch, ai.AgentEvent{Type: ai.AgentEventThinking, Content: fmt.Sprintf("Warning: backup failed: %s", backupErr)})
	} else {
		safeSend(ch, ai.AgentEvent{Type: ai.AgentEventThinking, Content: fmt.Sprintf("=== Self-Improvement Cycle %d (backed up: %s) ===", cycle, entry.ID)})
	}

	engine, err := ai.NewEngine(ai.AgentEngineMode, s.config)
	if err != nil {
		safeSend(ch, ai.AgentEvent{Type: ai.AgentEventError, Error: err})
		return
	}
	engine.StartNewSession()

	// Override escalation: write questions to a review file instead of blocking
	engine.GetToolExecutor().SetOnEscalateToUser(func(question, context string) (string, error) {
		reviewDir := filepath.Join(s.homeDir, ".config", "helm", "evolution-reviews")
		os.MkdirAll(reviewDir, 0755)
		ts := time.Now().Format("20060102-150405")
		reviewFile := filepath.Join(reviewDir, fmt.Sprintf("cycle%d-%s.md", cycle, ts))
		content := fmt.Sprintf("# Evolution Review — Cycle %d\n\n**Date:** %s\n\n## Question\n\n%s\n",
			cycle, time.Now().Format("2006-01-02 15:04:05"), question)
		if context != "" {
			content += fmt.Sprintf("\n## Context\n\n%s\n", context)
		}
		content += "\n## Status\n\nPending user review.\n"
		os.WriteFile(reviewFile, []byte(content), 0644)

		safeSend(ch, ai.AgentEvent{Type: ai.AgentEventThinking, Content: fmt.Sprintf("Saved question for user review: %s", reviewFile)})

		return "This question has been saved for user review at " + reviewFile + ". Continue your cycle without waiting — skip this task or find an alternative approach that doesn't require user input.", nil
	})

	s.mu.Lock()
	s.selfImproveEngine = engine
	s.mu.Unlock()

	// Build the prompt with prime directive
	prompt := selfImprovePrompt
	s.mu.Lock()
	directive := s.selfImproveDirective
	s.mu.Unlock()
	if directive != "" {
		prompt = "# PRIME DIRECTIVE — OVERRIDES ALL GOALS\n" +
			"Your overriding mission is: " + directive + "\n\n" +
			"This directive takes ABSOLUTE PRIORITY over any existing goals.\n" +
			"- Review your existing goals with `list_goals`. If any conflict with this directive, update them to align.\n" +
			"- Create new goals that serve this directive if none exist.\n" +
			"- Every skill you build, agent you create, and action you take MUST advance this mission.\n" +
			"- If existing goals are irrelevant to this directive, mark them as `paused` and create aligned ones.\n\n" +
			prompt
	} else {
		prompt = "# AUTONOMOUS MODE\n" +
			"No prime directive has been set. You are fully self-guiding.\n" +
			"Research what would be most useful, set your own direction, and pursue it.\n" +
			"If this is your first cycle, start by researching what capabilities would be most valuable, " +
			"then create foundational goals based on your findings.\n\n" +
			prompt
	}

	done := make(chan struct{})
	go func() {
		engine.AgentCompletion(prompt, true)
		close(engine.GetAgentChannel())
		close(done)
	}()

	// Forward events to broadcast channel
	agentCh := engine.GetAgentChannel()
	for event := range agentCh {
		safeSend(ch, event)
	}

	<-done
	engine.SaveSession(s.homeDir)

	s.mu.Lock()
	s.selfImproveEngine = nil
	s.mu.Unlock()

	// Emit cycle end
	safeSend(ch, ai.AgentEvent{Type: ai.AgentEventDone})
}

// Update escalation to also check self-improve engine
func (s *Server) handleAgentRespond(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "method not allowed", 405)
		return
	}

	var req struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", 400)
		return
	}

	s.mu.Lock()
	engine := s.activeEngine
	if engine == nil {
		engine = s.selfImproveEngine
	}
	s.mu.Unlock()

	if engine == nil {
		s.jsonError(w, "no active agent to respond to", 404)
		return
	}

	engine.RespondToEscalation(req.Response)
	s.jsonResponse(w, map[string]string{"status": "sent"})
}

func (s *Server) handleEvolutionReviews(w http.ResponseWriter, r *http.Request) {
	reviewDir := filepath.Join(s.homeDir, ".config", "helm", "evolution-reviews")

	if r.Method == "DELETE" {
		// Delete a specific review file
		filename := r.URL.Query().Get("file")
		if filename == "" {
			s.jsonError(w, "file parameter required", 400)
			return
		}
		// Prevent path traversal
		if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
			s.jsonError(w, "invalid filename", 400)
			return
		}
		os.Remove(filepath.Join(reviewDir, filename))
		s.jsonResponse(w, map[string]string{"status": "deleted"})
		return
	}

	entries, err := os.ReadDir(reviewDir)
	if err != nil {
		s.jsonResponse(w, []interface{}{})
		return
	}
	type review struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
		ModTime  string `json:"mod_time"`
	}
	var reviews []review
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reviewDir, e.Name()))
		if err != nil {
			continue
		}
		info, _ := e.Info()
		modTime := ""
		if info != nil {
			modTime = info.ModTime().Format(time.RFC3339)
		}
		reviews = append(reviews, review{
			Filename: e.Name(),
			Content:  string(data),
			ModTime:  modTime,
		})
	}
	s.jsonResponse(w, reviews)
}

func sseEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\n", event)
	// SSE requires multi-line data to have each line prefixed with "data: "
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprintf(w, "\n")
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
