package system

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	InstructionFileName = "YAI.md"
	maxPerFile          = 4096  // 4KB per file
	maxTotal            = 12288 // 12KB total
)

// DiscoverInstructions walks up the directory tree from startDir to the
// filesystem root, collecting YAI.md files. Files closer to the root are
// returned first (broadest context first), so project-level instructions
// appear before subdirectory-specific ones.
func DiscoverInstructions(startDir string) string {
	if startDir == "" {
		return ""
	}

	// Collect paths from startDir upward.
	var paths []string
	dir := startDir
	for {
		candidate := filepath.Join(dir, InstructionFileName)
		if _, err := os.Stat(candidate); err == nil {
			paths = append(paths, candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached root
		}
		dir = parent
	}

	if len(paths) == 0 {
		return ""
	}

	// Reverse so root-level files come first.
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}

	var parts []string
	totalLen := 0

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		if len(content) > maxPerFile {
			content = content[:maxPerFile] + "\n... [truncated]"
		}

		if totalLen+len(content) > maxTotal {
			remaining := maxTotal - totalLen
			if remaining > 100 {
				content = content[:remaining] + "\n... [truncated]"
			} else {
				break
			}
		}

		parts = append(parts, content)
		totalLen += len(content)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n---\n\n")
}
