package picobrain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockEmbedder is a mock implementation of the Embedder interface for testing.
type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	emb := make([]float32, m.dim)
	if len(text) > 0 {
		emb[int(text[0])%m.dim] = 1.0
	}
	return emb, nil
}

func (m *mockEmbedder) Close() error {
	return nil
}

func testBrain(t *testing.T) *Brain {
	t.Helper()

	cfg := Config{
		DBPath:        ":memory:",
		EmbedModel:    "mock",
		ModelCacheDir: "",
		AutoDownload:  false,
	}

	brain, err := NewWithEmbedder(cfg, &mockEmbedder{dim: 768})
	if err != nil {
		t.Fatalf("New brain: %v", err)
	}
	t.Cleanup(func() { brain.Close() })

	return brain
}

func TestNewBrain(t *testing.T) {
	brain := testBrain(t)
	if brain == nil {
		t.Fatal("expected non-nil brain")
	}
}

func TestBrainStore(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{
		Content: "Sarah is thinking about leaving her job",
		People:  []string{"Sarah"},
		Topics:  []string{"career"},
		Type:    "person_note",
		Source:  "slack",
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if thought.ID == "" {
		t.Error("expected ID to be set after Store")
	}
}

func TestBrainStoreWithExistingID(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{
		ID:      "custom-id",
		Content: "A specific thought",
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if thought.ID != "custom-id" {
		t.Errorf("expected ID to remain 'custom-id', got %s", thought.ID)
	}
}

func TestBrainStoreWithPrecomputedEmbedding(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	emb := make([]float32, 768)
	emb[0] = 1.0

	thought := &Thought{
		Content:   "Pre-embedded thought",
		Embedding: emb,
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Verify the thought was stored (search should find it)
	results, err := brain.Search(ctx, "Pre-embedded thought", 1, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected to find stored thought")
	}
}

func TestBrainSearch(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store a few thoughts
	thoughts := []*Thought{
		{Content: "Alice is working on the frontend redesign", People: []string{"Alice"}, Topics: []string{"frontend"}, Source: "slack"},
		{Content: "Bob fixed the database migration bug", People: []string{"Bob"}, Topics: []string{"backend"}, Source: "claude"},
		{Content: "Carol proposed a new testing strategy", People: []string{"Carol"}, Topics: []string{"testing"}, Source: "cli"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	results, err := brain.Search(ctx, "Alice frontend", 2, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestBrainListRecent(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	err := brain.Store(ctx, &Thought{Content: "thought one", Source: "test"})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	err = brain.Store(ctx, &Thought{Content: "thought two", Source: "test"})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	results, err := brain.ListRecent(ctx, since, 10, "")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestBrainStats(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	brain.Store(ctx, &Thought{Content: "first", Topics: []string{"work"}, Source: "slack"})
	brain.Store(ctx, &Thought{Content: "second", Topics: []string{"work", "ai"}, Source: "claude"})

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 2 {
		t.Errorf("TotalThoughts: expected 2, got %d", stats.TotalThoughts)
	}
}

func TestBrainBulkImport(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	jsonl := `{"content":"imported thought one","people":["Alice"],"topics":["work"],"type":"insight","source":"import"}
{"content":"imported thought two","people":["Bob"],"topics":["ai"],"type":"decision","source":"import"}
{"content":"imported thought three","topics":["life"],"source":"import"}
`

	count, err := brain.BulkImport(ctx, strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("BulkImport: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 imported, got %d", count)
	}

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 3 {
		t.Errorf("expected 3 total after import, got %d", stats.TotalThoughts)
	}
}

func TestBrainBulkImportEmpty(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	count, err := brain.BulkImport(ctx, strings.NewReader(""))
	if err != nil {
		t.Fatalf("BulkImport empty: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 imported from empty input, got %d", count)
	}
}

func TestBrainDelete(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{Content: "to be deleted", Source: "test"}
	if err := brain.Store(ctx, thought); err != nil {
		t.Fatalf("Store: %v", err)
	}

	err := brain.Delete(ctx, thought.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	results, err := brain.Search(ctx, "deleted", 10, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.ID == thought.ID {
			t.Error("deleted thought should not appear in search results")
		}
	}
}

func TestBrainDeleteNonexistent(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	err := brain.Delete(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("Delete nonexistent should not error: %v", err)
	}
}

func TestBrainReflect(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store two observations
	brain.Store(ctx, &Thought{Content: "Obs 1: user discussed auth", Type: "observation", Source: "agent"})
	brain.Store(ctx, &Thought{Content: "Obs 2: user discussed auth flow", Type: "observation", Source: "agent"})

	// Get their IDs
	since := time.Now().Add(-1 * time.Hour)
	obs, _ := brain.ListRecent(ctx, since, 10, "observation")
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(obs))
	}

	// Reflect: consolidate into one
	newThoughts := []*Thought{
		{Content: "Consolidated: user discussed auth flow design", Type: "observation", Source: "agent"},
	}
	ids := []string{obs[0].ID, obs[1].ID}

	result, err := brain.Reflect(ctx, ids, newThoughts)
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}

	if len(result.Stored) != 1 {
		t.Errorf("expected 1 stored, got %d", len(result.Stored))
	}
	if len(result.Deleted) != 2 {
		t.Errorf("expected 2 deleted, got %d", len(result.Deleted))
	}

	// Verify only the consolidated observation exists
	recent, _ := brain.ListRecent(ctx, since, 10, "observation")
	if len(recent) != 1 {
		t.Fatalf("expected 1 observation after reflect, got %d", len(recent))
	}
}

func TestBrainSearchWithTypeFilter(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	brain.Store(ctx, &Thought{Content: "regular thought", Type: "idea", Source: "test"})
	brain.Store(ctx, &Thought{Content: "observation about coding", Type: "observation", Source: "agent"})

	results, err := brain.Search(ctx, "coding", 10, "observation")
	if err != nil {
		t.Fatalf("Search with type: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(results))
	}
	if results[0].Type != "observation" {
		t.Errorf("expected observation type, got %s", results[0].Type)
	}
}

// Namespace tests

func TestBrainStoreWithNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{
		Content:   "Alice's project update",
		People:    []string{"Alice"},
		Topics:    []string{"project"},
		Type:      "observation",
		Source:    "slack",
		Namespace: "team-alpha",
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store with namespace: %v", err)
	}

	if thought.ID == "" {
		t.Error("expected ID to be set after Store")
	}

	// Verify namespace was stored
	retrieved, err := brain.Get(ctx, thought.ID)
	if err != nil {
		t.Fatalf("Get stored thought: %v", err)
	}
	if retrieved.Namespace != "team-alpha" {
		t.Errorf("expected namespace 'team-alpha', got '%s'", retrieved.Namespace)
	}
}

func TestBrainStoreWithoutNamespaceUsesDefault(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{
		Content: "Thought without explicit namespace",
		Source:  "test",
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store without namespace: %v", err)
	}

	retrieved, err := brain.Get(ctx, thought.ID)
	if err != nil {
		t.Fatalf("Get stored thought: %v", err)
	}
	if retrieved.Namespace != "default" {
		t.Errorf("expected default namespace 'default', got '%s'", retrieved.Namespace)
	}
}

func TestBrainSearchFiltersByNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store thoughts in different namespaces
	thoughts := []*Thought{
		{Content: "Alice's frontend work", People: []string{"Alice"}, Namespace: "team-alpha", Source: "slack"},
		{Content: "Bob's backend work", People: []string{"Bob"}, Namespace: "team-beta", Source: "slack"},
		{Content: "Carol's frontend work", People: []string{"Carol"}, Namespace: "team-alpha", Source: "slack"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Search within team-alpha namespace
	results, err := brain.Search(ctx, "frontend work", 10, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Without namespace filter, should find all frontend work
	if len(results) < 2 {
		t.Errorf("expected at least 2 results without namespace filter, got %d", len(results))
	}
}

func TestBrainListRecentFiltersByNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store thoughts in different namespaces
	brain.Store(ctx, &Thought{Content: "Alpha thought 1", Namespace: "team-alpha", Source: "test"})
	brain.Store(ctx, &Thought{Content: "Beta thought 1", Namespace: "team-beta", Source: "test"})
	brain.Store(ctx, &Thought{Content: "Alpha thought 2", Namespace: "team-alpha", Source: "test"})

	since := time.Now().Add(-1 * time.Hour)

	// List recent without namespace filter should return all
	allResults, err := brain.ListRecent(ctx, since, 10, "")
	if err != nil {
		t.Fatalf("ListRecent all: %v", err)
	}
	if len(allResults) != 3 {
		t.Errorf("expected 3 total thoughts, got %d", len(allResults))
	}
}

func TestBrainStatsByNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store thoughts in different namespaces
	brain.Store(ctx, &Thought{Content: "Alpha 1", Namespace: "team-alpha", Topics: []string{"work"}, Source: "test"})
	brain.Store(ctx, &Thought{Content: "Alpha 2", Namespace: "team-alpha", Topics: []string{"work"}, Source: "test"})
	brain.Store(ctx, &Thought{Content: "Beta 1", Namespace: "team-beta", Topics: []string{"ai"}, Source: "test"})

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	// Without namespace filter, should count all
	if stats.TotalThoughts != 3 {
		t.Errorf("TotalThoughts: expected 3, got %d", stats.TotalThoughts)
	}
}

func TestBrainDeleteWithNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{Content: "To be deleted", Namespace: "team-alpha", Source: "test"}
	if err := brain.Store(ctx, thought); err != nil {
		t.Fatalf("Store: %v", err)
	}

	err := brain.Delete(ctx, thought.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deletion
	_, err = brain.Get(ctx, thought.ID)
	if err == nil {
		t.Error("expected error getting deleted thought")
	}
}

func TestBrainNamespaceIsolation(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store identical content in different namespaces
	thought1 := &Thought{Content: "Project roadmap discussion", Namespace: "team-alpha", Source: "meeting"}
	thought2 := &Thought{Content: "Project roadmap discussion", Namespace: "team-beta", Source: "meeting"}

	if err := brain.Store(ctx, thought1); err != nil {
		t.Fatalf("Store thought1: %v", err)
	}
	if err := brain.Store(ctx, thought2); err != nil {
		t.Fatalf("Store thought2: %v", err)
	}

	// Both should have different IDs
	if thought1.ID == thought2.ID {
		t.Error("thoughts in different namespaces should have different IDs")
	}

	// Search should find both
	results, err := brain.Search(ctx, "roadmap discussion", 10, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for identical content in different namespaces, got %d", len(results))
	}
}

func TestBrainReflectWithNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store two observations in same namespace
	brain.Store(ctx, &Thought{Content: "Obs 1: auth discussion", Type: "observation", Namespace: "team-alpha", Source: "agent"})
	brain.Store(ctx, &Thought{Content: "Obs 2: auth flow discussion", Type: "observation", Namespace: "team-alpha", Source: "agent"})

	// Get their IDs
	since := time.Now().Add(-1 * time.Hour)
	obs, _ := brain.ListRecent(ctx, since, 10, "observation")
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(obs))
	}

	// Reflect: consolidate into one with namespace preserved
	newThoughts := []*Thought{
		{Content: "Consolidated auth discussion", Type: "observation", Namespace: "team-alpha", Source: "agent"},
	}
	ids := []string{obs[0].ID, obs[1].ID}

	result, err := brain.Reflect(ctx, ids, newThoughts)
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}

	if len(result.Stored) != 1 {
		t.Errorf("expected 1 stored, got %d", len(result.Stored))
	}

	// Verify the new thought has the correct namespace
	newThought, err := brain.Get(ctx, result.Stored[0])
	if err != nil {
		t.Fatalf("Get reflected thought: %v", err)
	}
	if newThought.Namespace != "team-alpha" {
		t.Errorf("expected namespace 'team-alpha' on reflected thought, got '%s'", newThought.Namespace)
	}
}

func TestBrainBulkImportWithNamespace(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	jsonl := `{"content":"imported alpha","namespace":"team-alpha","topics":["work"],"type":"insight","source":"import"}
{"content":"imported beta","namespace":"team-beta","topics":["ai"],"type":"decision","source":"import"}
{"content":"imported default","topics":["life"],"source":"import"}
`

	count, err := brain.BulkImport(ctx, strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("BulkImport: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 imported, got %d", count)
	}

	// Verify all imported
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 3 {
		t.Errorf("expected 3 total after import, got %d", stats.TotalThoughts)
	}
}

func TestBrainListRecentWithTypeFilter(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	brain.Store(ctx, &Thought{Content: "regular thought", Type: "idea", Source: "test"})
	brain.Store(ctx, &Thought{Content: "observation", Type: "observation", Source: "agent"})

	since := time.Now().Add(-1 * time.Hour)
	results, err := brain.ListRecent(ctx, since, 10, "observation")
	if err != nil {
		t.Fatalf("ListRecent with type: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(results))
	}
}

// TestOllamaEmbedder tests the OllamaEmbedder with a mock server.
func TestOllamaEmbedder(t *testing.T) {
	// Start a mock Ollama server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		emb := make([]float64, 768)
		if len(req.Prompt) > 0 {
			idx := int(req.Prompt[0]) % 768
			emb[idx] = 1.0
		}

		json.NewEncoder(w).Encode(map[string]any{
			"embedding": emb,
		})
	}))
	defer ts.Close()

	embedder := NewOllamaEmbedder(ts.URL, "nomic-embed-text")
	ctx := context.Background()

	emb, err := embedder.Embed(ctx, "test prompt")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(emb) != 768 {
		t.Errorf("expected 768 dims, got %d", len(emb))
	}
}
