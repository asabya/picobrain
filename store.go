package picobrain

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS thoughts (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			people TEXT,
			topics TEXT,
			type TEXT,
			action_items TEXT,
			source TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create thoughts table: %w", err)
	}

	_, err = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS thought_vectors USING vec0(
			id TEXT PRIMARY KEY,
			embedding float[768] distance_metric=cosine
		)
	`)
	if err != nil {
		return fmt.Errorf("create thought_vectors table: %w", err)
	}

	return nil
}

func insertThought(db *sql.DB, t *Thought) error {
	peopleJSON, _ := json.Marshal(t.People)
	topicsJSON, _ := json.Marshal(t.Topics)
	actionItemsJSON, _ := json.Marshal(t.ActionItems)

	createdAt := t.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO thoughts (id, content, people, topics, type, action_items, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Content, string(peopleJSON), string(topicsJSON),
		t.Type, string(actionItemsJSON), t.Source, createdAt)
	if err != nil {
		return fmt.Errorf("insert thought: %w", err)
	}

	vec, err := sqlite_vec.SerializeFloat32(t.Embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO thought_vectors (id, embedding)
		VALUES (?, ?)
	`, t.ID, vec)
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return tx.Commit()
}

func getThought(db *sql.DB, id string) (*Thought, error) {
	var t Thought
	var peopleStr, topicsStr, actionItemsStr sql.NullString
	var createdAt string

	err := db.QueryRow(`
		SELECT id, content, people, topics, type, action_items, source, created_at
		FROM thoughts WHERE id = ?
	`, id).Scan(&t.ID, &t.Content, &peopleStr, &topicsStr,
		&t.Type, &actionItemsStr, &t.Source, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get thought %s: %w", id, err)
	}

	if peopleStr.Valid {
		json.Unmarshal([]byte(peopleStr.String), &t.People)
	}
	if topicsStr.Valid {
		json.Unmarshal([]byte(topicsStr.String), &t.Topics)
	}
	if actionItemsStr.Valid {
		json.Unmarshal([]byte(actionItemsStr.String), &t.ActionItems)
	}

	t.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
	if t.CreatedAt.IsZero() {
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05-07:00", createdAt)
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}

	return &t, nil
}
