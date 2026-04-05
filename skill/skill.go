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

// SkillsDir returns the default skills directory (~/.config/helm/skills/).
func SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "skills")
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
		// Validate and fix parameters — skip skills with unfixable schemas
		if validated, err := ValidateParameters(m.Parameters); err == nil {
			m.Parameters = validated
		} else {
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

	// Validate and sanitize parameters to ensure they form a valid JSON Schema
	var err error
	parameters, err = ValidateParameters(parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters schema: %w", err)
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

// Update modifies an existing skill's manifest and script.
func Update(homeDir string, name, description, language, scriptContent string, parameters json.RawMessage) (*Manifest, error) {
	safeName := sanitizeName(name)
	if safeName == "" {
		return nil, fmt.Errorf("invalid skill name: %q", name)
	}

	dir := filepath.Join(SkillsDir(homeDir), safeName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	parameters, err := ValidateParameters(parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters schema: %w", err)
	}

	// Load existing manifest to preserve created_at
	var existing Manifest
	manifestPath := filepath.Join(dir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		json.Unmarshal(data, &existing)
	}

	ext := extensionForLanguage(language)
	scriptFile := "script" + ext

	m := Manifest{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Language:    language,
		ScriptFile:  scriptFile,
		CreatedAt:   existing.CreatedAt,
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	// Remove old script if language changed
	if existing.ScriptFile != "" && existing.ScriptFile != scriptFile {
		os.Remove(filepath.Join(dir, existing.ScriptFile))
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, scriptFile), []byte(scriptContent), 0755); err != nil {
		return nil, fmt.Errorf("writing script: %w", err)
	}

	return &m, nil
}

// ReadScript reads the script content for a skill.
func ReadScript(homeDir string, m Manifest) (string, error) {
	path := ScriptPath(homeDir, m)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

// fixParameters unwraps double-encoded JSON parameters.
// If parameters is a JSON string containing a JSON object, it unwraps it.
// e.g. `"{\"type\":\"object\"}"` → `{"type":"object"}`
func fixParameters(params json.RawMessage) json.RawMessage {
	if len(params) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	trimmed := strings.TrimSpace(string(params))

	// If it starts with a quote, it's a string — try to unwrap
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(params, &s); err == nil {
			// Check if the unwrapped string is valid JSON
			if json.Valid([]byte(s)) {
				return json.RawMessage(s)
			}
		}
	}

	// Already a valid JSON object/array — return as-is
	if json.Valid(params) {
		return params
	}

	return json.RawMessage(`{"type":"object","properties":{}}`)
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

// ValidateParameters checks that a parameters JSON blob is a valid JSON Schema
// object suitable for use as a tool input_schema. Returns a sanitized version
// or a safe default if the input is invalid.
func ValidateParameters(params json.RawMessage) (json.RawMessage, error) {
	params = fixParameters(params)

	if len(params) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`), nil
	}

	// Must be valid JSON
	if !json.Valid(params) {
		return nil, fmt.Errorf("parameters is not valid JSON")
	}

	// Must be a JSON object
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(params, &obj); err != nil {
		return nil, fmt.Errorf("parameters must be a JSON object: %w", err)
	}

	// Must have "type" field set to "object"
	typeVal, hasType := obj["type"]
	if !hasType {
		// Add "type": "object" if missing
		obj["type"] = json.RawMessage(`"object"`)
	} else {
		var t string
		if err := json.Unmarshal(typeVal, &t); err != nil || t != "object" {
			obj["type"] = json.RawMessage(`"object"`)
		}
	}

	// Ensure "properties" exists
	if _, hasProp := obj["properties"]; !hasProp {
		obj["properties"] = json.RawMessage(`{}`)
	} else {
		// Validate that properties is an object
		var props map[string]json.RawMessage
		if err := json.Unmarshal(obj["properties"], &props); err != nil {
			obj["properties"] = json.RawMessage(`{}`)
		}
	}

	// Validate "required" if present — must be an array of strings
	if req, hasReq := obj["required"]; hasReq {
		var arr []string
		if err := json.Unmarshal(req, &arr); err != nil {
			// Remove invalid required field
			delete(obj, "required")
		}
	}

	result, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`), nil
	}
	return json.RawMessage(result), nil
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
