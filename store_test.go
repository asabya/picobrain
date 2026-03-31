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

func seedThoughts(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	thoughts := []Thought{
		{
			ID:      "t1",
			Content: "Sarah is considering leaving her job for consulting",
			People:  []string{"Sarah"},
			Topics:  []string{"career", "consulting"},
			Type:    "person_note",
			Source:  "slack",
		},
		{
			ID:      "t2",
			Content: "Decided to switch the API from REST to GraphQL",
			People:  []string{},
			Topics:  []string{"engineering", "api"},
			Type:    "decision",
			Source:  "claude",
		},
		{
			ID:      "t3",
			Content: "Weekly team standup went well, shipping v2 next week",
			People:  []string{"team"},
			Topics:  []string{"engineering", "shipping"},
			Type:    "meeting",
			Source:  "slack",
		},
	}

	// Give each thought a distinct embedding direction
	for i := range thoughts {
		thoughts[i].Embedding = make([]float32, 768)
		thoughts[i].Embedding[i] = 1.0 // each points in a different axis
		thoughts[i].CreatedAt = time.Now().Add(-time.Duration(len(thoughts)-i) * 24 * time.Hour)
		if err := insertThought(db, &thoughts[i]); err != nil {
			t.Fatalf("seed thought %d: %v", i, err)
		}
	}
}

func TestSearchByVector(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	// Search with a vector pointing in the direction of t1
	query := make([]float32, 768)
	query[0] = 1.0

	results, err := searchByVector(db, query, 2, "")
	if err != nil {
		t.Fatalf("searchByVector: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].ID != "t1" {
		t.Errorf("expected first result to be t1, got %s", results[0].ID)
	}
	if results[0].Distance > results[1].Distance {
		t.Errorf("results should be ordered by ascending distance")
	}
	if results[0].Content == "" {
		t.Errorf("expected content to be populated")
	}
	if len(results[0].People) == 0 {
		t.Errorf("expected people to be populated")
	}
}

func TestSearchByVectorEmpty(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	query := make([]float32, 768)
	query[0] = 1.0

	results, err := searchByVector(db, query, 5, "")
	if err != nil {
		t.Fatalf("searchByVector on empty db: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty db, got %d", len(results))
	}
}

func TestListRecent(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	// List all thoughts from the last week
	since := time.Now().Add(-7 * 24 * time.Hour)
	results, err := listRecent(db, since, 10, "")
	if err != nil {
		t.Fatalf("listRecent: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Should be newest first
	if results[0].ID != "t3" {
		t.Errorf("expected newest first (t3), got %s", results[0].ID)
	}
}

func TestListRecentWithLimit(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	since := time.Now().Add(-7 * 24 * time.Hour)
	results, err := listRecent(db, since, 1, "")
	if err != nil {
		t.Fatalf("listRecent: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result with limit=1, got %d", len(results))
	}
}

func TestGetStats(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	stats, err := getStats(db)
	if err != nil {
		t.Fatalf("getStats: %v", err)
	}

	if stats.TotalThoughts != 3 {
		t.Errorf("TotalThoughts: expected 3, got %d", stats.TotalThoughts)
	}
	if stats.ThoughtsThisWeek != 3 {
		t.Errorf("ThoughtsThisWeek: expected 3, got %d", stats.ThoughtsThisWeek)
	}
	if len(stats.TopTopics) == 0 {
		t.Errorf("TopTopics should not be empty")
	}
	// "engineering" appears in 2 thoughts, should be first
	if stats.TopTopics[0] != "engineering" {
		t.Errorf("TopTopics[0]: expected 'engineering', got '%s'", stats.TopTopics[0])
	}
	if len(stats.TopSources) == 0 {
		t.Errorf("TopSources should not be empty")
	}
	if stats.FirstThought.IsZero() {
		t.Errorf("FirstThought should not be zero")
	}
	if stats.LastThought.IsZero() {
		t.Errorf("LastThought should not be zero")
	}
}

func TestGetStatsEmpty(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	stats, err := getStats(db)
	if err != nil {
		t.Fatalf("getStats on empty db: %v", err)
	}
	if stats.TotalThoughts != 0 {
		t.Errorf("expected 0 total on empty db, got %d", stats.TotalThoughts)
	}
}

func TestDeleteThought(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	err := deleteThought(db, "t1")
	if err != nil {
		t.Fatalf("deleteThought: %v", err)
	}

	_, err = getThought(db, "t1")
	if err == nil {
		t.Fatal("expected error after delete, thought still exists")
	}

	// Verify other thoughts still exist
	got, err := getThought(db, "t2")
	if err != nil {
		t.Fatalf("t2 should still exist: %v", err)
	}
	if got.ID != "t2" {
		t.Errorf("expected t2, got %s", got.ID)
	}
}

func TestDeleteThoughtNotFound(t *testing.T) {
	db := testDB(t)
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	err := deleteThought(db, "nonexistent")
	if err != nil {
		t.Fatalf("deleteThought of nonexistent should not error: %v", err)
	}
}

func TestListRecentWithTypeFilter(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	// Add an observation-type thought
	obs := &Thought{
		ID:        "obs1",
		Content:   "User discussed API design patterns",
		Type:      "observation",
		Source:    "agent",
		Embedding: make([]float32, 768),
		CreatedAt: time.Now(),
	}
	if err := insertThought(db, obs); err != nil {
		t.Fatalf("insert observation: %v", err)
	}

	since := time.Now().Add(-7 * 24 * time.Hour)

	// Filter for observations only
	results, err := listRecent(db, since, 10, "observation")
	if err != nil {
		t.Fatalf("listRecent with type filter: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(results))
	}
	if results[0].ID != "obs1" {
		t.Errorf("expected obs1, got %s", results[0].ID)
	}

	// No filter returns all
	all, err := listRecent(db, since, 10, "")
	if err != nil {
		t.Fatalf("listRecent no filter: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 total, got %d", len(all))
	}
}

func TestReflectStore(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	newThoughts := []*Thought{
		{
			ID:        "new1",
			Content:   "Consolidated: Sarah career + API design decisions",
			Type:      "observation",
			Source:    "agent",
			Embedding: make([]float32, 768),
			CreatedAt: time.Now(),
		},
	}

	err := reflectTx(db, []string{"t1", "t2"}, newThoughts)
	if err != nil {
		t.Fatalf("reflectTx: %v", err)
	}

	// Old thoughts should be gone
	_, err = getThought(db, "t1")
	if err == nil {
		t.Error("t1 should be deleted after reflect")
	}
	_, err = getThought(db, "t2")
	if err == nil {
		t.Error("t2 should be deleted after reflect")
	}

	// t3 should still exist
	got, err := getThought(db, "t3")
	if err != nil {
		t.Fatalf("t3 should still exist: %v", err)
	}
	if got.ID != "t3" {
		t.Errorf("expected t3, got %s", got.ID)
	}

	// New thought should exist
	got, err = getThought(db, "new1")
	if err != nil {
		t.Fatalf("new1 should exist: %v", err)
	}
	if got.Content != newThoughts[0].Content {
		t.Errorf("content mismatch")
	}
}

func TestSearchByVectorWithTypeFilter(t *testing.T) {
	db := testDB(t)
	seedThoughts(t, db)

	obs := &Thought{
		ID:        "obs1",
		Content:   "API design discussion observations",
		Type:      "observation",
		Source:    "agent",
		Embedding: make([]float32, 768),
		CreatedAt: time.Now(),
	}
	obs.Embedding[3] = 1.0
	if err := insertThought(db, obs); err != nil {
		t.Fatalf("insert observation: %v", err)
	}

	query := make([]float32, 768)
	query[3] = 1.0

	// Filter for observations only
	results, err := searchByVector(db, query, 10, "observation")
	if err != nil {
		t.Fatalf("searchByVector with type filter: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 observation result, got %d", len(results))
	}
	if results[0].ID != "obs1" {
		t.Errorf("expected obs1, got %s", results[0].ID)
	}
}
