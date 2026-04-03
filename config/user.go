package config

const (
	user_default_prompt_mode    = "USER_DEFAULT_PROMPT_MODE"
	user_preferences            = "USER_PREFERENCES"
	user_allow_sudo             = "USER_ALLOW_SUDO"
	user_agent_auto_execute     = "USER_AGENT_AUTO_EXECUTE"
)

type UserConfig struct {
	defaultPromptMode string
	preferences       string
	allowSudo         bool
	agentAutoExecute  bool
	permissionMode    PermissionMode
	hooks             []HookConfig
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

func (c UserConfig) GetAgentAutoExecute() bool {
	return c.agentAutoExecute
}

func (c UserConfig) GetPermissionMode() PermissionMode {
	return c.permissionMode
}

func (c UserConfig) GetHooks() []HookConfig {
	return c.hooks
}
