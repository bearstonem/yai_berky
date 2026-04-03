package config

import (
	"fmt"
	"strings"

	"github.com/ekkinox/yai/system"
	"github.com/spf13/viper"
)

type Config struct {
	ai     AiConfig
	user   UserConfig
	system *system.Analysis
}

func (c *Config) GetAiConfig() AiConfig {
	return c.ai
}

func (c *Config) GetUserConfig() UserConfig {
	return c.user
}

func (c *Config) GetSystemConfig() *system.Analysis {
	return c.system
}

func NewConfig() (*Config, error) {
	sys := system.Analyse()

	viper.SetConfigName(strings.ToLower(sys.GetApplicationName()))
	viper.AddConfigPath(fmt.Sprintf("%s/.config/", sys.GetHomeDirectory()))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	// Layer 2: project-level config (.yai/settings.json in workspace root)
	// Layer 3: local config (.yai/settings.local.json — gitignored overrides)
	workRoot := sys.GetWorkspaceRoot()
	if workRoot == "" {
		workRoot = sys.GetCurrentDirectory()
	}
	if workRoot != "" {
		overlayConfigFile(workRoot, ".yai", "settings")
		overlayConfigFile(workRoot, ".yai", "settings.local")
	}

	provider := viper.GetString(ai_provider)
	key := viper.GetString(ai_api_key)
	model := viper.GetString(ai_model)
	baseURL := viper.GetString(ai_base_url)
	proxy := viper.GetString(ai_proxy)
	temperature := viper.GetFloat64(ai_temperature)
	maxTokens := viper.GetInt(ai_max_tokens)

	// Backward compatibility: fall back to legacy OPENAI_* keys
	if provider == "" {
		provider = ProviderOpenAI
	}
	if key == "" {
		key = viper.GetString(openai_key)
	}
	if model == "" {
		model = viper.GetString(openai_model)
	}
	if proxy == "" {
		proxy = viper.GetString(openai_proxy)
	}
	if temperature == 0 {
		legacyTemp := viper.GetFloat64(openai_temperature)
		if legacyTemp != 0 {
			temperature = legacyTemp
		}
	}
	if maxTokens == 0 {
		legacyTokens := viper.GetInt(openai_max_tokens)
		if legacyTokens != 0 {
			maxTokens = legacyTokens
		}
	}

	if model == "" {
		if defaultModel, ok := ProviderDefaultModels[provider]; ok {
			model = defaultModel
		}
	}

	return &Config{
		ai: AiConfig{
			provider:    provider,
			key:         key,
			model:       model,
			baseURL:     baseURL,
			proxy:       proxy,
			temperature: temperature,
			maxTokens:   maxTokens,
		},
		user: UserConfig{
			defaultPromptMode: viper.GetString(user_default_prompt_mode),
			preferences:       viper.GetString(user_preferences),
			allowSudo:         viper.GetBool(user_allow_sudo),
			agentAutoExecute:  viper.GetBool(user_agent_auto_execute),
			permissionMode:    PermissionModeFromString(viper.GetString(user_permission_mode)),
			hooks:             LoadHooksFromViper(),
		},
		system: sys,
	}, nil
}

// overlayConfigFile merges a JSON config file on top of the current viper state.
// It uses a temporary viper instance to read the file, then sets any non-zero
// values into the main viper. This allows project/local configs to selectively
// override user-level settings.
func overlayConfigFile(baseDir, subDir, name string) {
	path := fmt.Sprintf("%s/%s/%s.json", baseDir, subDir, name)

	overlay := viper.New()
	overlay.SetConfigFile(path)
	if err := overlay.ReadInConfig(); err != nil {
		return // file doesn't exist or can't be read — skip silently
	}

	for _, key := range overlay.AllKeys() {
		viper.Set(key, overlay.Get(key))
	}
}

func WriteConfig(provider, key, model, baseURL string, write bool) (*Config, error) {
	sys := system.Analyse()

	viper.Set(ai_provider, provider)
	viper.Set(ai_api_key, key)

	if model == "" {
		if defaultModel, ok := ProviderDefaultModels[provider]; ok {
			model = defaultModel
		}
	}
	viper.Set(ai_model, model)

	if baseURL == "" {
		if defaultURL, ok := ProviderBaseURLs[provider]; ok {
			baseURL = defaultURL
		}
	}
	viper.Set(ai_base_url, baseURL)

	viper.SetDefault(ai_proxy, "")
	viper.SetDefault(ai_temperature, 0.2)
	viper.SetDefault(ai_max_tokens, 2000)

	viper.SetDefault(user_default_prompt_mode, "exec")
	viper.SetDefault(user_preferences, "")
	viper.SetDefault(user_allow_sudo, false)
	viper.SetDefault(user_agent_auto_execute, false)

	if write {
		err := viper.SafeWriteConfigAs(sys.GetConfigFile())
		if err != nil {
			return nil, err
		}
	}

	return NewConfig()
}
