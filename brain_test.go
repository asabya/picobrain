package picobrain

import (
	"context"
	"encoding/json"
	"fmt"
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

// Priority system tests

func TestBrainStoreWithPriority(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought := &Thought{
		Content:  "Critical system configuration",
		Priority: "critical",
		Type:     "decision",
		Source:   "test",
	}

	err := brain.Store(ctx, thought)
	if err != nil {
		t.Fatalf("Store with priority: %v", err)
	}

	// Retrieve and verify priority
	retrieved, err := brain.Get(ctx, thought.ID)
	if err != nil {
		t.Fatalf("Get thought: %v", err)
	}

	if retrieved.Priority != "critical" {
		t.Errorf("expected priority 'critical', got '%s'", retrieved.Priority)
	}
}

func TestBrainStoreWithAllPriorityLevels(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	priorities := []string{"low", "medium", "high", "critical"}

	for _, p := range priorities {
		thought := &Thought{
			Content:  fmt.Sprintf("Thought with %s priority", p),
			Priority: p,
			Source:   "test",
		}
		if err := brain.Store(ctx, thought); err != nil {
			t.Fatalf("Store with priority %s: %v", p, err)
		}

		// Verify it was stored correctly
		retrieved, err := brain.Get(ctx, thought.ID)
		if err != nil {
			t.Fatalf("Get thought with priority %s: %v", p, err)
		}
		if retrieved.Priority != p {
			t.Errorf("priority %s: expected '%s', got '%s'", p, p, retrieved.Priority)
		}
	}
}

func TestBrainListRecentSortedByPriority(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store thoughts in non-priority order
	thoughts := []*Thought{
		{Content: "Low priority thought", Priority: "low", Source: "test"},
		{Content: "Critical priority thought", Priority: "critical", Source: "test"},
		{Content: "Medium priority thought", Priority: "medium", Source: "test"},
		{Content: "High priority thought", Priority: "high", Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	since := time.Now().Add(-1 * time.Hour)
	results, err := brain.ListRecent(ctx, since, 10, "")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Verify results are sorted by priority (critical > high > medium > low)
	expectedOrder := []string{"critical", "high", "medium", "low"}
	for i, result := range results {
		if result.Priority != expectedOrder[i] {
			t.Errorf("position %d: expected priority '%s', got '%s'", i, expectedOrder[i], result.Priority)
		}
	}
}

func TestBrainListRecentSortedByPriorityAndDate(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store multiple thoughts with same priority to verify secondary sort by date
	thoughts := []*Thought{
		{Content: "Older critical", Priority: "critical", CreatedAt: time.Now().Add(-2 * time.Hour), Source: "test"},
		{Content: "Newer critical", Priority: "critical", CreatedAt: time.Now().Add(-1 * time.Hour), Source: "test"},
		{Content: "Older high", Priority: "high", CreatedAt: time.Now().Add(-2 * time.Hour), Source: "test"},
		{Content: "Newer high", Priority: "high", CreatedAt: time.Now().Add(-1 * time.Hour), Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	since := time.Now().Add(-3 * time.Hour)
	results, err := brain.ListRecent(ctx, since, 10, "")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// First should be critical (priority order), then within critical, newer first
	if results[0].Priority != "critical" {
		t.Errorf("first result should be critical priority, got %s", results[0].Priority)
	}
	if results[2].Priority != "high" {
		t.Errorf("third result should be high priority, got %s", results[2].Priority)
	}
}

func TestBrainPruneExcludesCritical(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store old thoughts (simulating old dates by setting CreatedAt)
	oldTime := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago

	thoughts := []*Thought{
		{Content: "Old normal thought", Priority: "medium", CreatedAt: oldTime, Source: "test"},
		{Content: "Old critical thought", Priority: "critical", CreatedAt: oldTime, Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune thoughts older than 30 days
	prunedCount, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should have pruned only 1 (the medium priority one)
	if prunedCount != 1 {
		t.Errorf("expected 1 pruned thought, got %d", prunedCount)
	}

	// Verify the critical thought still exists
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 1 {
		t.Errorf("expected 1 thought remaining (critical), got %d", stats.TotalThoughts)
	}

	// Verify it's the critical one
	remaining, err := brain.ListRecent(ctx, time.Now().Add(-100*24*time.Hour), 10, "")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if len(remaining) != 1 || remaining[0].Priority != "critical" {
		t.Errorf("remaining thought should be critical, got %v", remaining)
	}
}

func TestBrainPruneWithNoCritical(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-60 * 24 * time.Hour)

	// Store only non-critical old thoughts
	thoughts := []*Thought{
		{Content: "Old low priority", Priority: "low", CreatedAt: oldTime, Source: "test"},
		{Content: "Old medium priority", Priority: "medium", CreatedAt: oldTime, Source: "test"},
		{Content: "Old high priority", Priority: "high", CreatedAt: oldTime, Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune thoughts older than 30 days
	prunedCount, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should have pruned all 3
	if prunedCount != 3 {
		t.Errorf("expected 3 pruned thoughts, got %d", prunedCount)
	}

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 0 {
		t.Errorf("expected 0 thoughts remaining, got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneDoesNotAffectRecent(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	recentTime := time.Now().Add(-5 * 24 * time.Hour) // 5 days ago

	thoughts := []*Thought{
		{Content: "Recent low", Priority: "low", CreatedAt: recentTime, Source: "test"},
		{Content: "Recent critical", Priority: "critical", CreatedAt: recentTime, Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune thoughts older than 30 days
	prunedCount, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should not prune any (all are recent)
	if prunedCount != 0 {
		t.Errorf("expected 0 pruned thoughts (all recent), got %d", prunedCount)
	}

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 2 {
		t.Errorf("expected 2 thoughts remaining, got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneWithMixedAges(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-60 * 24 * time.Hour)   // 60 days ago
	recentTime := time.Now().Add(-5 * 24 * time.Hour) // 5 days ago

	thoughts := []*Thought{
		{Content: "Old medium", Priority: "medium", CreatedAt: oldTime, Source: "test"},
		{Content: "Old critical", Priority: "critical", CreatedAt: oldTime, Source: "test"},
		{Content: "Recent medium", Priority: "medium", CreatedAt: recentTime, Source: "test"},
		{Content: "Recent critical", Priority: "critical", CreatedAt: recentTime, Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune thoughts older than 30 days
	prunedCount, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should prune only the old medium (not old critical, not recent ones)
	if prunedCount != 1 {
		t.Errorf("expected 1 pruned thought (old medium), got %d", prunedCount)
	}

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 3 {
		t.Errorf("expected 3 thoughts remaining, got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneZeroDays(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-60 * 24 * time.Hour)

	thoughts := []*Thought{
		{Content: "Old medium", Priority: "medium", CreatedAt: oldTime, Source: "test"},
		{Content: "Old critical", Priority: "critical", CreatedAt: oldTime, Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune with 0 days (should disable pruning)
	prunedCount, err := brain.Prune(ctx, 0)
	if err != nil {
		t.Fatalf("Prune with 0 days: %v", err)
	}

	if prunedCount != 0 {
		t.Errorf("expected 0 pruned thoughts with 0 days threshold, got %d", prunedCount)
	}

	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalThoughts != 2 {
		t.Errorf("expected 2 thoughts remaining with 0 days threshold, got %d", stats.TotalThoughts)
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
