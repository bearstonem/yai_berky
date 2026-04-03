package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

const embeddingDims = 512

// Store provides persistent vector-backed memory for conversations, skills, and sessions.
type Store struct {
	db *sql.DB
}

func init() {
	vec.Auto()
}

// Open opens (or creates) the memory database at ~/.config/yai/memory.db.
func Open(homeDir string) (*Store, error) {
	dir := filepath.Join(homeDir, ".config", "yai")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating memory dir: %w", err)
	}

	dbPath := filepath.Join(dir, "memory.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening memory db: %w", err)
	}

	// Verify sqlite-vec is loaded
	var vecVersion string
	if err := db.QueryRow("SELECT vec_version()").Scan(&vecVersion); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite-vec not available: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating memory db: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) migrate() error {
	migrations := []string{
		// Metadata table for messages
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Metadata table for skills
		`CREATE TABLE IF NOT EXISTS skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Metadata table for sessions
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL UNIQUE,
			summary TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Vector tables
		fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_messages USING vec0(
			embedding float[%d] distance_metric=cosine
		)`, embeddingDims),
		fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_skills USING vec0(
			embedding float[%d] distance_metric=cosine
		)`, embeddingDims),
		fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_sessions USING vec0(
			embedding float[%d] distance_metric=cosine
		)`, embeddingDims),
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}
