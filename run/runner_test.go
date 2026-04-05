package run

import (
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("RunCommand", testRunCommand)
	t.Run("PrepareInteractiveCommand", testPrepareInteractiveCommand)
	t.Run("CommandContainsSudo", testCommandContainsSudo)
	t.Run("CaptureCommand", testCaptureCommand)
	t.Run("CaptureSSHCommand", testCaptureSSHCommand)
	t.Run("CaptureSSHCommandWithStdin", testCaptureSSHCommandWithStdin)
}

func testRunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		output, err := RunCommand("cmd", "/C", "echo Hello, World!")
		require.NoError(t, err)
		assert.Contains(t, output, "Hello, World!")
	} else {
		output, err := RunCommand("echo", "Hello, World!")
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!\n", output)
	}
}

func testPrepareInteractiveCommand(t *testing.T) {
	cmd := PrepareInteractiveCommand("echo 'Hello, World!'")
	assert.NotNil(t, cmd)
	assert.True(t, len(cmd.Args) > 0, "Command should have arguments")
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
			if runtime.GOOS == "windows" {
				// sudo is never detected on Windows
				assert.False(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func testCaptureCommand(t *testing.T) {
	t.Run("captures stdout", func(t *testing.T) {
		output, err := CaptureCommand("echo hello", "", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Contains(t, output.Stdout, "hello")
	})

	t.Run("captures stderr", func(t *testing.T) {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "Write-Error 'error' 2>&1 | Out-Null; [Console]::Error.WriteLine('error')"
		} else {
			cmd = "echo error >&2"
		}
		output, err := CaptureCommand(cmd, "", 5*time.Second)
		require.NoError(t, err)
		assert.Contains(t, output.Stderr, "error")
	})

	t.Run("captures exit code", func(t *testing.T) {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "exit 42"
		} else {
			cmd = "exit 42"
		}
		output, err := CaptureCommand(cmd, "", 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 42, output.ExitCode)
	})

	t.Run("timeout", func(t *testing.T) {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "Start-Sleep -Seconds 10"
		} else {
			cmd = "sleep 10"
		}
		output, err := CaptureCommand(cmd, "", 1*time.Second)
		require.NoError(t, err)
		assert.Equal(t, -1, output.ExitCode)
		assert.Contains(t, output.Stderr, "timed out")
	})

	t.Run("working directory", func(t *testing.T) {
		var cmd, expectedDir string
		if runtime.GOOS == "windows" {
			cmd = "Get-Location | Select-Object -ExpandProperty Path"
			expectedDir = "C:\\"
		} else {
			cmd = "pwd"
			expectedDir = "/tmp"
		}
		dir := expectedDir
		if runtime.GOOS == "windows" {
			dir = "C:\\"
		} else {
			dir = "/tmp"
		}
		output, err := CaptureCommand(cmd, dir, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Contains(t, output.Stdout, expectedDir)
	})

	t.Run("default timeout used when zero", func(t *testing.T) {
		output, err := CaptureCommand("echo ok", "", 0)
		require.NoError(t, err)
		assert.Equal(t, 0, output.ExitCode)
		assert.Contains(t, output.Stdout, "ok")
	})
}

func testCaptureSSHCommand(t *testing.T) {
	t.Run("connection timeout on unreachable host", func(t *testing.T) {
		output, err := CaptureSSHCommand("user@192.0.2.1", "echo hello", 5*time.Second)
		if runtime.GOOS == "windows" {
			// ssh may not be available on all Windows systems
			if err != nil {
				t.Skip("ssh not available")
			}
		}
		if err == nil {
			assert.NotEqual(t, 0, output.ExitCode)
		}
	})
}

func testCaptureSSHCommandWithStdin(t *testing.T) {
	t.Run("stdin passed to ssh", func(t *testing.T) {
		stdin := io.NopCloser(strings.NewReader("test content"))
		output, err := CaptureSSHCommandWithStdin("user@192.0.2.1", "cat", stdin, 5*time.Second)
		if runtime.GOOS == "windows" {
			if err != nil {
				t.Skip("ssh not available")
			}
		}
		if err == nil {
			assert.NotEqual(t, 0, output.ExitCode)
		}
	})
}
