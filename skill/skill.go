package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest describes a user-created skill that the agent can invoke.
type Manifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Language    string          `json:"language"` // "bash", "python", "node", etc.
	ScriptFile  string          `json:"script_file"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ToolName returns the prefixed tool name used in the agent's tool inventory.
func (m *Manifest) ToolName() string {
	return "skill_" + sanitizeName(m.Name)
}

// SkillsDir returns the default skills directory (~/.config/yai/skills/).
func SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "yai", "skills")
}

// LoadAll reads all skill manifests from the skills directory.
func LoadAll(homeDir string) ([]Manifest, error) {
	dir := SkillsDir(homeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // skip broken skills
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		skills = append(skills, m)
	}
	return skills, nil
}

// Create writes a new skill to disk (manifest + script).
func Create(homeDir string, name, description, language, scriptContent string, parameters json.RawMessage) (*Manifest, error) {
	safeName := sanitizeName(name)
	if safeName == "" {
		return nil, fmt.Errorf("invalid skill name: %q", name)
	}

	dir := filepath.Join(SkillsDir(homeDir), safeName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating skill directory: %w", err)
	}

	ext := extensionForLanguage(language)
	scriptFile := "script" + ext

	m := Manifest{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Language:    language,
		ScriptFile:  scriptFile,
		CreatedAt:   time.Now(),
	}

	// Write manifest
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	// Write script
	scriptPath := filepath.Join(dir, scriptFile)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return nil, fmt.Errorf("writing script: %w", err)
	}

	return &m, nil
}

// Remove deletes a skill by name.
func Remove(homeDir, name string) error {
	safeName := sanitizeName(name)
	dir := filepath.Join(SkillsDir(homeDir), safeName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", name)
	}
	return os.RemoveAll(dir)
}

// ScriptPath returns the absolute path to a skill's script file.
func ScriptPath(homeDir string, m Manifest) string {
	return filepath.Join(SkillsDir(homeDir), sanitizeName(m.Name), m.ScriptFile)
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	// Keep only alphanumeric and underscores
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func extensionForLanguage(lang string) string {
	switch strings.ToLower(lang) {
	case "python", "python3", "py":
		return ".py"
	case "node", "nodejs", "javascript", "js":
		return ".js"
	case "ruby", "rb":
		return ".rb"
	case "bash", "sh", "shell", "":
		return ".sh"
	default:
		return ".sh"
	}
}
