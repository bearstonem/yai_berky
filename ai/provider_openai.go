package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/sashabaranov/go-openai"
)

type OpenAIProvider struct {
	client *openai.Client
	name   string
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

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Messages:    msgs,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no choices returned from API")
	}

	return resp.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, req CompletionRequest, ch chan<- StreamChunk) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Messages:    msgs,
		Stream:      true,
	})
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
