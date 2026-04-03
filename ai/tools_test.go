package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/hook"
	"github.com/ekkinox/yai/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTools(t *testing.T) {
	tools := AgentTools()
	assert.Len(t, tools, 7)

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
	assert.Contains(t, names, "edit_file")
	assert.Contains(t, names, "search_files")
	assert.Contains(t, names, "find_files")
}

func TestToolExecutorRunCommand(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

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
		teSudo := NewToolExecutor(true, "/tmp", "/tmp", config.PermFullAccess)
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
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

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
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

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
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

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

func TestToolExecutorWriteFileBase64(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	outFile := filepath.Join(t.TempDir(), "output_b64.txt")

	encoded := "d3JpdHRlbiBjb250ZW50" // base64("written content")
	tc := ToolCall{
		ID:        "call_8b",
		Name:      "write_file",
		Arguments: `{"path": "` + outFile + `", "content_base64": "` + encoded + `"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "successfully wrote")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "written content", string(data))
}

func TestToolExecutorWriteFileLines(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	outFile := filepath.Join(t.TempDir(), "output_lines.txt")

	tc := ToolCall{
		ID:   "call_8c",
		Name: "write_file",
		Arguments: `{"path": "` + outFile + `", "content_lines": ["line 1", "line 2", "line 3"]}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "successfully wrote")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "line 1\nline 2\nline 3", string(data))
}

func TestToolExecutorUnknownTool(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	tc := ToolCall{
		ID:        "call_9",
		Name:      "unknown_tool",
		Arguments: `{}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "unknown tool")
}

func TestToolExecutorBadJSON(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	tc := ToolCall{
		ID:        "call_10",
		Name:      "run_command",
		Arguments: `{invalid json}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "error parsing arguments")
}

func TestToolExecutorRemoteFlag(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)
	assert.False(t, te.IsRemote())

	te.SetRemoteHost("user@host", "/home/user", "/home/user/project")
	assert.True(t, te.IsRemote())
	assert.Equal(t, "user@host", te.remoteHost)
	assert.Equal(t, "/home/user", te.homeDir)
	assert.Equal(t, "/home/user/project", te.workDir)
}

func TestToolExecutorSetRemoteHostPreservesHomeDir(t *testing.T) {
	te := NewToolExecutor(false, "/local/home", "/local/home", config.PermFullAccess)
	assert.Equal(t, "/local/home", te.homeDir)

	te.SetRemoteHost("user@host", "/remote/home", "/remote/work")
	assert.Equal(t, "/remote/home", te.homeDir)
	assert.Equal(t, "/remote/work", te.workDir)

	te.SetRemoteHost("user@host", "", "")
	assert.Equal(t, "/remote/home", te.homeDir)
	assert.Equal(t, "/remote/work", te.workDir)
}

func TestToolExecutorWorkDirDefaults(t *testing.T) {
	t.Run("workDir used as default for commands", func(t *testing.T) {
		te := NewToolExecutor(false, "/home/user", "/home/user/project", config.PermFullAccess)
		assert.Equal(t, "/home/user/project", te.workDir)
	})

	t.Run("workDir falls back to homeDir when empty", func(t *testing.T) {
		te := NewToolExecutor(false, "/home/user", "", config.PermFullAccess)
		assert.Equal(t, "/home/user", te.workDir)
	})
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"with'quote", "'with'\"'\"'quote'"},
		{"", "''"},
		{"/path/to/file", "'/path/to/file'"},
		{"hello world's best", "'hello world'\"'\"'s best'"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellQuote(tt.input))
		})
	}
}

func TestFormatCapturedOutput(t *testing.T) {
	t.Run("with stdout only", func(t *testing.T) {
		output := &run.CapturedOutput{ExitCode: 0, Stdout: "hello\n", Stderr: ""}
		result := formatCapturedOutput(output)
		assert.Contains(t, result, "exit_code: 0")
		assert.Contains(t, result, "stdout:\nhello\n")
		assert.NotContains(t, result, "stderr:")
	})

	t.Run("with stderr only", func(t *testing.T) {
		output := &run.CapturedOutput{ExitCode: 1, Stdout: "", Stderr: "error\n"}
		result := formatCapturedOutput(output)
		assert.Contains(t, result, "exit_code: 1")
		assert.Contains(t, result, "stderr:\nerror\n")
		assert.NotContains(t, result, "stdout:")
	})

	t.Run("no output", func(t *testing.T) {
		output := &run.CapturedOutput{ExitCode: 0, Stdout: "", Stderr: ""}
		result := formatCapturedOutput(output)
		assert.Contains(t, result, "(no output)")
	})

	t.Run("both stdout and stderr", func(t *testing.T) {
		output := &run.CapturedOutput{ExitCode: 0, Stdout: "out\n", Stderr: "err\n"}
		result := formatCapturedOutput(output)
		assert.Contains(t, result, "stdout:\nout\n")
		assert.Contains(t, result, "stderr:\nerr\n")
	})
}

func TestToolExecutorEditFile(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	t.Run("successful edit", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "edit.txt")
		err := os.WriteFile(tmpFile, []byte("hello world\nfoo bar\n"), 0644)
		require.NoError(t, err)

		tc := ToolCall{
			ID:        "call_edit_1",
			Name:      "edit_file",
			Arguments: `{"path": "` + tmpFile + `", "old_string": "foo bar", "new_string": "baz qux"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "successfully edited")

		data, err := os.ReadFile(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "hello world\nbaz qux\n", string(data))
	})

	t.Run("old_string not found", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "edit2.txt")
		err := os.WriteFile(tmpFile, []byte("hello world\n"), 0644)
		require.NoError(t, err)

		tc := ToolCall{
			ID:        "call_edit_2",
			Name:      "edit_file",
			Arguments: `{"path": "` + tmpFile + `", "old_string": "not here", "new_string": "replacement"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "not found")
	})

	t.Run("old_string ambiguous", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "edit3.txt")
		err := os.WriteFile(tmpFile, []byte("foo\nfoo\n"), 0644)
		require.NoError(t, err)

		tc := ToolCall{
			ID:        "call_edit_3",
			Name:      "edit_file",
			Arguments: `{"path": "` + tmpFile + `", "old_string": "foo", "new_string": "bar"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "found 2 times")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_edit_4",
			Name:      "edit_file",
			Arguments: `{"path": "/tmp/nonexistent_edit_xyz", "old_string": "a", "new_string": "b"}`,
		}
		result := te.Execute(tc)
		assert.Contains(t, result.Content, "error reading file")
	})
}

func TestToolExecutorSearchFiles(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\nfunc hello() {}\n"), 0644)
	require.NoError(t, err)

	tc := ToolCall{
		ID:        "call_search_1",
		Name:      "search_files",
		Arguments: `{"pattern": "func hello", "path": "` + dir + `"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "func hello")
}

func TestToolExecutorFindFiles(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme\n"), 0644)
	require.NoError(t, err)

	tc := ToolCall{
		ID:        "call_find_1",
		Name:      "find_files",
		Arguments: `{"pattern": "*.go", "path": "` + dir + `"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "main.go")
	assert.NotContains(t, result.Content, "readme.md")
}

func TestToolExecutorHookDenyBlocks(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "echo 'not allowed' && exit 1", Name: "block-all"},
	}
	te.SetHookRunner(hook.NewRunner(hooks, "/tmp"))

	tc := ToolCall{
		ID:        "call_hook_1",
		Name:      "run_command",
		Arguments: `{"command": "echo hello"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "blocked by hook")
	assert.Contains(t, result.Content, "not allowed")
}

func TestToolExecutorHookAllowPasses(t *testing.T) {
	te := NewToolExecutor(false, "/tmp", "/tmp", config.PermFullAccess)
	hooks := []config.HookConfig{
		{Event: config.HookPreToolUse, Command: "exit 0", Name: "allow-all"},
	}
	te.SetHookRunner(hook.NewRunner(hooks, "/tmp"))

	tc := ToolCall{
		ID:        "call_hook_2",
		Name:      "run_command",
		Arguments: `{"command": "echo hello"}`,
	}
	result := te.Execute(tc)
	assert.Contains(t, result.Content, "hello")
	assert.Contains(t, result.Content, "exit_code: 0")
}
