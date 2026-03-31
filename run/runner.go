package run

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func RunCommand(cmd string, arg ...string) (string, error) {
	out, err := exec.Command(cmd, arg...).Output()
	if err != nil {
		return fmt.Sprintf("error: %v", err), err
	}

	return string(out), nil
}

func PrepareInteractiveCommand(input string) *exec.Cmd {
	return exec.Command(
		"bash",
		"-c",
		fmt.Sprintf("echo \"\n\";%s; echo \"\n\";", strings.TrimRight(input, ";")),
	)
}

func PrepareSudoInteractiveCommand(input string) *exec.Cmd {
	return exec.Command(
		"bash",
		"-c",
		fmt.Sprintf("sudo -v && echo \"\n\" && %s; echo \"\n\";", strings.TrimRight(input, ";")),
	)
}

func PrepareEditSettingsCommand(input string) *exec.Cmd {
	return exec.Command(
		"bash",
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

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

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

func truncateOutput(s string) string {
	if len(s) <= MaxOutputBytes {
		return s
	}
	return s[:MaxOutputBytes] + "\n... [output truncated at 50KB]"
}

func CommandContainsSudo(cmd string) bool {
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
