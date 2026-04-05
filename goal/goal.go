package goal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Goal tracks a self-improvement objective.
type Goal struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`   // "active", "completed", "paused"
	Progress    string    `json:"progress"`  // free-form progress notes
	Priority    int       `json:"priority"`  // 1=high, 2=medium, 3=low
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GoalsDir returns the directory where goals are stored.
func GoalsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "goals")
}

func goalPath(homeDir, id string) string {
	return filepath.Join(GoalsDir(homeDir), id+".json")
}

// LoadAll reads all goals from disk.
func LoadAll(homeDir string) ([]Goal, error) {
	dir := GoalsDir(homeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var goals []Goal
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var g Goal
		if err := json.Unmarshal(data, &g); err != nil {
			continue
		}
		goals = append(goals, g)
	}
	return goals, nil
}

// Load reads a single goal by ID.
func Load(homeDir, id string) (*Goal, error) {
	data, err := os.ReadFile(goalPath(homeDir, id))
	if err != nil {
		return nil, fmt.Errorf("goal %q not found: %w", id, err)
	}
	var g Goal
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse goal %q: %w", id, err)
	}
	return &g, nil
}

// Save writes a goal to disk.
func Save(homeDir string, g *Goal) error {
	dir := GoalsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create goals dir: %w", err)
	}

	if g.ID == "" {
		g.ID = sanitizeID(g.Title)
	}
	if g.ID == "" {
		return fmt.Errorf("goal must have a title")
	}
	if g.Status == "" {
		g.Status = "active"
	}

	now := time.Now()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now

	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal goal: %w", err)
	}

	return os.WriteFile(goalPath(homeDir, g.ID), data, 0644)
}

// Delete removes a goal by ID.
func Delete(homeDir, id string) error {
	path := goalPath(homeDir, id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("goal %q not found", id)
	}
	return os.Remove(path)
}

func sanitizeID(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = strings.ReplaceAll(title, " ", "_")
	var b strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
