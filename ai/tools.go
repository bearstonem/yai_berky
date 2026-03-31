package ai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ekkinox/yai/run"
)

var runCommandSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"command": {
			"type": "string",
			"description": "The shell command to execute via bash -c"
		},
		"working_directory": {
			"type": "string",
			"description": "Working directory for the command. Defaults to the user's home directory if not specified."
		}
	},
	"required": ["command"]
}`)

var readFileSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "Absolute path to the file to read"
		}
	},
	"required": ["path"]
}`)

var listDirectorySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "Absolute path to the directory to list"
		}
	},
	"required": ["path"]
}`)

var writeFileSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "Absolute path to the file to write"
		},
		"content": {
			"type": "string",
			"description": "Content to write to the file (plain text). Prefer content_base64 for large/multiline code to avoid JSON escaping issues."
		},
		"content_base64": {
			"type": "string",
			"description": "Base64-encoded content to write. Use this for large or complex multiline content."
		}
	},
	"required": ["path"]
}`)

func AgentTools() []Tool {
	return []Tool{
		{
			Name:        "run_command",
			Description: "Execute a shell command and return its stdout, stderr, and exit code. Use for running programs, installing packages, checking system state, etc.",
			Parameters:  runCommandSchema,
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file at the given absolute path.",
			Parameters:  readFileSchema,
		},
		{
			Name:        "list_directory",
			Description: "List the files and directories at the given absolute path.",
			Parameters:  listDirectorySchema,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file at the given absolute path, creating it if it doesn't exist or overwriting if it does.",
			Parameters:  writeFileSchema,
		},
	}
}

type ToolExecutor struct {
	allowSudo  bool
	homeDir    string
	remoteHost string
}

func NewToolExecutor(allowSudo bool, homeDir string) *ToolExecutor {
	return &ToolExecutor{
		allowSudo: allowSudo,
		homeDir:   homeDir,
	}
}

func (te *ToolExecutor) SetRemoteHost(host string, remoteHomeDir string) {
	te.remoteHost = host
	if remoteHomeDir != "" {
		te.homeDir = remoteHomeDir
	}
}

func (te *ToolExecutor) IsRemote() bool {
	return te.remoteHost != ""
}

func (te *ToolExecutor) Execute(tc ToolCall) ToolResult {
	var content string

	switch tc.Name {
	case "run_command":
		content = te.executeRunCommand(tc.Arguments)
	case "read_file":
		content = te.executeReadFile(tc.Arguments)
	case "list_directory":
		content = te.executeListDirectory(tc.Arguments)
	case "write_file":
		content = te.executeWriteFile(tc.Arguments)
	default:
		content = fmt.Sprintf("unknown tool: %s", tc.Name)
	}

	return ToolResult{
		ToolCallID: tc.ID,
		Content:    content,
	}
}

func (te *ToolExecutor) executeRunCommand(argsJSON string) string {
	var args struct {
		Command          string `json:"command"`
		WorkingDirectory string `json:"working_directory"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	if !te.allowSudo && run.CommandContainsSudo(args.Command) {
		return "error: sudo is not allowed. The user must enable sudo in settings (USER_ALLOW_SUDO=true)."
	}

	workDir := args.WorkingDirectory
	if workDir == "" {
		workDir = te.homeDir
	}

	var output *run.CapturedOutput
	var err error

	if te.IsRemote() {
		remoteCmd := args.Command
		if workDir != "" {
			remoteCmd = fmt.Sprintf("cd %s && %s", shellQuote(workDir), args.Command)
		}
		output, err = run.CaptureSSHCommand(te.remoteHost, remoteCmd, 60*time.Second)
	} else {
		output, err = run.CaptureCommand(args.Command, workDir, 60*time.Second)
	}

	if err != nil {
		return fmt.Sprintf("error executing command: %s", err)
	}

	return formatCapturedOutput(output)
}

func (te *ToolExecutor) executeReadFile(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	if te.IsRemote() {
		output, err := run.CaptureSSHCommand(te.remoteHost, fmt.Sprintf("cat %s", shellQuote(args.Path)), 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error reading file: %s", err)
		}
		if output.ExitCode != 0 {
			return fmt.Sprintf("error reading file: %s", output.Stderr)
		}
		content := output.Stdout
		if len(content) > run.MaxOutputBytes {
			content = content[:run.MaxOutputBytes] + "\n... [file truncated at 50KB]"
		}
		return content
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return fmt.Sprintf("error reading file: %s", err)
	}

	content := string(data)
	if len(content) > run.MaxOutputBytes {
		content = content[:run.MaxOutputBytes] + "\n... [file truncated at 50KB]"
	}

	return content
}

func (te *ToolExecutor) executeListDirectory(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	if te.IsRemote() {
		output, err := run.CaptureSSHCommand(te.remoteHost, fmt.Sprintf("ls -la %s", shellQuote(args.Path)), 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error listing directory: %s", err)
		}
		if output.ExitCode != 0 {
			return fmt.Sprintf("error listing directory: %s", output.Stderr)
		}
		content := output.Stdout
		if content == "" {
			return "(empty directory)"
		}
		if len(content) > run.MaxOutputBytes {
			content = content[:run.MaxOutputBytes] + "\n... [listing truncated at 50KB]"
		}
		return content
	}

	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return fmt.Sprintf("error listing directory: %s", err)
	}

	var result strings.Builder
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			result.WriteString(fmt.Sprintf("%s (error reading info)\n", entry.Name()))
			continue
		}
		suffix := ""
		if entry.IsDir() {
			suffix = "/"
		}
		result.WriteString(fmt.Sprintf("%s  %8d  %s%s\n",
			info.ModTime().Format("2006-01-02 15:04"),
			info.Size(),
			entry.Name(),
			suffix,
		))
	}

	if result.Len() == 0 {
		return "(empty directory)"
	}

	output := result.String()
	if len(output) > run.MaxOutputBytes {
		output = output[:run.MaxOutputBytes] + "\n... [listing truncated at 50KB]"
	}

	return output
}

func (te *ToolExecutor) executeWriteFile(argsJSON string) string {
	var args struct {
		Path          string `json:"path"`
		Content       string `json:"content"`
		ContentBase64 string `json:"content_base64"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	content := args.Content
	if args.ContentBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(args.ContentBase64)
		if err != nil {
			return fmt.Sprintf("error decoding content_base64: %s", err)
		}
		content = string(decoded)
	}
	if content == "" {
		return "error: missing content. Provide either content or content_base64."
	}

	if te.IsRemote() {
		dir := filepath.Dir(args.Path)
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		remoteCmd := fmt.Sprintf("mkdir -p %s && base64 -d > %s", shellQuote(dir), shellQuote(args.Path))
		stdin := strings.NewReader(encoded)
		output, err := run.CaptureSSHCommandWithStdin(te.remoteHost, remoteCmd, stdin, 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error writing file: %s", err)
		}
		if output.ExitCode != 0 {
			return fmt.Sprintf("error writing file: %s", output.Stderr)
		}
		return fmt.Sprintf("successfully wrote %d bytes to %s (remote)", len(content), args.Path)
	}

	dir := filepath.Dir(args.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("error creating directory %s: %s", dir, err)
	}

	if err := os.WriteFile(args.Path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("error writing file: %s", err)
	}

	return fmt.Sprintf("successfully wrote %d bytes to %s", len(content), args.Path)
}

func formatCapturedOutput(output *run.CapturedOutput) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("exit_code: %d\n", output.ExitCode))
	if output.Stdout != "" {
		result.WriteString(fmt.Sprintf("stdout:\n%s\n", output.Stdout))
	}
	if output.Stderr != "" {
		result.WriteString(fmt.Sprintf("stderr:\n%s\n", output.Stderr))
	}
	if output.Stdout == "" && output.Stderr == "" {
		result.WriteString("(no output)\n")
	}
	return result.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
