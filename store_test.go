package picobrain

import (
	"database/sql"
	"testing"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitSchema(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// Verify thoughts table exists
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='thoughts'").Scan(&name)
	if err != nil {
		t.Fatalf("thoughts table not found: %v", err)
	}

	// Verify thought_vectors virtual table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='thought_vectors'").Scan(&name)
	if err != nil {
		t.Fatalf("thought_vectors table not found: %v", err)
	}
}

func TestInitSchemaIdempotent(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("first initSchema: %v", err)
	}
	if err := initSchema(db); err != nil {
		t.Fatalf("second initSchema should not fail: %v", err)
	}
}

func TestInsertAndGetThought(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	thought := &Thought{
		ID:          "test-id-1",
		Content:     "Met with Sarah about the consulting transition",
		Embedding:   make([]float32, 768),
		People:      []string{"Sarah"},
		Topics:      []string{"career", "consulting"},
		Type:        "meeting",
		ActionItems: []string{"Follow up next week"},
		Source:      "slack",
		CreatedAt:   now,
	}
	// Set a recognizable pattern in the embedding
	thought.Embedding[0] = 0.5
	thought.Embedding[1] = 0.3

	if err := insertThought(db, thought); err != nil {
		t.Fatalf("insertThought: %v", err)
	}

	got, err := getThought(db, "test-id-1")
	if err != nil {
		t.Fatalf("getThought: %v", err)
	}

	if got.ID != thought.ID {
		t.Errorf("ID: expected %s, got %s", thought.ID, got.ID)
	}
	if got.Content != thought.Content {
		t.Errorf("Content: expected %s, got %s", thought.Content, got.Content)
	}
	if len(got.People) != 1 || got.People[0] != "Sarah" {
		t.Errorf("People: expected [Sarah], got %v", got.People)
	}
	if len(got.Topics) != 2 {
		t.Errorf("Topics: expected 2, got %d", len(got.Topics))
	}
	if got.Type != "meeting" {
		t.Errorf("Type: expected meeting, got %s", got.Type)
	}
	if len(got.ActionItems) != 1 || got.ActionItems[0] != "Follow up next week" {
		t.Errorf("ActionItems: expected [Follow up next week], got %v", got.ActionItems)
	}
	if got.Source != "slack" {
		t.Errorf("Source: expected slack, got %s", got.Source)
	}
}

func TestGetThoughtNotFound(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	_, err := getThought(db, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent thought")
	}
}
