package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".config", "helm", "agents"), 0755)
	return dir
}

func TestSanitizeID(t *testing.T) {
	assert.Equal(t, "hello_world", SanitizeID("Hello World"))
	assert.Equal(t, "test-agent", SanitizeID("Test-Agent"))
	assert.Equal(t, "agent_123", SanitizeID("agent 123"))
	assert.Equal(t, "", SanitizeID("   "))
	assert.Equal(t, "abc", SanitizeID("a!@#b$%^c"))
}

func TestSaveAndLoad(t *testing.T) {
	dir := setupTestDir(t)

	p := &Profile{
		Name:         "Test Agent",
		Description:  "A test agent",
		SystemPrompt: "You are a test agent.",
		Tools:        []string{"read_file", "run_command"},
	}

	err := Save(dir, p)
	require.NoError(t, err)
	assert.Equal(t, "test_agent", p.ID)
	assert.False(t, p.CreatedAt.IsZero())
	assert.False(t, p.UpdatedAt.IsZero())

	loaded, err := Load(dir, "test_agent")
	require.NoError(t, err)
	assert.Equal(t, "Test Agent", loaded.Name)
	assert.Equal(t, "A test agent", loaded.Description)
	assert.Equal(t, "You are a test agent.", loaded.SystemPrompt)
	assert.Equal(t, []string{"read_file", "run_command"}, loaded.Tools)
}

func TestLoadAll(t *testing.T) {
	dir := setupTestDir(t)

	Save(dir, &Profile{Name: "Agent A", Description: "First"})
	Save(dir, &Profile{Name: "Agent B", Description: "Second"})

	agents, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}

func TestLoadAllEmpty(t *testing.T) {
	dir := setupTestDir(t)
	agents, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Nil(t, agents)
}

func TestLoadNotFound(t *testing.T) {
	dir := setupTestDir(t)
	_, err := Load(dir, "nonexistent")
	assert.Error(t, err)
}

func TestDelete(t *testing.T) {
	dir := setupTestDir(t)
	Save(dir, &Profile{Name: "To Delete", Description: "temp"})

	err := Delete(dir, "to_delete")
	require.NoError(t, err)

	_, err = Load(dir, "to_delete")
	assert.Error(t, err)
}

func TestDeleteNotFound(t *testing.T) {
	dir := setupTestDir(t)
	err := Delete(dir, "nonexistent")
	assert.Error(t, err)
}

func TestSaveEmptyName(t *testing.T) {
	dir := setupTestDir(t)
	err := Save(dir, &Profile{Name: "", Description: "no name"})
	assert.Error(t, err)
}

func TestSaveUpdate(t *testing.T) {
	dir := setupTestDir(t)

	Save(dir, &Profile{Name: "Updatable", Description: "v1"})
	p, _ := Load(dir, "updatable")
	origCreated := p.CreatedAt

	p.Description = "v2"
	Save(dir, p)

	updated, _ := Load(dir, "updatable")
	assert.Equal(t, "v2", updated.Description)
	assert.Equal(t, origCreated, updated.CreatedAt) // created_at preserved
}
