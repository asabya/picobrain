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
	results, err := brain.Search(ctx, "Pre-embedded thought", 1)
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

	results, err := brain.Search(ctx, "Alice frontend", 2)
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
	results, err := brain.ListRecent(ctx, since, 10)
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
