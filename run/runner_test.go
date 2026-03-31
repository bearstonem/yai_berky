package run

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("RunCommand", testRunCommand)
	t.Run("PrepareInteractiveCommand", testPrepareInteractiveCommand)
	t.Run("PrepareSudoInteractiveCommand", testPrepareSudoInteractiveCommand)
	t.Run("PrepareEditSettingsCommand", testPrepareEditSettingsCommand)
	t.Run("CommandContainsSudo", testCommandContainsSudo)
	t.Run("CaptureCommand", testCaptureCommand)
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

func testCaptureCommand(t *testing.T) {
	t.Run("captures stdout", func(t *testing.T) {
		output, err := CaptureCommand("echo hello", "", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Equal(t, "hello\n", output.Stdout)
		assert.Empty(t, output.Stderr)
	})

	t.Run("captures stderr", func(t *testing.T) {
		output, err := CaptureCommand("echo error >&2", "", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Empty(t, output.Stdout)
		assert.Equal(t, "error\n", output.Stderr)
	})

	t.Run("captures exit code", func(t *testing.T) {
		output, err := CaptureCommand("exit 42", "", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 42, output.ExitCode)
	})

	t.Run("timeout", func(t *testing.T) {
		output, err := CaptureCommand("sleep 10", "", 1*time.Second)
		require.NoError(t, err)
		assert.Equal(t, -1, output.ExitCode)
		assert.Contains(t, output.Stderr, "timed out")
	})

	t.Run("working directory", func(t *testing.T) {
		output, err := CaptureCommand("pwd", "/tmp", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Contains(t, output.Stdout, "/tmp")
	})

	t.Run("default timeout used when zero", func(t *testing.T) {
		output, err := CaptureCommand("echo ok", "", 0)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Equal(t, "ok\n", output.Stdout)
	})
}
