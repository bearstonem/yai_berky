package memory

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// serializeFloat32 converts a []float32 to the compact binary format sqlite-vec expects.
func serializeFloat32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// MessageRecord is a conversation message stored in memory.
type MessageRecord struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Distance  float64
	CreatedAt time.Time
}

// SkillRecord is a skill entry stored in memory.
type SkillRecord struct {
	ID          int64
	Name        string
	Description string
	Distance    float64
}

// SessionRecord is a session entry stored in memory.
type SessionRecord struct {
	ID        int64
	SessionID string
	Summary   string
	Mode      string
	Distance  float64
}

// IndexMessage stores a message and its embedding.
func (s *Store) IndexMessage(ctx context.Context, embedder EmbeddingProvider, sessionID, role, content string) error {
	if content == "" {
		return nil
	}

	vec, err := embedder.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embedding message: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)",
		sessionID, role, content,
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	id, _ := res.LastInsertId()

	_, err = tx.Exec(
		"INSERT INTO vec_messages (rowid, embedding) VALUES (?, ?)",
		id, serializeFloat32(vec),
	)
	if err != nil {
		return fmt.Errorf("inserting message embedding: %w", err)
	}

	return tx.Commit()
}

// IndexSkill stores a skill description and its embedding.
func (s *Store) IndexSkill(ctx context.Context, embedder EmbeddingProvider, name, description string) error {
	// Upsert: remove old entry if it exists
	var oldID int64
	err := s.db.QueryRow("SELECT id FROM skills WHERE name = ?", name).Scan(&oldID)
	if err == nil {
		s.db.Exec("DELETE FROM vec_skills WHERE rowid = ?", oldID)
		s.db.Exec("DELETE FROM skills WHERE id = ?", oldID)
	}

	text := fmt.Sprintf("%s: %s", name, description)
	vec, err := embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embedding skill: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO skills (name, description) VALUES (?, ?)",
		name, description,
	)
	if err != nil {
		return fmt.Errorf("inserting skill: %w", err)
	}

	id, _ := res.LastInsertId()

	_, err = tx.Exec(
		"INSERT INTO vec_skills (rowid, embedding) VALUES (?, ?)",
		id, serializeFloat32(vec),
	)
	if err != nil {
		return fmt.Errorf("inserting skill embedding: %w", err)
	}

	return tx.Commit()
}

// IndexSession stores a session summary and its embedding.
func (s *Store) IndexSession(ctx context.Context, embedder EmbeddingProvider, sessionID, summary, mode string) error {
	// Upsert
	var oldID int64
	err := s.db.QueryRow("SELECT id FROM sessions WHERE session_id = ?", sessionID).Scan(&oldID)
	if err == nil {
		s.db.Exec("DELETE FROM vec_sessions WHERE rowid = ?", oldID)
		s.db.Exec("DELETE FROM sessions WHERE id = ?", oldID)
	}

	vec, err := embedder.Embed(ctx, summary)
	if err != nil {
		return fmt.Errorf("embedding session: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO sessions (session_id, summary, mode) VALUES (?, ?, ?)",
		sessionID, summary, mode,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	id, _ := res.LastInsertId()

	_, err = tx.Exec(
		"INSERT INTO vec_sessions (rowid, embedding) VALUES (?, ?)",
		id, serializeFloat32(vec),
	)
	if err != nil {
		return fmt.Errorf("inserting session embedding: %w", err)
	}

	return tx.Commit()
}

// SearchMessages finds the k most similar messages to the query.
func (s *Store) SearchMessages(ctx context.Context, embedder EmbeddingProvider, query string, k int) ([]MessageRecord, error) {
	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT m.id, m.session_id, m.role, m.content, v.distance, m.created_at
		FROM vec_messages v
		LEFT JOIN messages m ON m.id = v.rowid
		WHERE v.embedding MATCH ?
		  AND k = ?
		ORDER BY v.distance
	`, serializeFloat32(vec), k)
	if err != nil {
		return nil, fmt.Errorf("searching messages: %w", err)
	}
	defer rows.Close()

	var results []MessageRecord
	for rows.Next() {
		var r MessageRecord
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Role, &r.Content, &r.Distance, &r.CreatedAt); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// SearchSkills finds the k most similar skills to the query.
func (s *Store) SearchSkills(ctx context.Context, embedder EmbeddingProvider, query string, k int) ([]SkillRecord, error) {
	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT sk.id, sk.name, sk.description, v.distance
		FROM vec_skills v
		LEFT JOIN skills sk ON sk.id = v.rowid
		WHERE v.embedding MATCH ?
		  AND k = ?
		ORDER BY v.distance
	`, serializeFloat32(vec), k)
	if err != nil {
		return nil, fmt.Errorf("searching skills: %w", err)
	}
	defer rows.Close()

	var results []SkillRecord
	for rows.Next() {
		var r SkillRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Distance); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// SearchSessions finds the k most similar sessions to the query.
func (s *Store) SearchSessions(ctx context.Context, embedder EmbeddingProvider, query string, k int) ([]SessionRecord, error) {
	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT ss.id, ss.session_id, ss.summary, ss.mode, v.distance
		FROM vec_sessions v
		LEFT JOIN sessions ss ON ss.id = v.rowid
		WHERE v.embedding MATCH ?
		  AND k = ?
		ORDER BY v.distance
	`, serializeFloat32(vec), k)
	if err != nil {
		return nil, fmt.Errorf("searching sessions: %w", err)
	}
	defer rows.Close()

	var results []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Summary, &r.Mode, &r.Distance); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// Stats returns counts of indexed items.
func (s *Store) Stats() (messages, skills, sessions int) {
	s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messages)
	s.db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&skills)
	s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions)
	return
}

// RemoveSkill deletes a skill from the memory index.
func (s *Store) RemoveSkill(name string) error {
	var id int64
	err := s.db.QueryRow("SELECT id FROM skills WHERE name = ?", name).Scan(&id)
	if err != nil {
		return nil // not indexed
	}
	s.db.Exec("DELETE FROM vec_skills WHERE rowid = ?", id)
	s.db.Exec("DELETE FROM skills WHERE id = ?", id)
	return nil
}
