package run

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("RunCommand", testRunCommand)
	t.Run("PrepareInteractiveCommand", testPrepareInteractiveCommand)
	t.Run("PrepareSudoInteractiveCommand", testPrepareSudoInteractiveCommand)
	t.Run("PrepareEditSettingsCommand", testPrepareEditSettingsCommand)
	t.Run("CommandContainsSudo", testCommandContainsSudo)
}

func testRunCommand(t *testing.T) {
	output, err := RunCommand("echo", "Hello, World!")
	require.NoError(t, err)

	assert.Equal(t, "Hello, World!\n", output, "The command output should be the same.")
}

func testPrepareInteractiveCommand(t *testing.T) {
	cmd := PrepareInteractiveCommand("echo 'Hello, World!'")

	expectedCmd := exec.Command(
		"bash",
		"-c",
		"echo \"\n\";echo 'Hello, World!'; echo \"\n\";",
	)

	assert.Equal(t, expectedCmd.Args, cmd.Args, "The command arguments should be the same.")
}

func testPrepareSudoInteractiveCommand(t *testing.T) {
	cmd := PrepareSudoInteractiveCommand("sudo apt update")

	expectedCmd := exec.Command(
		"bash",
		"-c",
		"sudo -v && echo \"\n\" && sudo apt update; echo \"\n\";",
	)

	assert.Equal(t, expectedCmd.Args, cmd.Args, "The command arguments should be the same.")
}

func testPrepareEditSettingsCommand(t *testing.T) {
	cmd := PrepareEditSettingsCommand("nano yo.json")

	expectedCmd := exec.Command(
		"bash",
		"-c",
		"nano yo.json; echo \"\n\";",
	)

	assert.Equal(t, expectedCmd.Args, cmd.Args, "The command arguments should be the same.")
}

func testCommandContainsSudo(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		{"simple sudo", "sudo apt update", true},
		{"sudo with path", "sudo /usr/bin/systemctl restart nginx", true},
		{"no sudo", "ls -la", false},
		{"sudo in pipeline", "echo test | sudo tee /etc/config", true},
		{"sudo after &&", "cd /tmp && sudo make install", true},
		{"sudo after ;", "echo hello; sudo reboot", true},
		{"sudo-like word", "pseudocode", false},
		{"empty command", "", false},
		{"just sudo", "sudo", true},
		{"sudo with leading space", "  sudo apt install vim", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CommandContainsSudo(tt.cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}
