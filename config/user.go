package config

const (
	user_default_prompt_mode = "USER_DEFAULT_PROMPT_MODE"
	user_preferences         = "USER_PREFERENCES"
	user_allow_sudo          = "USER_ALLOW_SUDO"
)

type UserConfig struct {
	defaultPromptMode string
	preferences       string
	allowSudo         bool
}

func (c UserConfig) GetDefaultPromptMode() string {
	return c.defaultPromptMode
}

func (c UserConfig) GetPreferences() string {
	return c.preferences
}

func (c UserConfig) GetAllowSudo() bool {
	return c.allowSudo
}
