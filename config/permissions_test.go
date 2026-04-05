package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermissionModeString(t *testing.T) {
	assert.Equal(t, "read-only", PermReadOnly.String())
	assert.Equal(t, "workspace-write", PermWorkspaceWrite.String())
	assert.Equal(t, "full-access", PermFullAccess.String())
}

func TestPermissionModeFromString(t *testing.T) {
	assert.Equal(t, PermReadOnly, PermissionModeFromString("read-only"))
	assert.Equal(t, PermReadOnly, PermissionModeFromString("readonly"))
	assert.Equal(t, PermWorkspaceWrite, PermissionModeFromString("workspace-write"))
	assert.Equal(t, PermWorkspaceWrite, PermissionModeFromString("write"))
	assert.Equal(t, PermFullAccess, PermissionModeFromString("full-access"))
	assert.Equal(t, PermFullAccess, PermissionModeFromString("danger"))
	assert.Equal(t, PermWorkspaceWrite, PermissionModeFromString("unknown"))
}

func TestIsToolAllowed_ReadOnly(t *testing.T) {
	mode := PermReadOnly

	// Read tools always allowed
	assert.True(t, IsToolAllowed("web_search", mode))
	assert.True(t, IsToolAllowed("read_file", mode))
	assert.True(t, IsToolAllowed("list_directory", mode))
	assert.True(t, IsToolAllowed("search_files", mode))
	assert.True(t, IsToolAllowed("find_files", mode))

	// Write tools denied
	assert.False(t, IsToolAllowed("write_file", mode))
	assert.False(t, IsToolAllowed("edit_file", mode))
	assert.False(t, IsToolAllowed("run_command", mode))

	// Agent tools denied
	assert.False(t, IsToolAllowed("create_skill", mode))
	assert.False(t, IsToolAllowed("create_agent", mode))
	assert.False(t, IsToolAllowed("delegate_task", mode))
	assert.False(t, IsToolAllowed("escalate_to_user", mode))
	assert.False(t, IsToolAllowed("skill_test", mode))
	assert.False(t, IsToolAllowed("agent_test", mode))
	assert.False(t, IsToolAllowed("restart_helm", mode))
}

func TestIsToolAllowed_WorkspaceWrite(t *testing.T) {
	mode := PermWorkspaceWrite

	// Read tools
	assert.True(t, IsToolAllowed("web_search", mode))
	assert.True(t, IsToolAllowed("read_file", mode))

	// Write tools
	assert.True(t, IsToolAllowed("write_file", mode))
	assert.True(t, IsToolAllowed("edit_file", mode))
	assert.True(t, IsToolAllowed("run_command", mode))

	// Agent management
	assert.True(t, IsToolAllowed("create_skill", mode))
	assert.True(t, IsToolAllowed("list_skills", mode))
	assert.True(t, IsToolAllowed("remove_skill", mode))
	assert.True(t, IsToolAllowed("create_agent", mode))
	assert.True(t, IsToolAllowed("delegate_task", mode))
	assert.True(t, IsToolAllowed("escalate_to_user", mode))

	// Goal tools
	assert.True(t, IsToolAllowed("list_goals", mode))
	assert.True(t, IsToolAllowed("create_goal", mode))
	assert.True(t, IsToolAllowed("update_goal", mode))

	// Skill/agent prefix tools
	assert.True(t, IsToolAllowed("skill_weather", mode))
	assert.True(t, IsToolAllowed("agent_reviewer", mode))

	// Restart requires full access
	assert.False(t, IsToolAllowed("restart_helm", mode))
}

func TestIsToolAllowed_FullAccess(t *testing.T) {
	mode := PermFullAccess

	assert.True(t, IsToolAllowed("restart_helm", mode))
	assert.True(t, IsToolAllowed("run_command", mode))
	assert.True(t, IsToolAllowed("anything_unknown", mode))
}
