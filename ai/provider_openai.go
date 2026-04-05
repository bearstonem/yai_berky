package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bearstonem/helm/config"
	"github.com/sashabaranov/go-openai"
)

type OpenAIProvider struct {
	client         *openai.Client
	name           string
	betaRestricted bool // true if model rejects temperature/top_p params
	lastUsage      Usage
}

type OpenAIProviderConfig struct {
	APIKey  string
	BaseURL string
	Proxy   string
	Name    string
}

func NewOpenAIProvider(cfg OpenAIProviderConfig) (*OpenAIProvider, error) {
	clientConfig := openai.DefaultConfig(cfg.APIKey)

	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}

	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, err
		}
		clientConfig.HTTPClient = &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		}
	}

	name := cfg.Name
	if name == "" {
		name = "openai"
	}

	return &OpenAIProvider{
		client: openai.NewClientWithConfig(clientConfig),
		name:   name,
	}, nil
}

func (p *OpenAIProvider) Name() string {
	return p.name
}

func (p *OpenAIProvider) LastUsage() Usage {
	return p.lastUsage
}

func (p *OpenAIProvider) toOpenAIMessages(messages []Message) []openai.ChatCompletionMessage {
	msgs := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		content := m.Content
		// The go-openai library uses `json:"content,omitempty"` which omits
		// empty strings, sending null to the API. Tool-role messages require
		// content to be a non-null string, and some providers reject null
		// content on assistant messages with tool calls too.
		if content == "" && (m.Role == "tool" || (m.Role == "assistant" && len(m.ToolCalls) > 0)) {
			content = " "
		}
		msg := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: content,
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				msg.ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
		msgs[i] = msg
	}
	return msgs
}

func (p *OpenAIProvider) toOpenAITools(tools []Tool) []openai.Tool {
	out := make([]openai.Tool, len(tools))
	for i, t := range tools {
		out[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}

func isBetaRestrictionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "beta-limitations") ||
		strings.Contains(msg, "temperature") && strings.Contains(msg, "fixed at")
}

func (p *OpenAIProvider) buildRequest(req CompletionRequest, msgs []openai.ChatCompletionMessage) openai.ChatCompletionRequest {
	apiReq := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: msgs,
	}
	// OpenAI and OpenRouter support the newer max_completion_tokens field.
	// Other providers (MiniMax, Ollama, llama.cpp, etc.) need the older max_tokens.
	switch p.name {
	case "openai", "openrouter":
		apiReq.MaxCompletionTokens = req.MaxTokens
	default:
		apiReq.MaxTokens = req.MaxTokens
	}
	if !p.betaRestricted {
		apiReq.Temperature = float32(req.Temperature)
	}
	return apiReq
}

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	msgs := p.toOpenAIMessages(req.Messages)
	apiReq := p.buildRequest(req, msgs)

	resp, err := p.client.CreateChatCompletion(ctx, apiReq)
	if err != nil && !p.betaRestricted && isBetaRestrictionError(err) {
		p.betaRestricted = true
		apiReq = p.buildRequest(req, msgs)
		resp, err = p.client.CreateChatCompletion(ctx, apiReq)
	}
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no choices returned from API")
	}

	p.lastUsage = Usage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	return resp.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) CompleteWithTools(ctx context.Context, req CompletionRequest) (Message, error) {
	// If the provider doesn't support OpenAI-style tools, use prompt-based tool calling
	if !config.ProviderSupportsTools(p.name) && len(req.Tools) > 0 {
		return p.completeWithPromptTools(ctx, req)
	}

	msgs := p.toOpenAIMessages(req.Messages)
	apiReq := p.buildRequest(req, msgs)
	if len(req.Tools) > 0 {
		apiReq.Tools = p.toOpenAITools(req.Tools)
	}

	resp, err := p.client.CreateChatCompletion(ctx, apiReq)
	if err != nil && !p.betaRestricted && isBetaRestrictionError(err) {
		p.betaRestricted = true
		apiReq = p.buildRequest(req, msgs)
		if len(req.Tools) > 0 {
			apiReq.Tools = p.toOpenAITools(req.Tools)
		}
		resp, err = p.client.CreateChatCompletion(ctx, apiReq)
	}
	if err != nil {
		return Message{}, err
	}

	if len(resp.Choices) == 0 {
		return Message{}, errors.New("no choices returned from API")
	}

	p.lastUsage = Usage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	choice := resp.Choices[0]
	result := Message{
		Role:    choice.Message.Role,
		Content: choice.Message.Content,
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return result, nil
}

// completeWithPromptTools handles tool calling for providers that don't support
// OpenAI-style function calling. Tools are described in the system prompt and
// the model responds with JSON tool calls that we parse.
func (p *OpenAIProvider) completeWithPromptTools(ctx context.Context, req CompletionRequest) (Message, error) {
	// Build a tool description to inject into the system prompt
	toolDesc := buildPromptToolDescription(req.Tools)

	// Modify messages: prepend tool instructions to system message
	modified := make([]Message, len(req.Messages))
	copy(modified, req.Messages)
	for i, m := range modified {
		if m.Role == "system" {
			modified[i].Content = m.Content + "\n\n" + toolDesc
			break
		}
	}

	// Strip tool-role messages — convert them to user messages with context
	var cleaned []Message
	for _, m := range modified {
		if m.Role == "tool" {
			cleaned = append(cleaned, Message{
				Role:    "user",
				Content: fmt.Sprintf("[tool result for %s]: %s", m.ToolCallID, m.Content),
			})
		} else if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Convert assistant tool-call messages to plain text
			content := m.Content
			for _, tc := range m.ToolCalls {
				content += fmt.Sprintf("\n[called tool %s with: %s]", tc.Name, tc.Arguments)
			}
			cleaned = append(cleaned, Message{Role: "assistant", Content: content})
		} else {
			cleaned = append(cleaned, m)
		}
	}

	modReq := CompletionRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    cleaned,
		// No tools — they're in the prompt
	}

	msgs := p.toOpenAIMessages(modReq.Messages)
	apiReq := p.buildRequest(modReq, msgs)

	resp, err := p.client.CreateChatCompletion(ctx, apiReq)
	if err != nil {
		return Message{}, err
	}

	if len(resp.Choices) == 0 {
		return Message{}, errors.New("no choices returned from API")
	}

	p.lastUsage = Usage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	content := resp.Choices[0].Message.Content
	result := Message{
		Role:    "assistant",
		Content: content,
	}

	// Try to parse tool calls from the response
	if tc := parsePromptToolCall(content); tc != nil {
		result.ToolCalls = []ToolCall{*tc}
		// Strip the JSON block from the visible content
		result.Content = stripToolCallJSON(content)
	}

	return result, nil
}

// buildPromptToolDescription creates a system prompt section describing available tools.
func buildPromptToolDescription(tools []Tool) string {
	var b strings.Builder
	b.WriteString("# Available Tools\n")
	b.WriteString("You have tools available. To use a tool, respond with a JSON block like this:\n")
	b.WriteString("```tool_call\n{\"tool\": \"tool_name\", \"arguments\": {\"param\": \"value\"}}\n```\n\n")
	b.WriteString("IMPORTANT: Only use ONE tool call per response. Wait for the result before calling another.\n")
	b.WriteString("If you don't need a tool, just respond normally with text.\n\n")
	b.WriteString("Available tools:\n\n")

	for _, t := range tools {
		b.WriteString(fmt.Sprintf("## %s\n%s\n", t.Name, t.Description))
		if len(t.Parameters) > 0 {
			b.WriteString(fmt.Sprintf("Parameters: %s\n", string(t.Parameters)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// parsePromptToolCall extracts a tool call from a ```tool_call``` code block in the response.
func parsePromptToolCall(content string) *ToolCall {
	// Look for ```tool_call ... ``` blocks
	markers := []string{"```tool_call\n", "```tool_call\r\n", "```json\n"}
	var jsonStr string

	for _, marker := range markers {
		start := strings.Index(content, marker)
		if start == -1 {
			continue
		}
		start += len(marker)
		end := strings.Index(content[start:], "```")
		if end == -1 {
			continue
		}
		jsonStr = strings.TrimSpace(content[start : start+end])
		break
	}

	// Also try bare JSON objects at the start/end
	if jsonStr == "" {
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"tool"`) {
			// Find the JSON object
			depth := 0
			for i, c := range trimmed {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						jsonStr = trimmed[:i+1]
						break
					}
				}
			}
		}
	}

	if jsonStr == "" {
		return nil
	}

	var call struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
		return nil
	}
	if call.Tool == "" {
		return nil
	}

	return &ToolCall{
		ID:        fmt.Sprintf("prompt_%d", len(jsonStr)),
		Name:      call.Tool,
		Arguments: string(call.Arguments),
	}
}

// stripToolCallJSON removes the tool_call JSON block from the visible content.
func stripToolCallJSON(content string) string {
	for _, marker := range []string{"```tool_call\n", "```tool_call\r\n"} {
		start := strings.Index(content, marker)
		if start == -1 {
			continue
		}
		end := strings.Index(content[start+len(marker):], "```")
		if end == -1 {
			continue
		}
		end += start + len(marker) + 3
		content = strings.TrimSpace(content[:start] + content[end:])
	}
	return content
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, req CompletionRequest, ch chan<- StreamChunk) {
	msgs := p.toOpenAIMessages(req.Messages)
	apiReq := p.buildRequest(req, msgs)
	apiReq.Stream = true

	stream, err := p.client.CreateChatCompletionStream(ctx, apiReq)
	if err != nil && !p.betaRestricted && isBetaRestrictionError(err) {
		p.betaRestricted = true
		apiReq = p.buildRequest(req, msgs)
		apiReq.Stream = true
		stream, err = p.client.CreateChatCompletionStream(ctx, apiReq)
	}
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			ch <- StreamChunk{Done: true}
			return
		}
		if err != nil {
			ch <- StreamChunk{Err: err, Done: true}
			return
		}
		if len(resp.Choices) > 0 {
			ch <- StreamChunk{Content: resp.Choices[0].Delta.Content}
		}
	}
}
