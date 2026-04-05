package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bearstonem/helm/config"
)

// MCPClient is a client for connecting to MCP servers via stdio.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	jsonID int
}

// MCPTool represents a tool exposed by an MCP server.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPToolsResult is the result of listing tools.
type MCPToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPCallResult is the result of calling a tool.
type MCPCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents content returned from an MCP tool.
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// JSONRPCRequest represents a JSON-RPC request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError  `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC error.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewMCPClient creates a new MCP client for the given integration config.
func NewMCPClient(ic config.IntegrationConfig) (*MCPClient, error) {
	if ic.Command == "" {
		return nil, fmt.Errorf("MCP command is required")
	}

	args := ic.Args
	if args == nil {
		args = []string{}
	}

	cmd := exec.Command(ic.Command, args...)

	// Set environment variables
	env := os.Environ()
	for k, v := range ic.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	client := &MCPClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}

	// Initialize the MCP connection
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return client, nil
}

// Close terminates the MCP server process.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.cmd.Wait()
	return nil
}

// initialize sends the MCP initialize request.
func (c *MCPClient) initialize() error {
	params := json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"helm","version":"1.0.0"}}`)

	resp, err := c.sendRequest("initialize", params)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	return nil
}

// ListTools returns the list of available tools from the MCP server.
func (c *MCPClient) ListTools() ([]MCPTool, error) {
	resp, err := c.sendRequest("tools/list", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("list tools error: %s", resp.Error.Message)
	}

	var result MCPToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools result: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool on the MCP server.
func (c *MCPClient) CallTool(name string, arguments map[string]interface{}) (*MCPCallResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	resp, err := c.sendRequest("tools/call", paramsJSON)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("call tool error: %s", resp.Error.Message)
	}

	var result MCPCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse call result: %w", err)
	}

	return &result, nil
}

// sendRequest sends a JSON-RPC request and waits for the response.
func (c *MCPClient) sendRequest(method string, params json.RawMessage) (*JSONRPCResponse, error) {
	c.mu.Lock()
	c.jsonID++
	reqID := c.jsonID
	c.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Send the request
	c.mu.Lock()
	_, err = c.stdin.Write(append(reqJSON, '\n'))
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read the response with a timeout
	resultChan := make(chan *JSONRPCResponse, 1)
	errChan := make(chan error, 1)

	go func() {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			errChan <- fmt.Errorf("read response: %w", err)
			return
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			errChan <- fmt.Errorf("parse response: %w", err)
			return
		}
		resultChan <- &resp
	}()

	select {
	case resp := <-resultChan:
		return resp, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for MCP response")
	}
}

// BuildMCPToolDefs creates tool definitions for an MCP integration.
func BuildMCPToolDefs(ic config.IntegrationConfig) ([]ToolDef, error) {
	client, err := NewMCPClient(ic)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	tools, err := client.ListTools()
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	var defs []ToolDef
	prefix := sanitizeToolName(ic.Name) + "_"

	for _, tool := range tools {
		schema := tool.InputSchema
		if schema == nil || string(schema) == "null" {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		defs = append(defs, ToolDef{
			Name:        prefix + tool.Name,
			Description: tool.Description,
			Parameters:  schema,
		})
	}

	return defs, nil
}

// ExecuteMCPTool executes a specific tool on an MCP server.
func ExecuteMCPTool(ic config.IntegrationConfig, toolName string, argsJSON string) ToolResult {
	client, err := NewMCPClient(ic)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error connecting to MCP server: %s", err)}
	}
	defer client.Close()

	// Strip the prefix from the tool name
	prefix := sanitizeToolName(ic.Name) + "_"
	if len(toolName) <= len(prefix) {
		return ToolResult{Content: fmt.Sprintf("error: invalid tool name %s", toolName)}
	}
	actualToolName := toolName[len(prefix):]

	// Parse arguments
	var args map[string]interface{}
	if argsJSON != "" && argsJSON != "{}" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return ToolResult{Content: fmt.Sprintf("error parsing arguments: %s", err)}
		}
	} else {
		args = make(map[string]interface{})
	}

	result, err := client.CallTool(actualToolName, args)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("error calling tool: %s", err)}
	}

	if result.IsError {
		var errMsg string
		for _, c := range result.Content {
			errMsg += c.Text + "\n"
		}
		return ToolResult{Content: fmt.Sprintf("tool error: %s", errMsg)}
	}

	var output strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" {
			output.WriteString(c.Text)
			output.WriteString("\n")
		}
	}

	return ToolResult{Content: strings.TrimSpace(output.String())}
}
