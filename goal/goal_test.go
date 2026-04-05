package goal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".config", "helm", "goals"), 0755)
	return dir
}

func TestSanitizeID(t *testing.T) {
	assert.Equal(t, "expand_skills", sanitizeID("Expand Skills"))
	assert.Equal(t, "goal-1", sanitizeID("Goal-1"))
	assert.Equal(t, "", sanitizeID("   "))
}

func TestSaveAndLoad(t *testing.T) {
	dir := setupTestDir(t)

	g := &Goal{
		Title:       "Learn APIs",
		Description: "Create skills for common APIs",
		Priority:    1,
	}

	err := Save(dir, g)
	require.NoError(t, err)
	assert.Equal(t, "learn_apis", g.ID)
	assert.Equal(t, "active", g.Status)
	assert.False(t, g.CreatedAt.IsZero())

	loaded, err := Load(dir, "learn_apis")
	require.NoError(t, err)
	assert.Equal(t, "Learn APIs", loaded.Title)
	assert.Equal(t, 1, loaded.Priority)
}

func TestLoadAll(t *testing.T) {
	dir := setupTestDir(t)

	Save(dir, &Goal{Title: "Goal A", Description: "first"})
	Save(dir, &Goal{Title: "Goal B", Description: "second"})

	goals, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Len(t, goals, 2)
}

func TestLoadAllEmpty(t *testing.T) {
	dir := setupTestDir(t)
	goals, err := LoadAll(dir)
	require.NoError(t, err)
	assert.Nil(t, goals)
}

func TestDelete(t *testing.T) {
	dir := setupTestDir(t)
	Save(dir, &Goal{Title: "Temp Goal"})

	err := Delete(dir, "temp_goal")
	require.NoError(t, err)

	_, err = Load(dir, "temp_goal")
	assert.Error(t, err)
}

func TestProgressAppend(t *testing.T) {
	dir := setupTestDir(t)

	g := &Goal{Title: "Track Progress", Progress: "step 1"}
	Save(dir, g)

	loaded, _ := Load(dir, "track_progress")
	loaded.Progress += "\nstep 2"
	Save(dir, loaded)

	reloaded, _ := Load(dir, "track_progress")
	assert.Contains(t, reloaded.Progress, "step 1")
	assert.Contains(t, reloaded.Progress, "step 2")
}

func TestDefaultStatus(t *testing.T) {
	dir := setupTestDir(t)
	g := &Goal{Title: "No Status Set"}
	Save(dir, g)

	loaded, _ := Load(dir, "no_status_set")
	assert.Equal(t, "active", loaded.Status)
}
