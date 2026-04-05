package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".config", "helm", "sessions"), 0755)
	return dir
}

func TestNewSession(t *testing.T) {
	s := NewSession("agent")
	assert.Equal(t, "agent", s.Mode)
	assert.NotEmpty(t, s.ID)
	assert.Len(t, s.ID, 8) // hex-encoded 4 bytes
	assert.False(t, s.CreatedAt.IsZero())
	assert.Empty(t, s.Messages)
}

func TestSaveAndLoad(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSession("chat")
	s.Messages = []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	err := s.Save(dir)
	require.NoError(t, err)
	assert.Equal(t, "hello", s.Summary) // auto-summary from first user msg

	loaded, err := Load(dir, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "chat", loaded.Mode)
	assert.Len(t, loaded.Messages, 2)
	assert.Equal(t, "hello", loaded.Messages[0].Content)
}

func TestList(t *testing.T) {
	dir := setupTestDir(t)

	s1 := NewSession("exec")
	s1.Messages = []Message{{Role: "user", Content: "first"}}
	s1.Save(dir)

	s2 := NewSession("agent")
	s2.Messages = []Message{{Role: "user", Content: "second"}}
	s2.Save(dir)

	sessions, err := List(dir)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	// Should be sorted newest first
	assert.True(t, sessions[0].UpdatedAt.After(sessions[1].UpdatedAt) || sessions[0].UpdatedAt.Equal(sessions[1].UpdatedAt))
}

func TestListEmpty(t *testing.T) {
	dir := setupTestDir(t)
	sessions, err := List(dir)
	require.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestDelete(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSession("chat")
	s.Save(dir)

	err := Delete(dir, s.ID)
	require.NoError(t, err)

	_, err = Load(dir, s.ID)
	assert.Error(t, err)
}

func TestAutoSummary(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSession("agent")
	s.Messages = []Message{
		{Role: "system", Content: "You are an agent"},
		{Role: "user", Content: "Build a website for my restaurant"},
	}
	s.Save(dir)

	assert.Equal(t, "Build a website for my restaurant", s.Summary)
}

func TestAutoSummaryTruncate(t *testing.T) {
	dir := setupTestDir(t)

	long := "This is a very long message that should be truncated because it exceeds the maximum summary length of eighty characters which we set"
	s := NewSession("chat")
	s.Messages = []Message{{Role: "user", Content: long}}
	s.Save(dir)

	assert.True(t, len(s.Summary) <= 80)
	assert.Contains(t, s.Summary, "...")
}

func TestExport(t *testing.T) {
	s := NewSession("exec")
	s.Messages = []Message{{Role: "user", Content: "test"}}

	data, err := s.Export()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mode": "exec"`)
	assert.Contains(t, string(data), `"content": "test"`)
}

func TestToolCallsInMessages(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSession("agent")
	s.Messages = []Message{
		{Role: "user", Content: "run ls"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "tc1", Name: "run_command", Arguments: `{"command":"ls"}`},
			},
		},
		{Role: "tool", Content: "file1.txt\nfile2.txt", ToolCallID: "tc1"},
	}
	s.Save(dir)

	loaded, err := Load(dir, s.ID)
	require.NoError(t, err)
	assert.Len(t, loaded.Messages, 3)
	assert.Len(t, loaded.Messages[1].ToolCalls, 1)
	assert.Equal(t, "run_command", loaded.Messages[1].ToolCalls[0].Name)
	assert.Equal(t, "tc1", loaded.Messages[2].ToolCallID)
}
