package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".config", "helm", "skills"), 0755)
	return dir
}

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "hello_world", sanitizeName("Hello World"))
	assert.Equal(t, "test123", sanitizeName("test123"))
	assert.Equal(t, "abc", sanitizeName("a!@#b$c"))
	assert.Equal(t, "", sanitizeName("   "))
}

func TestExtensionForLanguage(t *testing.T) {
	assert.Equal(t, ".sh", extensionForLanguage("bash"))
	assert.Equal(t, ".sh", extensionForLanguage("sh"))
	assert.Equal(t, ".py", extensionForLanguage("python"))
	assert.Equal(t, ".py", extensionForLanguage("python3"))
	assert.Equal(t, ".js", extensionForLanguage("node"))
	assert.Equal(t, ".js", extensionForLanguage("javascript"))
	assert.Equal(t, ".rb", extensionForLanguage("ruby"))
	assert.Equal(t, ".sh", extensionForLanguage("unknown"))
}

func TestCreateAndLoadAll(t *testing.T) {
	dir := setupTestDir(t)

	m, err := Create(dir, "greet", "Says hello", "bash", "#!/bin/bash\necho hello", json.RawMessage(`{"type":"object","properties":{}}`))
	require.NoError(t, err)
	assert.Equal(t, "greet", m.Name)
	assert.Equal(t, "script.sh", m.ScriptFile)
	assert.Equal(t, "skill_greet", m.ToolName())

	skills, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "greet", skills[0].Name)
}

func TestCreateInvalidName(t *testing.T) {
	dir := setupTestDir(t)
	_, err := Create(dir, "!!!", "bad", "bash", "echo x", nil)
	assert.Error(t, err)
}

func TestReadScript(t *testing.T) {
	dir := setupTestDir(t)
	Create(dir, "reader", "Reads", "python", "print('hi')", nil)

	skills, _ := LoadAll(dir)
	content, err := ReadScript(dir, skills[0])
	require.NoError(t, err)
	assert.Equal(t, "print('hi')", content)
}

func TestUpdate(t *testing.T) {
	dir := setupTestDir(t)
	Create(dir, "updatable", "v1", "bash", "echo v1", nil)

	m, err := Update(dir, "updatable", "v2", "bash", "echo v2", nil)
	require.NoError(t, err)
	assert.Equal(t, "v2", m.Description)

	skills, _ := LoadAll(dir)
	content, _ := ReadScript(dir, skills[0])
	assert.Equal(t, "echo v2", content)
}

func TestUpdateLanguageChange(t *testing.T) {
	dir := setupTestDir(t)
	Create(dir, "lang_change", "test", "bash", "echo hi", nil)

	_, err := Update(dir, "lang_change", "test", "python", "print('hi')", nil)
	require.NoError(t, err)

	skills, _ := LoadAll(dir)
	assert.Equal(t, "python", skills[0].Language)
	assert.Equal(t, "script.py", skills[0].ScriptFile)
}

func TestRemove(t *testing.T) {
	dir := setupTestDir(t)
	Create(dir, "temp", "temp", "bash", "echo x", nil)

	err := Remove(dir, "temp")
	require.NoError(t, err)

	skills, _ := LoadAll(dir)
	assert.Len(t, skills, 0)
}

func TestRemoveNotFound(t *testing.T) {
	dir := setupTestDir(t)
	err := Remove(dir, "nonexistent")
	assert.Error(t, err)
}

func TestFixParameters(t *testing.T) {
	// Empty → default
	result := fixParameters(nil)
	assert.True(t, json.Valid(result))

	// Already valid object
	valid := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	result = fixParameters(valid)
	assert.Equal(t, valid, result)

	// Double-encoded string
	doubleEncoded := json.RawMessage(`"{\"type\":\"object\"}"`)
	result = fixParameters(doubleEncoded)
	assert.JSONEq(t, `{"type":"object"}`, string(result))

	// Invalid JSON → default
	result = fixParameters(json.RawMessage(`not json`))
	assert.True(t, json.Valid(result))
}

func TestToolName(t *testing.T) {
	m := Manifest{Name: "My Cool Skill"}
	assert.Equal(t, "skill_my_cool_skill", m.ToolName())
}

func TestLoadAllEmpty(t *testing.T) {
	dir := setupTestDir(t)
	skills, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Nil(t, skills)
}
