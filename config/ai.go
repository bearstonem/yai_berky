package config

const (
	// New provider-agnostic keys
	ai_provider    = "AI_PROVIDER"
	ai_api_key     = "AI_API_KEY"
	ai_model       = "AI_MODEL"
	ai_base_url    = "AI_BASE_URL"
	ai_proxy       = "AI_PROXY"
	ai_temperature = "AI_TEMPERATURE"
	ai_max_tokens  = "AI_MAX_TOKENS"

	// Legacy OpenAI keys (for backward compatibility)
	openai_key         = "OPENAI_KEY"
	openai_model       = "OPENAI_MODEL"
	openai_proxy       = "OPENAI_PROXY"
	openai_temperature = "OPENAI_TEMPERATURE"
	openai_max_tokens  = "OPENAI_MAX_TOKENS"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderOpenRouter = "openrouter"
	ProviderMiniMax   = "minimax"
	ProviderOllama    = "ollama"
	ProviderLlamaCpp  = "llamacpp"
	ProviderLMStudio  = "lmstudio"
	ProviderCustom    = "custom"
)

var ProviderBaseURLs = map[string]string{
	ProviderOpenAI:     "", // default, handled by go-openai library
	ProviderAnthropic:  "https://api.anthropic.com",
	ProviderOpenRouter: "https://openrouter.ai/api/v1",
	ProviderMiniMax:    "https://api.minimax.io/v1",
	ProviderOllama:     "http://localhost:11434/v1",
	ProviderLlamaCpp:   "http://localhost:8080/v1",
	ProviderLMStudio:   "http://localhost:1234/v1",
}

var ProviderDefaultModels = map[string]string{
	ProviderOpenAI:     "gpt-4o-mini",
	ProviderAnthropic:  "claude-sonnet-4-6",
	ProviderOpenRouter: "openai/gpt-4o-mini",
	ProviderMiniMax:    "MiniMax-M2.7",
	ProviderOllama:     "llama3.2",
	ProviderLlamaCpp:   "default",
	ProviderLMStudio:   "default",
	ProviderCustom:     "default",
}

var ProviderDisplayNames = map[string]string{
	ProviderOpenAI:     "OpenAI",
	ProviderAnthropic:  "Anthropic Claude",
	ProviderOpenRouter: "OpenRouter",
	ProviderMiniMax:    "MiniMax",
	ProviderOllama:     "Ollama (local)",
	ProviderLlamaCpp:   "llama.cpp (local)",
	ProviderLMStudio:   "LM Studio (local)",
	ProviderCustom:     "Custom (OpenAI-compatible)",
}

func ProviderNeedsAPIKey(provider string) bool {
	switch provider {
	case ProviderOllama, ProviderLlamaCpp, ProviderLMStudio:
		return false
	default:
		return true
	}
}

func ProviderList() []string {
	return []string{
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderOpenRouter,
		ProviderMiniMax,
		ProviderOllama,
		ProviderLlamaCpp,
		ProviderLMStudio,
		ProviderCustom,
	}
}

type AiConfig struct {
	provider    string
	key         string
	model       string
	baseURL     string
	proxy       string
	temperature float64
	maxTokens   int
}

func (c AiConfig) GetProvider() string {
	return c.provider
}

func (c AiConfig) GetKey() string {
	return c.key
}

func (c AiConfig) GetModel() string {
	return c.model
}

func (c AiConfig) GetBaseURL() string {
	return c.baseURL
}

func (c AiConfig) GetProxy() string {
	return c.proxy
}

func (c AiConfig) GetTemperature() float64 {
	return c.temperature
}

func (c AiConfig) GetMaxTokens() int {
	return c.maxTokens
}

func (c AiConfig) GetEffectiveBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	if url, ok := ProviderBaseURLs[c.provider]; ok {
		return url
	}
	return ""
}
