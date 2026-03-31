package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTools(t *testing.T) {
	tools := AgentTools()
	assert.Len(t, tools, 4)

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
		assert.NotEmpty(t, tool.Description)
		assert.True(t, json.Valid(tool.Parameters))
	}
	assert.Contains(t, names, "run_command")
	assert.Contains(t, names, "read_file")
	assert.Contains(t, names, "list_directory")
	assert.Contains(t, names, "write_file")
}

func TestToolExecutorRunCommand(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	t.Run("successful command", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_1",
			Name:      "run_command",
			Arguments: `{"command": "echo hello"}`,
		}
		result := te.Execute(tc)
		assert.Equal(t, "call_1", result.ToolCallID)
		assert.Contains(t, result.Content, "exit_code: 0")
		assert.Contains(t, result.Content, "hello")
	})

	t.Run("failing command", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_2",
			Name:      "run_command",
			Arguments: `{"command": "false"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "exit_code: 1")
	})

	t.Run("sudo blocked", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_3",
			Name:      "run_command",
			Arguments: `{"command": "sudo ls"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "sudo is not allowed")
	})

	t.Run("sudo allowed", func(t *testing.T) {
		teSudo := NewToolExecutor(true, "/tmp")
		tc := ToolCall{
			ID:        "call_4",
			Name:      "run_command",
			Arguments: `{"command": "echo test"}`,
		}
		result := teSudo.Execute(tc)
		assert.Contains(t, result.Content, "exit_code: 0")
	})
}

func TestToolExecutorReadFile(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("file content"), 0644)
	require.NoError(t, err)

	t.Run("existing file", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_5",
			Name:      "read_file",
			Arguments: `{"path": "` + tmpFile + `"}`,
		}
		result := te.Execute(tc)
		assert.Equal(t, "file content", result.Content)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_6",
			Name:      "read_file",
			Arguments: `{"path": "/tmp/nonexistent_test_file_xyz"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "error reading file")
	})
}

func TestToolExecutorListDirectory(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	require.NoError(t, err)

	tc := ToolCall{
		ID:        "call_7",
		Name:      "list_directory",
		Arguments: `{"path": "` + dir + `"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "subdir/")
}

func TestToolExecutorWriteFile(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	outFile := filepath.Join(t.TempDir(), "output.txt")

	tc := ToolCall{
		ID:        "call_8",
		Name:      "write_file",
		Arguments: `{"path": "` + outFile + `", "content": "written content"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "successfully wrote")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "written content", string(data))
}

func TestToolExecutorUnknownTool(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	tc := ToolCall{
		ID:        "call_9",
		Name:      "unknown_tool",
		Arguments: `{}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "unknown tool")
}

func TestToolExecutorBadJSON(t *testing.T) {
	te := NewToolExecutor(false, "/tmp")

	tc := ToolCall{
		ID:        "call_10",
		Name:      "run_command",
		Arguments: `{invalid json}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "error parsing arguments")
}
