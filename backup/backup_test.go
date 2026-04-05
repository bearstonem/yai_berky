package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDirs(t *testing.T) (homeDir, sourceDir string) {
	homeDir = t.TempDir()
	sourceDir = t.TempDir()

	// Create some source files to back up
	os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(sourceDir, "go.mod"), []byte("module test"), 0644)
	os.MkdirAll(filepath.Join(sourceDir, "pkg"), 0755)
	os.WriteFile(filepath.Join(sourceDir, "pkg", "lib.go"), []byte("package pkg"), 0644)

	return homeDir, sourceDir
}

func TestCreateBackup(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	entry, err := Create(homeDir, sourceDir, "test backup")
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "test backup", entry.Reason)
	assert.DirExists(t, entry.Path)

	// Verify files were copied
	assert.FileExists(t, filepath.Join(entry.Path, "main.go"))
	assert.FileExists(t, filepath.Join(entry.Path, "go.mod"))
	assert.FileExists(t, filepath.Join(entry.Path, "pkg", "lib.go"))
}

func TestManifest(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	entry1, _ := Create(homeDir, sourceDir, "first")
	entry2, _ := Create(homeDir, sourceDir, "second")

	manifest, err := LoadManifest(homeDir)
	require.NoError(t, err)
	assert.Equal(t, entry2.ID, manifest.Latest)
	assert.Len(t, manifest.Backups, 2)
	assert.Equal(t, entry1.ID, manifest.Backups[0].ID)
	assert.Equal(t, entry2.ID, manifest.Backups[1].ID)
}

func TestManifestEmpty(t *testing.T) {
	homeDir := t.TempDir()
	manifest, err := LoadManifest(homeDir)
	require.NoError(t, err)
	assert.Empty(t, manifest.Latest)
	assert.Empty(t, manifest.Backups)
}

func TestRestore(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	// Create backup
	Create(homeDir, sourceDir, "before change")

	// Modify source
	os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("MODIFIED"), 0644)
	os.WriteFile(filepath.Join(sourceDir, "new_file.go"), []byte("NEW"), 0644)

	// Verify modification
	data, _ := os.ReadFile(filepath.Join(sourceDir, "main.go"))
	assert.Equal(t, "MODIFIED", string(data))

	// Restore
	err := Restore(homeDir, sourceDir)
	require.NoError(t, err)

	// Verify restored
	data, _ = os.ReadFile(filepath.Join(sourceDir, "main.go"))
	assert.Equal(t, "package main", string(data))
}

func TestRestoreNoBackup(t *testing.T) {
	homeDir := t.TempDir()
	err := Restore(homeDir, t.TempDir())
	assert.Error(t, err)
}

func TestLatestBackup(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	Create(homeDir, sourceDir, "first")
	Create(homeDir, sourceDir, "second")

	latest, err := LatestBackup(homeDir)
	require.NoError(t, err)
	assert.NotNil(t, latest)
	assert.Equal(t, "second", latest.Reason)
}

func TestLatestBackupEmpty(t *testing.T) {
	homeDir := t.TempDir()
	latest, err := LatestBackup(homeDir)
	require.NoError(t, err)
	assert.Nil(t, latest)
}

func TestBackupPruning(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	// Create 12 backups
	for i := 0; i < 12; i++ {
		Create(homeDir, sourceDir, "backup")
	}

	manifest, _ := LoadManifest(homeDir)
	assert.Len(t, manifest.Backups, 10) // keeps only last 10
}

func TestGenerateRestartScript(t *testing.T) {
	homeDir := t.TempDir()
	sourceDir := t.TempDir()

	path, err := GenerateRestartScript(homeDir, sourceDir, "/usr/local/bin/helm", 6900)
	require.NoError(t, err)
	assert.FileExists(t, path)

	data, _ := os.ReadFile(path)
	script := string(data)
	assert.Contains(t, script, "#!/usr/bin/env bash")
	assert.Contains(t, script, sourceDir)
	assert.Contains(t, script, "go build")
	assert.Contains(t, script, "6900")
}

func TestExcludesGitDir(t *testing.T) {
	homeDir, sourceDir := setupTestDirs(t)

	// Create a .git dir in source
	os.MkdirAll(filepath.Join(sourceDir, ".git", "objects"), 0755)
	os.WriteFile(filepath.Join(sourceDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

	entry, _ := Create(homeDir, sourceDir, "test")

	// .git should NOT be in the backup
	assert.NoDirExists(t, filepath.Join(entry.Path, ".git"))
	// But source files should be
	assert.FileExists(t, filepath.Join(entry.Path, "main.go"))
}
