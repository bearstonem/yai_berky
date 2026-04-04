package config

import (
	"os"
	"strings"
	"testing"

	"github.com/bearstonem/helm/system"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Run("NewConfig", testNewConfig)
	t.Run("NewConfigLegacy", testNewConfigLegacy)
	t.Run("WriteConfig", testWriteConfig)
}

func setupViper(t *testing.T) {
	t.Helper()
	viper.Reset()

	sys := system.Analyse()
	viper.SetConfigName(strings.ToLower(sys.GetApplicationName()))
	viper.AddConfigPath("/tmp/")

	viper.Set(ai_provider, ProviderOpenAI)
	viper.Set(ai_api_key, "test_key")
	viper.Set(ai_model, "gpt-4o-mini")
	viper.Set(ai_proxy, "test_proxy")
	viper.Set(ai_temperature, 0.2)
	viper.Set(ai_max_tokens, 2000)
	viper.Set(user_default_prompt_mode, "exec")
	viper.Set(user_preferences, "test_preferences")

	require.NoError(t, viper.SafeWriteConfigAs("/tmp/helm.json"))
}

func setupViperLegacy(t *testing.T) {
	t.Helper()
	viper.Reset()

	sys := system.Analyse()
	viper.SetConfigName(strings.ToLower(sys.GetApplicationName()))
	viper.AddConfigPath("/tmp/")

	viper.Set(openai_key, "legacy_key")
	viper.Set(openai_model, "gpt-3.5-turbo")
	viper.Set(openai_proxy, "legacy_proxy")
	viper.Set(openai_temperature, 0.5)
	viper.Set(openai_max_tokens, 1000)
	viper.Set(user_default_prompt_mode, "exec")
	viper.Set(user_preferences, "legacy_prefs")

	require.NoError(t, viper.SafeWriteConfigAs("/tmp/helm.json"))
}

func cleanup(t *testing.T) {
	t.Helper()
	os.Remove("/tmp/helm.json")
	viper.Reset()
}

func testNewConfig(t *testing.T) {
	setupViper(t)
	defer cleanup(t)

	cfg, err := NewConfig()
	require.NoError(t, err)

	assert.Equal(t, ProviderOpenAI, cfg.GetAiConfig().GetProvider())
	assert.Equal(t, "test_key", cfg.GetAiConfig().GetKey())
	assert.Equal(t, "gpt-4o-mini", cfg.GetAiConfig().GetModel())
	assert.Equal(t, "test_proxy", cfg.GetAiConfig().GetProxy())
	assert.Equal(t, 0.2, cfg.GetAiConfig().GetTemperature())
	assert.Equal(t, 2000, cfg.GetAiConfig().GetMaxTokens())
	assert.Equal(t, "exec", cfg.GetUserConfig().GetDefaultPromptMode())
	assert.Equal(t, "test_preferences", cfg.GetUserConfig().GetPreferences())
	assert.NotNil(t, cfg.GetSystemConfig())
}

func testNewConfigLegacy(t *testing.T) {
	setupViperLegacy(t)
	defer cleanup(t)

	cfg, err := NewConfig()
	require.NoError(t, err)

	assert.Equal(t, ProviderOpenAI, cfg.GetAiConfig().GetProvider())
	assert.Equal(t, "legacy_key", cfg.GetAiConfig().GetKey())
	assert.Equal(t, "gpt-3.5-turbo", cfg.GetAiConfig().GetModel())
	assert.Equal(t, "legacy_proxy", cfg.GetAiConfig().GetProxy())
	assert.Equal(t, 0.5, cfg.GetAiConfig().GetTemperature())
	assert.Equal(t, 1000, cfg.GetAiConfig().GetMaxTokens())
}

func testWriteConfig(t *testing.T) {
	setupViper(t)
	defer cleanup(t)

	cfg, err := WriteConfig(ProviderAnthropic, "new_test_key", "claude-sonnet-4-6", "", false)
	require.NoError(t, err)

	assert.Equal(t, ProviderAnthropic, cfg.GetAiConfig().GetProvider())
	assert.Equal(t, "new_test_key", cfg.GetAiConfig().GetKey())
	assert.Equal(t, "claude-sonnet-4-6", cfg.GetAiConfig().GetModel())
	assert.Equal(t, "exec", cfg.GetUserConfig().GetDefaultPromptMode())
	assert.NotNil(t, cfg.GetSystemConfig())
}
