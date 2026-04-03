package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

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
		Model:               req.Model,
		MaxCompletionTokens: req.MaxTokens,
		Messages:            msgs,
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
