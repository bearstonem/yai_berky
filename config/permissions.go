package config

const (
	user_permission_mode = "USER_PERMISSION_MODE"
)

// PermissionMode controls what tools the agent is allowed to use.
type PermissionMode int

const (
	// PermReadOnly — agent can only read files, list directories, search.
	PermReadOnly PermissionMode = iota
	// PermWorkspaceWrite — agent can read/write files within the workspace.
	PermWorkspaceWrite
	// PermFullAccess — agent can run any command, modify any file, use sudo.
	PermFullAccess
)

func (p PermissionMode) String() string {
	switch p {
	case PermReadOnly:
		return "read-only"
	case PermWorkspaceWrite:
		return "workspace-write"
	case PermFullAccess:
		return "full-access"
	default:
		return "unknown"
	}
}

func PermissionModeFromString(s string) PermissionMode {
	switch s {
	case "read-only", "readonly":
		return PermReadOnly
	case "workspace-write", "write":
		return PermWorkspaceWrite
	case "full-access", "full", "danger":
		return PermFullAccess
	default:
		return PermWorkspaceWrite // safe default
	}
}

// ToolPermission declares the minimum permission required to use a tool.
type ToolPermission int

const (
	ToolPermRead  ToolPermission = iota // read_file, list_directory, search_files, find_files
	ToolPermWrite                       // write_file, edit_file
	ToolPermExec                        // run_command
)

// ToolPermissions maps tool names to their required permission level.
var ToolPermissions = map[string]ToolPermission{
	"read_file":      ToolPermRead,
	"list_directory":  ToolPermRead,
	"search_files":    ToolPermRead,
	"find_files":      ToolPermRead,
	"write_file":      ToolPermWrite,
	"edit_file":       ToolPermWrite,
	"run_command":      ToolPermExec,
}

// IsToolAllowed checks if a tool is permitted under the given mode.
func IsToolAllowed(toolName string, mode PermissionMode) bool {
	perm, ok := ToolPermissions[toolName]
	if !ok {
		// Integration tools are allowed in workspace-write mode.
		if len(toolName) > 12 && toolName[:12] == "integration_" {
			return mode >= PermWorkspaceWrite
		}
		// Skill tools are allowed in workspace-write mode.
		if len(toolName) > 6 && toolName[:6] == "skill_" {
			return mode >= PermWorkspaceWrite
		}
		// Agent-as-tool calls are allowed in workspace-write mode.
		if len(toolName) > 6 && toolName[:6] == "agent_" {
			return mode >= PermWorkspaceWrite
		}
		// Skill management tools
		if toolName == "create_skill" || toolName == "list_skills" || toolName == "remove_skill" {
			return mode >= PermWorkspaceWrite
		}
		// Agent management, delegation, and escalation tools
		if toolName == "create_agent" || toolName == "delegate_task" || toolName == "escalate_to_user" {
			return mode >= PermWorkspaceWrite
		}
		// Goal management tools
		if toolName == "list_goals" || toolName == "create_goal" || toolName == "update_goal" {
			return mode >= PermWorkspaceWrite
		}
		// Restart requires full access (it kills the process)
		if toolName == "restart_helm" {
			return mode >= PermFullAccess
		}
		// Unknown tools require full access.
		return mode >= PermFullAccess
	}

	switch perm {
	case ToolPermRead:
		return true // always allowed
	case ToolPermWrite:
		return mode >= PermWorkspaceWrite
	case ToolPermExec:
		return mode >= PermWorkspaceWrite // write mode allows commands within workspace
	default:
		return mode >= PermFullAccess
	}
}
