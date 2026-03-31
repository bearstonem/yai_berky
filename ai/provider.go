package ai

import "context"

type Message struct {
	Role    string
	Content string
}

type CompletionRequest struct {
	Model       string
	MaxTokens   int
	Temperature float64
	Messages    []Message
}

type StreamChunk struct {
	Content string
	Done    bool
	Err     error
}

type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (string, error)
	StreamComplete(ctx context.Context, req CompletionRequest, ch chan<- StreamChunk)
	Name() string
}
