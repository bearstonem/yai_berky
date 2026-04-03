package skill

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const maxOutputBytes = 50 * 1024

// Execute runs a skill's script with the given JSON arguments passed via stdin.
func Execute(homeDir string, m Manifest, argsJSON string) (string, error) {
	scriptPath := ScriptPath(homeDir, m)

	interpreter := interpreterForLanguage(m.Language)
	cmd := exec.Command(interpreter, scriptPath)
	cmd.Stdin = strings.NewReader(argsJSON)
	cmd.Dir = SkillsDir(homeDir)

	done := make(chan error, 1)
	var out []byte
	var cmdErr error

	go func() {
		out, cmdErr = cmd.CombinedOutput()
		done <- cmdErr
	}()

	select {
	case <-done:
		// completed
	case <-time.After(120 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return "", fmt.Errorf("skill %q timed out after 120s", m.Name)
	}

	output := string(out)
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... [output truncated]"
	}

	if cmdErr != nil {
		exitErr, ok := cmdErr.(*exec.ExitError)
		if ok {
			return fmt.Sprintf("%s\nexit_code: %d", output, exitErr.ExitCode()), nil
		}
		return "", fmt.Errorf("running skill %q: %w", m.Name, cmdErr)
	}

	return output, nil
}

func interpreterForLanguage(lang string) string {
	switch strings.ToLower(lang) {
	case "python", "python3", "py":
		return "python3"
	case "node", "nodejs", "javascript", "js":
		return "node"
	case "ruby", "rb":
		return "ruby"
	case "bash", "sh", "shell", "":
		return "bash"
	default:
		return "bash"
	}
}
