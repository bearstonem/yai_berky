package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAiConfig(t *testing.T) {
	t.Run("GetProvider", testGetProvider)
	t.Run("GetKey", testGetKey)
	t.Run("GetModel", testGetModel)
	t.Run("GetBaseURL", testGetBaseURL)
	t.Run("GetProxy", testGetProxy)
	t.Run("GetTemperature", testGetTemperature)
	t.Run("GetMaxTokens", testGetMaxTokens)
	t.Run("GetEffectiveBaseURL", testGetEffectiveBaseURL)
	t.Run("ProviderNeedsAPIKey", testProviderNeedsAPIKey)
	t.Run("ProviderList", testProviderList)
}

func testGetProvider(t *testing.T) {
	aiConfig := AiConfig{provider: ProviderAnthropic}
	assert.Equal(t, ProviderAnthropic, aiConfig.GetProvider())
}

func testGetKey(t *testing.T) {
	aiConfig := AiConfig{key: "test_key"}
	assert.Equal(t, "test_key", aiConfig.GetKey())
}

func testGetModel(t *testing.T) {
	aiConfig := AiConfig{model: "gpt-4o"}
	assert.Equal(t, "gpt-4o", aiConfig.GetModel())
}

func testGetBaseURL(t *testing.T) {
	aiConfig := AiConfig{baseURL: "https://custom.api.com/v1"}
	assert.Equal(t, "https://custom.api.com/v1", aiConfig.GetBaseURL())
}

func testGetProxy(t *testing.T) {
	aiConfig := AiConfig{proxy: "test_proxy"}
	assert.Equal(t, "test_proxy", aiConfig.GetProxy())
}

func testGetTemperature(t *testing.T) {
	aiConfig := AiConfig{temperature: 0.7}
	assert.Equal(t, 0.7, aiConfig.GetTemperature())
}

func testGetMaxTokens(t *testing.T) {
	aiConfig := AiConfig{maxTokens: 2000}
	assert.Equal(t, 2000, aiConfig.GetMaxTokens())
}

func testGetEffectiveBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		config   AiConfig
		expected string
	}{
		{
			name:     "explicit base URL takes priority",
			config:   AiConfig{provider: ProviderOpenAI, baseURL: "https://custom.com/v1"},
			expected: "https://custom.com/v1",
		},
		{
			name:     "ollama default URL",
			config:   AiConfig{provider: ProviderOllama},
			expected: "http://localhost:11434/v1",
		},
		{
			name:     "openrouter default URL",
			config:   AiConfig{provider: ProviderOpenRouter},
			expected: "https://openrouter.ai/api/v1",
		},
		{
			name:     "openai default URL is empty",
			config:   AiConfig{provider: ProviderOpenAI},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetEffectiveBaseURL())
		})
	}
}

func testProviderNeedsAPIKey(t *testing.T) {
	assert.True(t, ProviderNeedsAPIKey(ProviderOpenAI))
	assert.True(t, ProviderNeedsAPIKey(ProviderAnthropic))
	assert.True(t, ProviderNeedsAPIKey(ProviderOpenRouter))
	assert.False(t, ProviderNeedsAPIKey(ProviderOllama))
	assert.False(t, ProviderNeedsAPIKey(ProviderLlamaCpp))
	assert.False(t, ProviderNeedsAPIKey(ProviderLMStudio))
}

func testProviderList(t *testing.T) {
	providers := ProviderList()
	assert.Len(t, providers, 8)
	assert.Contains(t, providers, ProviderOpenAI)
	assert.Contains(t, providers, ProviderAnthropic)
	assert.Contains(t, providers, ProviderOllama)
}
