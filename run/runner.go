package run

import (
	"fmt"
	"os/exec"
	"strings"
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
