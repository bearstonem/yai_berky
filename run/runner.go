package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// shellArgs returns the shell and arguments for executing a command string
// on the current platform.
func shellArgs(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		// Try PowerShell first, fall back to cmd
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", command}
		}
		return "cmd", []string{"/C", command}
	}
	return "bash", []string{"-c", command}
}

func RunCommand(cmd string, arg ...string) (string, error) {
	out, err := exec.Command(cmd, arg...).Output()
	if err != nil {
		return fmt.Sprintf("error: %v", err), err
	}

	return string(out), nil
}

func PrepareInteractiveCommand(input string) *exec.Cmd {
	shell, args := shellArgs(strings.TrimRight(input, ";"))
	if runtime.GOOS == "windows" {
		return exec.Command(shell, args...)
	}
	return exec.Command(
		shell,
		"-c",
		fmt.Sprintf("echo \"\n\";%s; echo \"\n\";", strings.TrimRight(input, ";")),
	)
}

func PrepareSudoInteractiveCommand(input string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// No sudo on Windows — run the command directly
		return PrepareInteractiveCommand(input)
	}
	return exec.Command(
		"bash",
		"-c",
		fmt.Sprintf("sudo -v && echo \"\n\" && %s; echo \"\n\";", strings.TrimRight(input, ";")),
	)
}

func PrepareEditSettingsCommand(input string) *exec.Cmd {
	shell, args := shellArgs(strings.TrimRight(input, ";"))
	if runtime.GOOS == "windows" {
		return exec.Command(shell, args...)
	}
	return exec.Command(
		shell,
		"-c",
		fmt.Sprintf("%s; echo \"\n\";", strings.TrimRight(input, ";")),
	)
}

const (
	DefaultTimeout    = 60 * time.Second
	MaxOutputBytes    = 50 * 1024 // 50KB
)

type CapturedOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func CaptureCommand(command string, workingDir string, timeout time.Duration) (*CapturedOutput, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	shell, args := shellArgs(command)
	cmd := exec.CommandContext(ctx, shell, args...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	return captureExec(ctx, cmd, timeout)
}

func truncateOutput(s string) string {
	if len(s) <= MaxOutputBytes {
		return s
	}
	return s[:MaxOutputBytes] + "\n... [output truncated at 50KB]"
}

var sshBaseArgs = []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}

func CaptureSSHCommand(host, command string, timeout time.Duration) (*CapturedOutput, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := append(sshBaseArgs, host, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)

	return captureExec(ctx, cmd, timeout)
}

func CaptureSSHCommandWithStdin(host, command string, stdin io.Reader, timeout time.Duration) (*CapturedOutput, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := append(sshBaseArgs, host, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = stdin

	return captureExec(ctx, cmd, timeout)
}

func captureExec(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) (*CapturedOutput, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &CapturedOutput{
				Stdout:   truncateOutput(stdout.String()),
				Stderr:   "command timed out after " + timeout.String(),
				ExitCode: -1,
			}, nil
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	return &CapturedOutput{
		Stdout:   truncateOutput(stdout.String()),
		Stderr:   truncateOutput(stderr.String()),
		ExitCode: exitCode,
	}, nil
}

func CommandContainsSudo(cmd string) bool {
	// No sudo on Windows
	if runtime.GOOS == "windows" {
		return false
	}

	trimmed := strings.TrimSpace(cmd)

	if strings.HasPrefix(trimmed, "sudo ") || trimmed == "sudo" {
		return true
	}

	for _, sep := range []string{"&&", "||", ";", "|"} {
		parts := strings.Split(trimmed, sep)
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if strings.HasPrefix(p, "sudo ") || p == "sudo" {
				return true
			}
		}
	}

	return false
}
