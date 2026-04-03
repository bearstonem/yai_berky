package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OpenAIEmbedder uses OpenAI's text-embedding-3-small model.
type OpenAIEmbedder struct {
	client *openai.Client
}

func NewOpenAIEmbedder(apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		client: openai.NewClient(apiKey),
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Truncate very long text to avoid token limits
	if len(text) > 8000 {
		text = text[:8000]
	}

	resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input:      []string{text},
		Model:      openai.SmallEmbedding3,
		Dimensions: embeddingDims,
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embedding: empty response")
	}

	return resp.Data[0].Embedding, nil
}

// OllamaEmbedder uses a local Ollama instance for embeddings.
type OllamaEmbedder struct {
	baseURL string
	model   string
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbedder{baseURL: baseURL, model: model}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if len(text) > 8000 {
		text = text[:8000]
	}

	payload := fmt.Sprintf(`{"model":%q,"input":%q}`, e.model, text)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embed", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama embed read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama embed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ollama embed parse: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}

	vec := result.Embeddings[0]

	// Truncate or pad to match expected dimensions
	if len(vec) > embeddingDims {
		vec = vec[:embeddingDims]
	} else if len(vec) < embeddingDims {
		padded := make([]float32, embeddingDims)
		copy(padded, vec)
		vec = padded
	}

	return vec, nil
}
