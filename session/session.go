package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Message mirrors ai.Message for serialization without circular imports.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Session struct {
	ID        string    `json:"id"`
	Mode      string    `json:"mode"` // "exec", "chat", "agent"
	Messages  []Message `json:"messages"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionInfo is a lightweight summary for listing sessions.
type SessionInfo struct {
	ID        string    `json:"id"`
	Mode      string    `json:"mode"`
	Summary   string    `json:"summary"`
	Messages  int       `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func sessionsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "sessions")
}

func sessionPath(homeDir, id string) string {
	return filepath.Join(sessionsDir(homeDir), id+".json")
}

// NewSession creates a new session with a unique ID.
func NewSession(mode string) *Session {
	now := time.Now()
	return &Session{
		ID:        shortID(),
		Mode:      mode,
		Messages:  []Message{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Save writes the session to disk.
func (s *Session) Save(homeDir string) error {
	dir := sessionsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	s.UpdatedAt = time.Now()

	// Auto-generate summary from first user message.
	if s.Summary == "" {
		for _, m := range s.Messages {
			if m.Role == "user" && m.Content != "" {
				s.Summary = truncate(m.Content, 80)
				break
			}
		}
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return os.WriteFile(sessionPath(homeDir, s.ID), data, 0644)
}

// Load reads a session from disk by ID.
func Load(homeDir, id string) (*Session, error) {
	data, err := os.ReadFile(sessionPath(homeDir, id))
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", id, err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session %s: %w", id, err)
	}

	return &s, nil
}

// List returns summaries of all saved sessions, newest first.
func List(homeDir string) ([]SessionInfo, error) {
	dir := sessionsDir(homeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var infos []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		infos = append(infos, SessionInfo{
			ID:        s.ID,
			Mode:      s.Mode,
			Summary:   s.Summary,
			Messages:  len(s.Messages),
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt.After(infos[j].UpdatedAt)
	})

	return infos, nil
}

// Delete removes a session file.
func Delete(homeDir, id string) error {
	return os.Remove(sessionPath(homeDir, id))
}

// Export returns the session as formatted JSON bytes.
func (s *Session) Export() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func shortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func truncate(s string, max int) string {
	// Take first line only.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
