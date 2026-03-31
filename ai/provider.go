package ai

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall // populated when the assistant requests tool use
	ToolCallID string     // set on role="tool" messages to match the originating call
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema for the tool's parameters
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string // raw JSON arguments from the model
}

type ToolResult struct {
	ToolCallID string
	Content    string
}

type CompletionRequest struct {
	Model       string
	MaxTokens   int
	Temperature float64
	Messages    []Message
	Tools       []Tool
}

type StreamChunk struct {
	Content string
	Done    bool
	Err     error
}

type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (string, error)
	CompleteWithTools(ctx context.Context, req CompletionRequest) (Message, error)
	StreamComplete(ctx context.Context, req CompletionRequest, ch chan<- StreamChunk)
	Name() string
}
