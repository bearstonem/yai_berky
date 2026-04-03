package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
)

type AnthropicProvider struct {
	apiKey    string
	client    *http.Client
	lastUsage Usage
}

func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) LastUsage() Usage {
	return p.lastUsage
}

func (p *AnthropicProvider) recordUsage(resp *anthropicResponse) {
	if resp.Usage != nil {
		p.lastUsage = Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		}
	}
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

func (p *AnthropicProvider) buildRequest(req CompletionRequest) (string, []anthropicMessage) {
	var system string
	msgs := make([]anthropicMessage, 0, len(req.Messages))

	for _, m := range req.Messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}

		if m.Role == "tool" {
			msgs = append(msgs, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
			continue
		}

		if len(m.ToolCalls) > 0 {
			blocks := make([]anthropicContentBlock, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Arguments),
				})
			}
			msgs = append(msgs, anthropicMessage{Role: m.Role, Content: blocks})
			continue
		}

		msgs = append(msgs, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	if len(msgs) == 0 {
		msgs = append(msgs, anthropicMessage{Role: "user", Content: "Hello"})
	}

	return system, msgs
}

func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	system, msgs := p.buildRequest(req)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if result.Error != nil {
		return "", fmt.Errorf("anthropic API error: %s", result.Error.Message)
	}

	p.recordUsage(&result)

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in anthropic response")
	}

	return result.Content[0].Text, nil
}

func (p *AnthropicProvider) CompleteWithTools(ctx context.Context, req CompletionRequest) (Message, error) {
	system, msgs := p.buildRequest(req)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
	}

	if len(req.Tools) > 0 {
		body.Tools = make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			body.Tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return Message{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("anthropic API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return Message{}, err
	}

	if result.Error != nil {
		return Message{}, fmt.Errorf("anthropic API error: %s", result.Error.Message)
	}

	p.recordUsage(&result)

	msg := Message{Role: "assistant"}
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			msg.Content += block.Text
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		}
	}

	return msg, nil
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, req CompletionRequest, ch chan<- StreamChunk) {
	system, msgs := p.buildRequest(req)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		ch <- StreamChunk{
			Err:  fmt.Errorf("anthropic API error (HTTP %d): %s", resp.StatusCode, string(respBody)),
			Done: true,
		}
		return
	}

	scanner := newSSEScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			ch <- StreamChunk{Done: true}
			return
		}

		var event struct {
			Type  string `json:"type"`
			Delta *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta,omitempty"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				ch <- StreamChunk{Content: event.Delta.Text}
			}
		case "message_stop":
			ch <- StreamChunk{Done: true}
			return
		case "error":
			ch <- StreamChunk{
				Err:  fmt.Errorf("anthropic stream error: %s", data),
				Done: true,
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Err: err, Done: true}
		return
	}

	ch <- StreamChunk{Done: true}
}

type sseScanner struct {
	reader *strings.Reader
	buf    *bytes.Buffer
	rawBuf []byte
	r      io.Reader
	line   string
	err    error
}

func newSSEScanner(r io.Reader) *sseScanner {
	return &sseScanner{
		r:      r,
		rawBuf: make([]byte, 4096),
		buf:    &bytes.Buffer{},
	}
}

func (s *sseScanner) Scan() bool {
	for {
		if idx := bytes.IndexByte(s.buf.Bytes(), '\n'); idx >= 0 {
			line := string(s.buf.Next(idx))
			s.buf.ReadByte() // consume newline
			s.line = strings.TrimRight(line, "\r")
			return true
		}

		n, err := s.r.Read(s.rawBuf)
		if n > 0 {
			s.buf.Write(s.rawBuf[:n])
		}
		if err != nil {
			if s.buf.Len() > 0 {
				s.line = strings.TrimRight(s.buf.String(), "\r\n")
				s.buf.Reset()
				return true
			}
			if err != io.EOF {
				s.err = err
			}
			return false
		}
	}
}

func (s *sseScanner) Text() string {
	return s.line
}

func (s *sseScanner) Err() error {
	return s.err
}
