package ai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ekkinox/yai/config"
	"github.com/ekkinox/yai/hook"
	"github.com/ekkinox/yai/integration"
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
			"description": "Working directory for the command. Defaults to the current workspace if not specified."
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
			"description": "Content to write to the file (plain text). For large or multiline code, prefer content_lines to avoid newline escaping issues."
		},
		"content_lines": {
			"type": "array",
			"items": { "type": "string" },
			"description": "Content as an array of lines (joined with \\n). Recommended for large multiline code."
		},
		"content_base64": {
			"type": "string",
			"description": "Base64-encoded content to write. Use this for large or complex multiline content."
		}
	},
	"required": ["path"]
}`)

var editFileSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "Absolute path to the file to edit. The file must already exist."
		},
		"old_string": {
			"type": "string",
			"description": "The exact text to find in the file. Must match uniquely (appears exactly once). Include enough surrounding context to ensure uniqueness."
		},
		"new_string": {
			"type": "string",
			"description": "The replacement text. To delete a section, use an empty string."
		}
	},
	"required": ["path", "old_string", "new_string"]
}`)

var searchFilesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"pattern": {
			"type": "string",
			"description": "The regex pattern to search for in file contents"
		},
		"path": {
			"type": "string",
			"description": "Directory to search in. Defaults to the working directory."
		},
		"include": {
			"type": "string",
			"description": "Glob pattern to filter files (e.g. '*.go', '*.js'). Optional."
		},
		"case_insensitive": {
			"type": "boolean",
			"description": "Perform case-insensitive matching. Default false."
		},
		"context_lines": {
			"type": "integer",
			"description": "Number of context lines to show before and after each match. Default 0."
		},
		"max_results": {
			"type": "integer",
			"description": "Maximum number of matching lines to return. Default 100."
		}
	},
	"required": ["pattern"]
}`)

var findFilesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"pattern": {
			"type": "string",
			"description": "Glob pattern to match file names (e.g. '*.go', 'Makefile', '**/*.test.js')"
		},
		"path": {
			"type": "string",
			"description": "Directory to search in. Defaults to the working directory."
		},
		"max_results": {
			"type": "integer",
			"description": "Maximum number of files to return. Default 100."
		},
		"type": {
			"type": "string",
			"description": "Filter by type: 'f' for files only, 'd' for directories only. Default: both.",
			"enum": ["f", "d"]
		}
	},
	"required": ["pattern"]
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
			Description: "Read the contents of a file at the given absolute path. Always read a file before editing it.",
			Parameters:  readFileSchema,
		},
		{
			Name:        "list_directory",
			Description: "List the files and directories at the given absolute path.",
			Parameters:  listDirectorySchema,
		},
		{
			Name:        "write_file",
			Description: "Create a new file or completely overwrite an existing file. For modifying existing files, prefer edit_file instead.",
			Parameters:  writeFileSchema,
		},
		{
			Name:        "edit_file",
			Description: "Make targeted edits to an existing file using search-and-replace. Finds the exact old_string in the file and replaces it with new_string. The old_string must match exactly once in the file. Always read_file first before editing.",
			Parameters:  editFileSchema,
		},
		{
			Name:        "search_files",
			Description: "Search file contents using a regex pattern, like grep. Returns matching lines with file paths and line numbers. Supports context lines, case insensitivity, and max results. Use this instead of run_command with grep.",
			Parameters:  searchFilesSchema,
		},
		{
			Name:        "find_files",
			Description: "Find files by name pattern using glob matching. Returns matching file paths. Supports max results and type filtering. Use this instead of run_command with find or ls.",
			Parameters:  findFilesSchema,
		},
	}
}

type ToolExecutor struct {
	allowSudo        bool
	homeDir          string
	workDir          string // current working directory / workspace root — default for commands and searches
	remoteHost       string
	permissionMode   config.PermissionMode
	hookRunner       *hook.Runner
	integrationTools []integration.IntegrationTool
}

func NewToolExecutor(allowSudo bool, homeDir string, workDir string, permMode config.PermissionMode) *ToolExecutor {
	if workDir == "" {
		workDir = homeDir
	}
	return &ToolExecutor{
		allowSudo:      allowSudo,
		homeDir:        homeDir,
		workDir:        workDir,
		permissionMode: permMode,
	}
}

func (te *ToolExecutor) SetRemoteHost(host string, remoteHomeDir string, remoteWorkDir string) {
	te.remoteHost = host
	if remoteHomeDir != "" {
		te.homeDir = remoteHomeDir
	}
	if remoteWorkDir != "" {
		te.workDir = remoteWorkDir
	} else if remoteHomeDir != "" {
		te.workDir = remoteHomeDir
	}
}

func (te *ToolExecutor) SetHookRunner(r *hook.Runner) {
	te.hookRunner = r
}

func (te *ToolExecutor) SetIntegrations(tools []integration.IntegrationTool) {
	te.integrationTools = tools
}

// AllTools returns built-in tools plus any integration tools.
func (te *ToolExecutor) AllTools() []Tool {
	tools := AgentTools()
	for _, it := range te.integrationTools {
		tools = append(tools, Tool{
			Name:        it.Def.Name,
			Description: it.Def.Description,
			Parameters:  it.Def.Parameters,
		})
	}
	return tools
}

func (te *ToolExecutor) IsRemote() bool {
	return te.remoteHost != ""
}

func (te *ToolExecutor) findIntegrationTool(name string) *integration.IntegrationTool {
	for i := range te.integrationTools {
		if te.integrationTools[i].Def.Name == name {
			return &te.integrationTools[i]
		}
	}
	return nil
}

func (te *ToolExecutor) Execute(tc ToolCall) ToolResult {
	// Permission enforcement
	if !config.IsToolAllowed(tc.Name, te.permissionMode) {
		return ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("error: tool %q is not allowed in %s permission mode. The user must change USER_PERMISSION_MODE in settings.", tc.Name, te.permissionMode.String()),
		}
	}

	// PreToolUse hooks
	if te.hookRunner != nil {
		result := te.hookRunner.RunPreToolUse(tc.Name, tc.Arguments)
		if result.Action == config.HookDeny {
			return ToolResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("error: tool %q blocked by hook: %s", tc.Name, result.Message),
			}
		}
	}

	var content string
	var diff string

	switch tc.Name {
	case "run_command":
		content = te.executeRunCommand(tc.Arguments)
	case "read_file":
		content = te.executeReadFile(tc.Arguments)
	case "list_directory":
		content = te.executeListDirectory(tc.Arguments)
	case "write_file":
		content, diff = te.executeWriteFile(tc.Arguments)
	case "edit_file":
		content, diff = te.executeEditFile(tc.Arguments)
	case "search_files":
		content = te.executeSearchFiles(tc.Arguments)
	case "find_files":
		content = te.executeFindFiles(tc.Arguments)
	default:
		// Check integration tools
		if it := te.findIntegrationTool(tc.Name); it != nil {
			result := integration.Execute(*it, tc.Arguments)
			content = result.Content
		} else {
			content = fmt.Sprintf("unknown tool: %s", tc.Name)
		}
	}

	// PostToolUse hooks
	if te.hookRunner != nil {
		te.hookRunner.RunPostToolUse(tc.Name, tc.Arguments, content)
	}

	return ToolResult{
		ToolCallID: tc.ID,
		Content:    content,
		Diff:       diff,
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
		workDir = te.workDir
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

func (te *ToolExecutor) executeWriteFile(argsJSON string) (string, string) {
	var args struct {
		Path          string   `json:"path"`
		Content       string   `json:"content"`
		ContentLines  []string `json:"content_lines"`
		ContentBase64 string   `json:"content_base64"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err), ""
	}

	content := args.Content
	if len(args.ContentLines) > 0 {
		content = strings.Join(args.ContentLines, "\n")
	}
	if args.ContentBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(args.ContentBase64)
		if err != nil {
			return fmt.Sprintf("error decoding content_base64: %s", err), ""
		}
		content = string(decoded)
	}
	if content == "" {
		return "error: missing content. Provide content, content_lines, or content_base64.", ""
	}

	if te.IsRemote() {
		dir := filepath.Dir(args.Path)
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		remoteCmd := fmt.Sprintf("mkdir -p %s && base64 -d > %s", shellQuote(dir), shellQuote(args.Path))
		stdin := strings.NewReader(encoded)
		output, err := run.CaptureSSHCommandWithStdin(te.remoteHost, remoteCmd, stdin, 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error writing file: %s", err), ""
		}
		if output.ExitCode != 0 {
			return fmt.Sprintf("error writing file: %s", output.Stderr), ""
		}
		return fmt.Sprintf("successfully wrote %d bytes to %s (remote)", len(content), args.Path), ""
	}

	// Capture existing content for diff
	oldContent := ""
	if existing, err := os.ReadFile(args.Path); err == nil {
		oldContent = string(existing)
	}

	dir := filepath.Dir(args.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("error creating directory %s: %s", dir, err), ""
	}

	if err := os.WriteFile(args.Path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("error writing file: %s", err), ""
	}

	diff := generateUnifiedDiff(args.Path, oldContent, content)
	return fmt.Sprintf("successfully wrote %d bytes to %s", len(content), args.Path), diff
}

func (te *ToolExecutor) executeEditFile(argsJSON string) (string, string) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err), ""
	}

	if te.IsRemote() {
		// Read the remote file, perform replacement, write back
		readOutput, err := run.CaptureSSHCommand(te.remoteHost, fmt.Sprintf("cat %s", shellQuote(args.Path)), 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error reading file: %s", err), ""
		}
		if readOutput.ExitCode != 0 {
			return fmt.Sprintf("error reading file: %s", readOutput.Stderr), ""
		}
		content := readOutput.Stdout
		count := strings.Count(content, args.OldString)
		if count == 0 {
			return "error: old_string not found in file. Make sure it matches exactly, including whitespace and indentation.", ""
		}
		if count > 1 {
			return fmt.Sprintf("error: old_string found %d times in file. It must be unique. Include more surrounding context to disambiguate.", count), ""
		}
		newContent := strings.Replace(content, args.OldString, args.NewString, 1)
		encoded := base64.StdEncoding.EncodeToString([]byte(newContent))
		remoteCmd := fmt.Sprintf("base64 -d > %s", shellQuote(args.Path))
		stdin := strings.NewReader(encoded)
		writeOutput, err := run.CaptureSSHCommandWithStdin(te.remoteHost, remoteCmd, stdin, 30*time.Second)
		if err != nil {
			return fmt.Sprintf("error writing file: %s", err), ""
		}
		if writeOutput.ExitCode != 0 {
			return fmt.Sprintf("error writing file: %s", writeOutput.Stderr), ""
		}
		return fmt.Sprintf("successfully edited %s (remote)", args.Path), ""
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return fmt.Sprintf("error reading file: %s", err), ""
	}

	content := string(data)
	count := strings.Count(content, args.OldString)
	if count == 0 {
		return "error: old_string not found in file. Make sure it matches exactly, including whitespace and indentation.", ""
	}
	if count > 1 {
		return fmt.Sprintf("error: old_string found %d times in file. It must be unique. Include more surrounding context to disambiguate.", count), ""
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(args.Path, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("error writing file: %s", err), ""
	}

	diff := generateEditDiff(args.Path, args.OldString, args.NewString)
	return fmt.Sprintf("successfully edited %s", args.Path), diff
}

func (te *ToolExecutor) executeSearchFiles(argsJSON string) string {
	var args struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Include         string `json:"include"`
		CaseInsensitive bool   `json:"case_insensitive"`
		ContextLines    int    `json:"context_lines"`
		MaxResults      int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	searchDir := args.Path
	if searchDir == "" {
		searchDir = te.workDir
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	// Build grep flags
	flags := "-rn"
	if args.CaseInsensitive {
		flags += "i"
	}
	if args.ContextLines > 0 {
		flags += fmt.Sprintf(" -C %d", args.ContextLines)
	}

	var cmd string
	if args.Include != "" {
		cmd = fmt.Sprintf("grep %s --include=%s %s %s | head -%d",
			flags,
			shellQuote(args.Include),
			shellQuote(args.Pattern),
			shellQuote(searchDir),
			maxResults,
		)
	} else {
		cmd = fmt.Sprintf("grep %s %s %s | head -%d",
			flags,
			shellQuote(args.Pattern),
			shellQuote(searchDir),
			maxResults,
		)
	}

	var output *run.CapturedOutput
	var err error

	if te.IsRemote() {
		output, err = run.CaptureSSHCommand(te.remoteHost, cmd, 30*time.Second)
	} else {
		output, err = run.CaptureCommand(cmd, searchDir, 30*time.Second)
	}

	if err != nil {
		return fmt.Sprintf("error searching: %s", err)
	}
	if output.Stdout == "" && output.ExitCode == 1 {
		return "no matches found"
	}
	result := output.Stdout
	if len(result) > run.MaxOutputBytes {
		result = result[:run.MaxOutputBytes] + "\n... [results truncated]"
	}
	return result
}

func (te *ToolExecutor) executeFindFiles(argsJSON string) string {
	var args struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}

	searchDir := args.Path
	if searchDir == "" {
		searchDir = te.workDir
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	typeFilter := ""
	if args.Type == "f" {
		typeFilter = "-type f "
	} else if args.Type == "d" {
		typeFilter = "-type d "
	}

	cmd := fmt.Sprintf("find %s %s-name %s -not -path '*/\\.git/*' 2>/dev/null | head -%d",
		shellQuote(searchDir),
		typeFilter,
		shellQuote(args.Pattern),
		maxResults,
	)

	var output *run.CapturedOutput
	var err error

	if te.IsRemote() {
		output, err = run.CaptureSSHCommand(te.remoteHost, cmd, 30*time.Second)
	} else {
		output, err = run.CaptureCommand(cmd, searchDir, 30*time.Second)
	}

	if err != nil {
		return fmt.Sprintf("error finding files: %s", err)
	}
	if output.Stdout == "" {
		return "no files found matching pattern"
	}
	result := output.Stdout
	if len(result) > run.MaxOutputBytes {
		result = result[:run.MaxOutputBytes] + "\n... [results truncated]"
	}
	return result
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

// generateUnifiedDiff produces a unified diff between old and new file content.
// Used for write_file where we have full before/after content.
func generateUnifiedDiff(path string, oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	if len(oldLines) == 0 {
		// New file — show all lines as additions (capped)
		return formatNewFileDiff(path, newLines)
	}

	return computeUnifiedDiff(path, oldLines, newLines)
}

// generateEditDiff produces a focused diff showing old_string → new_string replacement.
func generateEditDiff(path string, oldString, newString string) string {
	if oldString == newString {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", path, path))

	oldLines := splitLines(oldString)
	newLines := splitLines(newString)

	b.WriteString(fmt.Sprintf("@@ -%d +%d @@\n", len(oldLines), len(newLines)))
	for _, line := range oldLines {
		b.WriteString("-")
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, line := range newLines {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func formatNewFileDiff(path string, lines []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- /dev/null\n+++ %s\n", path))

	maxLines := 40
	showing := lines
	if len(showing) > maxLines {
		showing = showing[:maxLines]
	}

	b.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
	for _, line := range showing {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(lines) > maxLines {
		b.WriteString(fmt.Sprintf("... +%d more lines\n", len(lines)-maxLines))
	}
	return b.String()
}

// computeUnifiedDiff builds a simple unified diff using longest common subsequence.
// For large files, falls back to a truncated view.
func computeUnifiedDiff(path string, oldLines, newLines []string) string {
	// For very large diffs, fall back to a simple summary
	if len(oldLines)+len(newLines) > 2000 {
		return fmt.Sprintf("--- %s\n+++ %s\n@@ large change: %d lines → %d lines @@\n",
			path, path, len(oldLines), len(newLines))
	}

	// Myers-like diff: compute edit script via LCS
	type edit struct {
		op   byte // ' ', '-', '+'
		line string
	}

	// Simple O(NM) LCS for reasonable file sizes
	m, n := len(oldLines), len(newLines)
	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	// Trace back to get edits
	edits := make([]edit, 0, m+n)
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			edits = append(edits, edit{' ', oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			edits = append(edits, edit{'-', oldLines[i]})
			i++
		} else {
			edits = append(edits, edit{'+', newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		edits = append(edits, edit{'-', oldLines[i]})
	}
	for ; j < n; j++ {
		edits = append(edits, edit{'+', newLines[j]})
	}

	// Format as unified diff hunks (context=3)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", path, path))

	ctx := 3
	inHunk := false
	hunkStart := 0

	for idx, e := range edits {
		if e.op != ' ' {
			// Start a hunk if not in one
			if !inHunk {
				hunkStart = idx - ctx
				if hunkStart < 0 {
					hunkStart = 0
				}
				inHunk = true
				// Write hunk header (simplified)
				b.WriteString("@@ ... @@\n")
				// Write leading context
				for k := hunkStart; k < idx; k++ {
					b.WriteString(" ")
					b.WriteString(edits[k].line)
					b.WriteString("\n")
				}
			}
			b.WriteByte(e.op)
			b.WriteString(e.line)
			b.WriteString("\n")
		} else if inHunk {
			// Count trailing context
			endOfChanges := true
			for k := idx + 1; k < len(edits) && k <= idx+ctx; k++ {
				if edits[k].op != ' ' {
					endOfChanges = false
					break
				}
			}
			if endOfChanges && idx-hunkStart > 2*ctx+20 {
				// Close hunk after trailing context
				b.WriteString(" ")
				b.WriteString(e.line)
				b.WriteString("\n")

				// Check if we've written enough trailing context
				trailingCount := 0
				for k := idx; k >= 0 && edits[k].op == ' '; k-- {
					trailingCount++
				}
				if trailingCount >= ctx {
					inHunk = false
				}
			} else {
				b.WriteString(" ")
				b.WriteString(e.line)
				b.WriteString("\n")
			}
		}
	}

	result := b.String()
	if len(result) > 4000 {
		result = result[:4000] + "\n... [diff truncated]\n"
	}
	return result
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
