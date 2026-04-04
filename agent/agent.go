package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Profile defines a custom agent configuration.
type Profile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`          // allowed tool names; empty = all tools
	Model        string   `json:"model,omitempty"` // model override; empty = use default
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AgentsDir returns the directory where agent profiles are stored.
func AgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "agents")
}

func agentPath(homeDir, id string) string {
	return filepath.Join(AgentsDir(homeDir), id+".json")
}

// LoadAll reads all agent profiles from disk.
func LoadAll(homeDir string) ([]Profile, error) {
	dir := AgentsDir(homeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var agents []Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var a Profile
		if err := json.Unmarshal(data, &a); err != nil {
			continue
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// Load reads a single agent profile by ID.
func Load(homeDir, id string) (*Profile, error) {
	data, err := os.ReadFile(agentPath(homeDir, id))
	if err != nil {
		return nil, fmt.Errorf("agent %q not found: %w", id, err)
	}
	var a Profile
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse agent %q: %w", id, err)
	}
	return &a, nil
}

// Save writes an agent profile to disk.
func Save(homeDir string, a *Profile) error {
	dir := AgentsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	if a.ID == "" {
		a.ID = sanitizeID(a.Name)
	}
	if a.ID == "" {
		return fmt.Errorf("agent must have a name")
	}

	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent: %w", err)
	}

	return os.WriteFile(agentPath(homeDir, a.ID), data, 0644)
}

// Delete removes an agent profile by ID.
func Delete(homeDir, id string) error {
	path := agentPath(homeDir, id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent %q not found", id)
	}
	return os.Remove(path)
}

func sanitizeID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
