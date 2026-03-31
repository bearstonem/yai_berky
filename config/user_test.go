package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserConfig(t *testing.T) {
	t.Run("GetDefaultPromptMode", testGetDefaultPromptMode)
	t.Run("GetPreferences", testGetPreferences)
	t.Run("GetAllowSudo", testGetAllowSudo)
}

func testGetDefaultPromptMode(t *testing.T) {
	expectedDefaultPromptMode := "test_mode"
	userConfig := UserConfig{defaultPromptMode: expectedDefaultPromptMode}

	actualDefaultPromptMode := userConfig.GetDefaultPromptMode()

	assert.Equal(t, expectedDefaultPromptMode, actualDefaultPromptMode, "The two default prompt modes should be the same.")
}

func testGetPreferences(t *testing.T) {
	expectedPreferences := "test_preferences"
	userConfig := UserConfig{preferences: expectedPreferences}

	actualPreferences := userConfig.GetPreferences()

	assert.Equal(t, expectedPreferences, actualPreferences, "The two preferences should be the same.")
}

func testGetAllowSudo(t *testing.T) {
	userConfig := UserConfig{allowSudo: true}
	assert.True(t, userConfig.GetAllowSudo(), "Allow sudo should be true.")

	userConfig = UserConfig{allowSudo: false}
	assert.False(t, userConfig.GetAllowSudo(), "Allow sudo should be false.")
}
