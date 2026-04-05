package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest tracks all backups and points to the latest.
type Manifest struct {
	Latest  string        `json:"latest"`  // ID of most recent backup
	Backups []BackupEntry `json:"backups"`
}

// BackupEntry describes a single backup.
type BackupEntry struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"` // e.g. "self-improvement cycle 3"
}

// BackupsDir returns the backup storage directory.
func BackupsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "backups")
}

func manifestPath(homeDir string) string {
	return filepath.Join(BackupsDir(homeDir), "manifest.json")
}

// LoadManifest reads the backup manifest.
func LoadManifest(homeDir string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath(homeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return &Manifest{}, nil
	}
	return &m, nil
}

// SaveManifest writes the manifest to disk.
func SaveManifest(homeDir string, m *Manifest) error {
	dir := BackupsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(homeDir), data, 0644)
}

// Create backs up the entire application source to a timestamped subfolder.
// sourceDir is the root of the Go project (where go.mod lives).
// Returns the backup entry.
func Create(homeDir, sourceDir, reason string) (*BackupEntry, error) {
	id := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(BackupsDir(homeDir), id)

	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	// Exclude: .git, vendor, node_modules, build artifacts
	excludes := map[string]bool{
		".git": true, "vendor": true, "node_modules": true,
		"helm": true, "yai": true, "__pycache__": true, ".cache": true,
	}

	// Use rsync if available (faster), otherwise pure Go copy
	if _, err := exec.LookPath("rsync"); err == nil {
		args := []string{"-a", "--delete"}
		for e := range excludes {
			args = append(args, "--exclude", e)
		}
		args = append(args, sourceDir+"/", backupPath+"/")
		cmd := exec.Command("rsync", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("backup copy failed: %w\n%s", err, string(output))
		}
	} else {
		if err := copyDir(sourceDir, backupPath, excludes); err != nil {
			return nil, fmt.Errorf("backup copy failed: %w", err)
		}
	}

	entry := &BackupEntry{
		ID:        id,
		Path:      backupPath,
		CreatedAt: time.Now(),
		Reason:    reason,
	}

	// Update manifest
	manifest, _ := LoadManifest(homeDir)
	manifest.Latest = id
	manifest.Backups = append(manifest.Backups, *entry)

	// Keep only last 10 backups
	if len(manifest.Backups) > 10 {
		for _, old := range manifest.Backups[:len(manifest.Backups)-10] {
			os.RemoveAll(old.Path)
		}
		manifest.Backups = manifest.Backups[len(manifest.Backups)-10:]
	}

	if err := SaveManifest(homeDir, manifest); err != nil {
		return entry, fmt.Errorf("saved backup but failed to update manifest: %w", err)
	}

	return entry, nil
}

// Restore copies the latest backup over the source directory.
func Restore(homeDir, sourceDir string) error {
	manifest, err := LoadManifest(homeDir)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if manifest.Latest == "" || len(manifest.Backups) == 0 {
		return fmt.Errorf("no backups available")
	}

	// Find latest backup
	var latest *BackupEntry
	for i := range manifest.Backups {
		if manifest.Backups[i].ID == manifest.Latest {
			latest = &manifest.Backups[i]
			break
		}
	}
	if latest == nil {
		return fmt.Errorf("latest backup %q not found", manifest.Latest)
	}

	if _, err := os.Stat(latest.Path); os.IsNotExist(err) {
		return fmt.Errorf("backup path %q does not exist", latest.Path)
	}

	// Sync backup over source, preserving .git
	excludes := map[string]bool{".git": true}
	if _, err := exec.LookPath("rsync"); err == nil {
		cmd := exec.Command("rsync", "-a", "--delete", "--exclude", ".git",
			latest.Path+"/", sourceDir+"/")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore failed: %w\n%s", err, string(output))
		}
	} else {
		// Remove source contents (except .git) then copy
		entries, _ := os.ReadDir(sourceDir)
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			os.RemoveAll(filepath.Join(sourceDir, e.Name()))
		}
		if err := copyDir(latest.Path, sourceDir, excludes); err != nil {
			return fmt.Errorf("restore copy failed: %w", err)
		}
	}

	return nil
}

// LatestBackup returns info about the most recent backup.
func LatestBackup(homeDir string) (*BackupEntry, error) {
	manifest, err := LoadManifest(homeDir)
	if err != nil {
		return nil, err
	}
	if len(manifest.Backups) == 0 {
		return nil, nil
	}
	sort.Slice(manifest.Backups, func(i, j int) bool {
		return manifest.Backups[i].CreatedAt.After(manifest.Backups[j].CreatedAt)
	})
	return &manifest.Backups[0], nil
}

// copyDir recursively copies src to dst, skipping entries in excludes.
func copyDir(src, dst string, excludes map[string]bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Check if any path component is excluded
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for _, p := range parts {
			if excludes[p] {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// GenerateRestartScript creates a shell script that rebuilds and restarts helm.
// If the build fails, it restores from the latest backup and tries again.
func GenerateRestartScript(homeDir, sourceDir, binaryPath string, guiPort int) (string, error) {
	scriptPath := filepath.Join(BackupsDir(homeDir), "restart.sh")

	script := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -e",
		"",
		"SOURCE_DIR=\"" + sourceDir + "\"",
		"BINARY=\"" + binaryPath + "\"",
		"HOME_DIR=\"" + homeDir + "\"",
		"GUI_PORT=" + fmt.Sprintf("%d", guiPort),
		"MANIFEST=\"" + manifestPath(homeDir) + "\"",
		"PID_FILE=\"" + filepath.Join(BackupsDir(homeDir), "helm.pid") + "\"",
		"",
		"echo \"[restart] Stopping current helm instance...\"",
		"if [ -f \"$PID_FILE\" ]; then",
		"    kill $(cat \"$PID_FILE\") 2>/dev/null || true",
		"    sleep 2",
		"fi",
		"",
		"echo \"[restart] Building from source...\"",
		"cd \"$SOURCE_DIR\"",
		"if CGO_ENABLED=1 go build -o \"$BINARY\" . 2>/tmp/helm-build-error.log; then",
		"    echo \"[restart] Build succeeded. Launching...\"",
		"    nohup \"$BINARY\" --gui --port $GUI_PORT > /tmp/helm.log 2>&1 &",
		"    echo $! > \"$PID_FILE\"",
		"    echo \"[restart] Helm restarted (PID: $(cat $PID_FILE))\"",
		"else",
		"    echo \"[restart] Build FAILED. Restoring from backup...\"",
		"    cat /tmp/helm-build-error.log",
		"    ",
		"    # Get latest backup path from manifest",
		"    LATEST_ID=$(python3 -c \"import json; m=json.load(open('$MANIFEST')); print(m['latest'])\" 2>/dev/null)",
		"    BACKUP_DIR=\"" + BackupsDir(homeDir) + "/$LATEST_ID\"",
		"    ",
		"    if [ -d \"$BACKUP_DIR\" ]; then",
		"        echo \"[restart] Restoring from backup: $LATEST_ID\"",
		"        rsync -a --delete --exclude .git \"$BACKUP_DIR/\" \"$SOURCE_DIR/\" 2>/dev/null || cp -r \"$BACKUP_DIR/.\" \"$SOURCE_DIR/\"",
		"        echo \"[restart] Rebuilding from backup...\"",
		"        cd \"$SOURCE_DIR\"",
		"        if CGO_ENABLED=1 go build -o \"$BINARY\" . 2>/tmp/helm-restore-error.log; then",
		"            echo \"[restart] Restored build succeeded. Launching...\"",
		"            nohup \"$BINARY\" --gui --port $GUI_PORT > /tmp/helm.log 2>&1 &",
		"            echo $! > \"$PID_FILE\"",
		"            echo \"[restart] Helm restored and restarted (PID: $(cat $PID_FILE))\"",
		"        else",
		"            echo \"[restart] CRITICAL: Restore build also failed!\"",
		"            cat /tmp/helm-restore-error.log",
		"            exit 1",
		"        fi",
		"    else",
		"        echo \"[restart] No backup found to restore from!\"",
		"        exit 1",
		"    fi",
		"fi",
	}, "\n")

	dir := BackupsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}
