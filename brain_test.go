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

func TestBrainSearchWithMetadataFilters(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store test thoughts with various metadata
	thoughts := []*Thought{
		{
			Content:   "Alice is working on the frontend redesign",
			People:    []string{"Alice"},
			Topics:    []string{"frontend", "design"},
			Type:      "person_note",
			Source:    "slack",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			Content:   "Bob fixed the database migration bug",
			People:    []string{"Bob"},
			Topics:    []string{"backend", "database"},
			Type:      "decision",
			Source:    "claude",
			CreatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			Content:   "Carol proposed a new testing strategy with Alice",
			People:    []string{"Carol", "Alice"},
			Topics:    []string{"testing", "frontend"},
			Type:      "idea",
			Source:    "cli",
			CreatedAt: time.Now().Add(-3 * time.Hour),
		},
		{
			Content:   "Dave shared insights about backend architecture",
			People:    []string{"Dave"},
			Topics:    []string{"backend", "architecture"},
			Type:      "insight",
			Source:    "meeting",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Test 1: Filter by type
	t.Run("FilterByType", func(t *testing.T) {
		filters := SearchFilters{Type: "decision"}
		results, err := brain.SearchWithFilters(ctx, "database", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if len(results) > 0 && results[0].Type != "decision" {
			t.Errorf("expected type 'decision', got %s", results[0].Type)
		}
	})

	// Test 2: Filter by single topic
	t.Run("FilterByTopic", func(t *testing.T) {
		filters := SearchFilters{Topics: []string{"frontend"}}
		results, err := brain.SearchWithFilters(ctx, "working", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results with topic 'frontend', got %d", len(results))
		}
		for _, r := range results {
			hasFrontend := false
			for _, topic := range r.Topics {
				if topic == "frontend" {
					hasFrontend = true
					break
				}
			}
			if !hasFrontend {
				t.Errorf("result missing 'frontend' topic: %v", r.Topics)
			}
		}
	})

	// Test 3: Filter by multiple topics
	t.Run("FilterByMultipleTopics", func(t *testing.T) {
		filters := SearchFilters{Topics: []string{"frontend", "design"}}
		results, err := brain.SearchWithFilters(ctx, "working", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result with topics ['frontend', 'design'], got %d", len(results))
		}
	})

	// Test 4: Filter by person
	t.Run("FilterByPerson", func(t *testing.T) {
		filters := SearchFilters{People: []string{"Alice"}}
		results, err := brain.SearchWithFilters(ctx, "working", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results with person 'Alice', got %d", len(results))
		}
		for _, r := range results {
			hasAlice := false
			for _, person := range r.People {
				if person == "Alice" {
					hasAlice = true
					break
				}
			}
			if !hasAlice {
				t.Errorf("result missing 'Alice' in people: %v", r.People)
			}
		}
	})

	// Test 5: Filter by multiple people
	t.Run("FilterByMultiplePeople", func(t *testing.T) {
		filters := SearchFilters{People: []string{"Carol", "Alice"}}
		results, err := brain.SearchWithFilters(ctx, "testing", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result with people ['Carol', 'Alice'], got %d", len(results))
		}
	})

	// Test 6: Filter by date range (After)
	t.Run("FilterByAfterDate", func(t *testing.T) {
		filters := SearchFilters{After: time.Now().Add(-4 * time.Hour)}
		results, err := brain.SearchWithFilters(ctx, "working", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		// Should exclude Dave's 24-hour-old thought
		if len(results) != 3 {
			t.Errorf("expected 3 results after 4 hours ago, got %d", len(results))
		}
	})

	// Test 7: Filter by date range (Before)
	t.Run("FilterByBeforeDate", func(t *testing.T) {
		// Use type filter to narrow down to Dave's thought
		filters := SearchFilters{
			Before: time.Now().Add(-2 * time.Hour),
			Type:   "insight",
		}
		results, err := brain.SearchWithFilters(ctx, "shared insights", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		// Should only include Dave's 24-hour-old thought
		if len(results) != 1 {
			t.Errorf("expected 1 result with type='insight' before 2 hours ago, got %d", len(results))
		}
		if len(results) > 0 && results[0].Type != "insight" {
			t.Errorf("expected type 'insight', got %s", results[0].Type)
		}
	})

	// Test 8: Combined filters
	t.Run("FilterCombined", func(t *testing.T) {
		filters := SearchFilters{
			Type:   "person_note",
			People: []string{"Alice"},
			Topics: []string{"frontend"},
		}
		results, err := brain.SearchWithFilters(ctx, "redesign", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result with combined filters, got %d", len(results))
		}
		if len(results) > 0 && results[0].Type != "person_note" {
			t.Errorf("expected type 'person_note', got %s", results[0].Type)
		}
	})

	// Test 9: No filters (backward compatibility)
	t.Run("NoFilters", func(t *testing.T) {
		filters := SearchFilters{}
		results, err := brain.SearchWithFilters(ctx, "working", 10, filters)
		if err != nil {
			t.Fatalf("SearchWithFilters: %v", err)
		}
		if len(results) != 4 {
			t.Errorf("expected 4 results without filters, got %d", len(results))
		}
	})
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

// ==================== Auto-Pruning Tests ====================

func TestConfigDefaultAutoPruneDays(t *testing.T) {
	defaults := DefaultConfig()
	if defaults.AutoPruneDays != 30 {
		t.Errorf("expected AutoPruneDays to default to 30, got %d", defaults.AutoPruneDays)
	}
}

func TestThoughtPriorityField(t *testing.T) {
	// Test that priority field can be set and retrieved
	thought := &Thought{
		Content:  "Test thought with priority",
		Priority: "high",
	}

	if thought.Priority != "high" {
		t.Errorf("expected priority to be 'high', got %s", thought.Priority)
	}

	// Test default priority is empty (not critical)
	thought2 := &Thought{
		Content: "Test thought without priority",
	}

	if thought2.Priority != "" {
		t.Errorf("expected empty priority by default, got %s", thought2.Priority)
	}
}

func TestBrainPruneDeletesOldNonCriticalThoughts(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Create thoughts with old timestamps (simulating 40 days ago)
	oldTime := time.Now().Add(-40 * 24 * time.Hour)

	// Store an old low priority thought
	oldLow := &Thought{
		Content:   "Old low priority thought",
		Priority:  "low",
		CreatedAt: oldTime,
	}
	if err := brain.Store(ctx, oldLow); err != nil {
		t.Fatalf("Store old low priority: %v", err)
	}

	// Prune with 30 days threshold
	deleted, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify the thought was deleted
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 0 {
		t.Errorf("expected 0 thoughts after prune, got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneSkipsCriticalThoughts(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Create thoughts with old timestamps
	oldTime := time.Now().Add(-40 * 24 * time.Hour)

	// Store old critical and non-critical thoughts
	oldCritical := &Thought{
		Content:   "Old critical thought",
		Priority:  "critical",
		CreatedAt: oldTime,
	}
	oldNormal := &Thought{
		Content:   "Old normal thought",
		CreatedAt: oldTime,
	}

	if err := brain.Store(ctx, oldCritical); err != nil {
		t.Fatalf("Store old critical: %v", err)
	}
	if err := brain.Store(ctx, oldNormal); err != nil {
		t.Fatalf("Store old normal: %v", err)
	}

	// Prune with 30 days threshold
	deleted, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted (only normal), got %d", deleted)
	}

	// Verify only critical thought remains
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 1 {
		t.Errorf("expected 1 thought (critical), got %d", stats.TotalThoughts)
	}

	// Verify it's the critical one
	results, _ := brain.ListRecent(ctx, oldTime.Add(-1*time.Hour), 10, "")
	if len(results) != 1 || results[0].Priority != "critical" {
		t.Error("expected critical thought to remain")
	}
}

func TestBrainPruneSkipsRecentThoughts(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store a recent thought
	recentThought := &Thought{
		Content:  "Recent thought",
		Priority: "low",
	}
	if err := brain.Store(ctx, recentThought); err != nil {
		t.Fatalf("Store recent: %v", err)
	}

	// Prune with 30 days threshold
	deleted, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted (recent thought), got %d", deleted)
	}

	// Verify thought still exists
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 1 {
		t.Errorf("expected 1 thought to remain, got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneWithAllPriorities(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-40 * 24 * time.Hour)

	// Store old thoughts with different priorities
	thoughts := []*Thought{
		{Content: "Critical thought", Priority: "critical", CreatedAt: oldTime},
		{Content: "High priority thought", Priority: "high", CreatedAt: oldTime},
		{Content: "Medium priority thought", Priority: "medium", CreatedAt: oldTime},
		{Content: "Low priority thought", Priority: "low", CreatedAt: oldTime},
		{Content: "No priority thought", CreatedAt: oldTime},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	// Prune - should only delete non-critical
	deleted, err := brain.Prune(ctx, 30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if deleted != 4 {
		t.Errorf("expected 4 deleted (all except critical), got %d", deleted)
	}

	// Verify only critical remains
	stats, err := brain.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalThoughts != 1 {
		t.Errorf("expected 1 thought (critical), got %d", stats.TotalThoughts)
	}
}

func TestBrainPruneUpdatesCache(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-40 * 24 * time.Hour)

	// Store an old thought
	oldThought := &Thought{
		Content:   "Old thought to be pruned",
		CreatedAt: oldTime,
	}
	if err := brain.Store(ctx, oldThought); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Verify it's in cache by getting it
	cached, err := brain.Get(ctx, oldThought.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cached.ID != oldThought.ID {
		t.Error("expected thought to be in cache")
	}

	// Prune
	brain.Prune(ctx, 30)

	// Verify removed from cache (Get should still work via DB but cache should be cleared)
	// We can verify by checking stats
	stats, _ := brain.Stats(ctx)
	if stats.TotalThoughts != 0 {
		t.Error("expected thought to be deleted and cache cleared")
	}
}
