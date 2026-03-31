# Observational Memory for Picobrain — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add observational memory (OM) capabilities to picobrain — delete, reflect, type filtering, and observer/reflector MCP prompts — without adding LLM dependencies.

**Architecture:** Extend existing storage layer with delete and atomic reflect operations. Add type filtering to existing query functions. Register two MCP prompts (observer, reflector) and two new tools (delete_thought, reflect). The MCP client (agent) handles LLM calls; picobrain stays a storage/search layer.

**Tech Stack:** Go, SQLite, sqlite-vec, mcp-go v0.46.0

---

## Files Overview

| File | Change |
|------|--------|
| `store.go` | Add `deleteThought`, update `listRecent`/`searchByVector` with type filter, add `reflectTx` |
| `brain.go` | Add `Delete`, `Reflect` methods; update `Search`/`ListRecent` signatures |
| `mcp.go` | Register `delete_thought`/`reflect` tools, add prompts, add type param to existing tools |
| `prompts.go` | New file — observer and reflector prompt constants |
| `cmd/picobrain-mcp/main.go` | Enable prompt capabilities on MCP server |

---

### Task 1: Add delete support to store layer

**Files:**
- Modify: `store.go`
- Test: `store_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestDeleteThought -v`
Expected: FAIL with "deleteThought not defined"

**Step 3: Write minimal implementation in store.go**

Add after `insertThought` (around line 94):

```go
func deleteThought(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM thoughts WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete from thoughts: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM thought_vectors WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete from thought_vectors: %w", err)
	}

	return tx.Commit()
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestDeleteThought -v`
Expected: PASS

**Step 5: Commit**

```bash
git add store.go store_test.go
git commit -m "feat(store): add deleteThought function with tests"
```

---

### Task 2: Add Delete to brain layer

**Files:**
- Modify: `brain.go`
- Test: `brain_test.go`

**Step 1: Write the failing test**

```go
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

	results, err := brain.Search(ctx, "deleted", 10)
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBrainDelete -v`
Expected: FAIL with "brain.Delete undefined"

**Step 3: Write minimal implementation in brain.go**

Add after `Stats` method (around line 135):

```go
func (b *Brain) Delete(ctx context.Context, id string) error {
	return deleteThought(b.db, id)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestBrainDelete -v`
Expected: PASS

**Step 5: Commit**

```bash
git add brain.go brain_test.go
git commit -m "feat(brain): add Delete method with tests"
```

---

### Task 3: Add type filter to store layer

**Files:**
- Modify: `store.go`
- Test: `store_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run 'TestListRecentWithTypeFilter|TestSearchByVectorWithTypeFilter' -v`
Expected: FAIL with "too many arguments" (signature mismatch)

**Step 3: Update function signatures and queries in store.go**

Update `listRecent` signature from:
```go
func listRecent(db *sql.DB, since time.Time, limit int) ([]Thought, error) {
```
to:
```go
func listRecent(db *sql.DB, since time.Time, limit int, thoughtType string) ([]Thought, error) {
```

And update the query to conditionally add type filter:
```go
func listRecent(db *sql.DB, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	var rows *sql.Rows
	var err error
	if thoughtType != "" {
		rows, err = db.Query(`
			SELECT id, content, people, topics, type, action_items, source, created_at
			FROM thoughts
			WHERE created_at >= ? AND type = ?
			ORDER BY created_at DESC
			LIMIT ?
		`, since.Format("2006-01-02 15:04:05"), thoughtType, limit)
	} else {
		rows, err = db.Query(`
			SELECT id, content, people, topics, type, action_items, source, created_at
			FROM thoughts
			WHERE created_at >= ?
			ORDER BY created_at DESC
			LIMIT ?
		`, since.Format("2006-01-02 15:04:05"), limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	defer rows.Close()

	return scanThoughts(rows, false)
}
```

Update `searchByVector` signature from:
```go
func searchByVector(db *sql.DB, embedding []float32, limit int) ([]Thought, error) {
```
to:
```go
func searchByVector(db *sql.DB, embedding []float32, limit int, thoughtType string) ([]Thought, error) {
```

And update the query — fetch more results when filtering, then filter by type:
```go
func searchByVector(db *sql.DB, embedding []float32, limit int, thoughtType string) ([]Thought, error) {
	vec, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	// When filtering by type, fetch more to account for filtered-out results
	searchLimit := limit
	if thoughtType != "" {
		searchLimit = limit * 3 // heuristic: fetch 3x to compensate for filtering
	}

	rows, err := db.Query(`
		SELECT v.id, v.distance,
		       t.content, t.people, t.topics, t.type, t.action_items, t.source, t.created_at
		FROM thought_vectors v
		JOIN thoughts t ON t.id = v.id
		WHERE v.embedding MATCH ?
		AND k = ?
		ORDER BY v.distance
	`, vec, searchLimit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	all, err := scanThoughts(rows, true)
	if err != nil {
		return nil, err
	}

	if thoughtType == "" {
		return all, nil
	}

	// Filter by type
	filtered := make([]Thought, 0, limit)
	for _, t := range all {
		if t.Type == thoughtType {
			filtered = append(filtered, t)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}
```

**Step 4: Fix existing callers**

Update `brain.go` calls to pass empty string for type:
- `brain.go:123`: `searchByVector(b.db, queryEmb, limit, "")`
- `brain.go:130`: `listRecent(b.db, since, limit, "")`

**Step 5: Run test to verify it passes**

Run: `go test -run 'TestListRecentWithTypeFilter|TestSearchByVectorWithTypeFilter' -v`
Expected: PASS

Also run existing tests to ensure backward compatibility:
Run: `go test -run 'TestListRecent$|TestSearchByVector$' -v`
Expected: PASS

**Step 6: Commit**

```bash
git add store.go store_test.go brain.go
git commit -m "feat(store): add type filter to listRecent and searchByVector"
```

---

### Task 4: Add type filter to brain layer

**Files:**
- Modify: `brain.go`
- Test: `brain_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run 'TestBrainSearchWithTypeFilter|TestBrainListRecentWithTypeFilter' -v`
Expected: FAIL with "too many arguments in call to brain.Search"

**Step 3: Update brain.go signatures**

Update `Search`:
```go
func (b *Brain) Search(ctx context.Context, query string, limit int, thoughtType string) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return searchByVector(b.db, queryEmb, limit, thoughtType)
}
```

Update `ListRecent`:
```go
func (b *Brain) ListRecent(ctx context.Context, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	if limit <= 0 {
		limit = 20
	}
	return listRecent(b.db, since, limit, thoughtType)
}
```

**Step 4: Run tests**

Run: `go test -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add brain.go brain_test.go
git commit -m "feat(brain): add type filter to Search and ListRecent"
```

---

### Task 5: Add reflect support to store layer

**Files:**
- Modify: `store.go`
- Test: `store_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestReflectStore -v`
Expected: FAIL with "reflectTx not defined"

**Step 3: Write minimal implementation in store.go**

Add after `deleteThought`:

```go
func reflectTx(db *sql.DB, deleteIDs []string, newThoughts []*Thought) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete old thoughts
	for _, id := range deleteIDs {
		if _, err := tx.Exec("DELETE FROM thoughts WHERE id = ?", id); err != nil {
			return fmt.Errorf("delete thought %s: %w", id, err)
		}
		if _, err := tx.Exec("DELETE FROM thought_vectors WHERE id = ?", id); err != nil {
			return fmt.Errorf("delete vector %s: %w", id, err)
		}
	}

	// Store new thoughts
	for _, t := range newThoughts {
		if err := insertThoughtTx(tx, t); err != nil {
			return fmt.Errorf("insert reflected thought: %w", err)
		}
	}

	return tx.Commit()
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestReflectStore -v`
Expected: PASS

**Step 5: Commit**

```bash
git add store.go store_test.go
git commit -m "feat(store): add reflectTx for atomic delete+store operations"
```

---

### Task 6: Add Reflect to brain layer

**Files:**
- Modify: `brain.go`
- Test: `brain_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBrainReflect -v`
Expected: FAIL with "brain.Reflect undefined" or "ReflectResult undefined"

**Step 3: Write minimal implementation in brain.go**

Add a result struct and method:

```go
type ReflectResult struct {
	Stored  []string `json:"stored"`
	Deleted []string `json:"deleted"`
}

func (b *Brain) Reflect(ctx context.Context, deleteIDs []string, newThoughts []*Thought) (*ReflectResult, error) {
	// Generate embeddings for new thoughts
	for _, t := range newThoughts {
		if t.ID == "" {
			t.ID = uuid.New().String()
		}
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		if t.Embedding == nil {
			emb, err := b.embedder.Embed(ctx, t.Content)
			if err != nil {
				return nil, fmt.Errorf("generate embedding: %w", err)
			}
			t.Embedding = emb
		}
	}

	if err := reflectTx(b.db, deleteIDs, newThoughts); err != nil {
		return nil, err
	}

	stored := make([]string, len(newThoughts))
	for i, t := range newThoughts {
		stored[i] = t.ID
	}

	return &ReflectResult{Stored: stored, Deleted: deleteIDs}, nil
}
```

**Step 4: Run tests**

Run: `go test -run TestBrainReflect -v`
Expected: PASS

Run: `go test -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add brain.go brain_test.go
git commit -m "feat(brain): add Reflect method for atomic observation consolidation"
```

---

### Task 7: Create observer and reflector prompts

**Files:**
- Create: `prompts.go`
- Test: `prompts_test.go`

**Step 1: Write the test**

```go
package picobrain

import "testing"

func TestObserverPromptNotEmpty(t *testing.T) {
	if ObserverPrompt == "" {
		t.Fatal("ObserverPrompt should not be empty")
	}
	if len(ObserverPrompt) < 100 {
		t.Errorf("ObserverPrompt too short: %d chars", len(ObserverPrompt))
	}
}

func TestReflectorPromptNotEmpty(t *testing.T) {
	if ReflectorPrompt == "" {
		t.Fatal("ReflectorPrompt should not be empty")
	}
	if len(ReflectorPrompt) < 100 {
		t.Errorf("ReflectorPrompt too short: %d chars", len(ReflectorPrompt))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run 'TestObserverPromptNotEmpty|TestReflectorPromptNotEmpty' -v`
Expected: FAIL with "ObserverPrompt not defined"

**Step 3: Create prompts.go**

```go
package picobrain

const ObserverPrompt = `You are the observational memory subsystem of an AI agent. Your job is to compress conversation messages into dense, factual observations.

Given a sequence of messages from a conversation, extract and record:

1. What actions were taken (tool calls, file edits, commands run)
2. What information was discovered or learned
3. What decisions were made and why
4. What problems were encountered and how they were resolved
5. What remains pending or unresolved

Rules:
- Be specific: include file names, function names, error messages, variable names
- Preserve facts, not vibes — "Set timeout to 30s" not "Changed some config"
- Maintain chronological order
- Do NOT summarize — compress. Every sentence should contain concrete information.
- Omit pleasantries, confirmations, and filler
- If a tool was called with specific inputs/outputs, note the key details
- Each observation should be self-contained and understandable without the original messages

Output format: A numbered list of observations. Each observation is 1-3 sentences of dense information.`

const ReflectorPrompt = `You are the reflector — the long-term consolidation subsystem of an AI agent's memory. Given a set of existing observations, reorganize and consolidate them.

Your job:
1. MERGE observations that describe the same topic, decision, or ongoing work
2. DROP observations that are no longer relevant (resolved issues, superseded decisions, completed tasks)
3. KEEP observations that contain important facts, decisions, or unresolved items
4. REORGANIZE so related observations are grouped together

Rules:
- Consolidated observations should be denser than the originals — combine, don't just concatenate
- Preserve specific details (file names, function names, error messages, config values)
- If two observations contradict, keep the most recent one
- Drop observations about routine/tool mechanics unless they reveal important context
- The output should be shorter than the input — aim for at least 2x compression
- Maintain enough context that a future conversation could pick up where things left off

Output format: A numbered list of consolidated observations. Each observation is 1-3 sentences of dense information.`
```

**Step 4: Run test to verify it passes**

Run: `go test -run 'TestObserverPromptNotEmpty|TestReflectorPromptNotEmpty' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add prompts.go prompts_test.go
git commit -m "feat: add observer and reflector prompt constants"
```

---

### Task 8: Register new MCP tools and prompts

**Files:**
- Modify: `mcp.go`
- Modify: `cmd/picobrain-mcp/main.go`

**Step 1: Add delete_thought and reflect tools to mcp.go**

Add to `RegisterMCPTools`:

```go
// delete_thought
s.AddTool(
	mcp.NewTool("delete_thought",
		mcp.WithDescription("Delete a thought from your brain by ID. Used by the reflector to remove old observations after consolidation."),
		mcp.WithString("id", mcp.Required(), mcp.Description("The ID of the thought to delete")),
	),
	deleteThoughtHandler(brain),
)

// reflect
s.AddTool(
	mcp.NewTool("reflect",
		mcp.WithDescription("Consolidate observations: atomically delete old thoughts and store new consolidated ones. Core operation for observational memory reflection."),
		mcp.WithArray("delete_ids", mcp.Required(), mcp.Description("IDs of thoughts to delete")),
		mcp.WithArray("consolidated", mcp.Required(), mcp.Description("New consolidated thoughts to store (each with: content, people, topics, type, action_items, source)")),
	),
	reflectHandler(brain),
)
```

**Step 2: Add type filter to semantic_search and list_recent tools**

Update `semantic_search` tool definition — add:
```go
mcp.WithString("type", mcp.Description("Filter by thought type (e.g., 'observation'). Leave empty to search all types.")),
```

Update `list_recent` tool definition — add:
```go
mcp.WithString("type", mcp.Description("Filter by thought type (e.g., 'observation'). Leave empty for all types.")),
```

**Step 3: Write handler functions**

```go
func deleteThoughtHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		if err := brain.Delete(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"deleted": true,
			"id":      id,
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func reflectHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		deleteIDs := stringSliceArg(request, "delete_ids")
		if len(deleteIDs) == 0 {
			return mcp.NewToolResultError("delete_ids is required and must not be empty"), nil
		}

		consolidatedRaw, ok := request.GetArguments()["consolidated"]
		if !ok {
			return mcp.NewToolResultError("consolidated is required"), nil
		}

		consolidatedArr, ok := consolidatedRaw.([]any)
		if !ok || len(consolidatedArr) == 0 {
			return mcp.NewToolResultError("consolidated must be a non-empty array"), nil
		}

		newThoughts := make([]*Thought, 0, len(consolidatedArr))
		for i, item := range consolidatedArr {
			obj, ok := item.(map[string]any)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("consolidated[%d] must be an object", i)), nil
			}

			content, _ := obj["content"].(string)
			if content == "" {
				return mcp.NewToolResultError(fmt.Sprintf("consolidated[%d].content is required", i)), nil
			}

			t := &Thought{
				Content: content,
				Type:    getStringFromMap(obj, "type"),
				Source:  getStringFromMap(obj, "source"),
			}
			if people, ok := obj["people"].([]any); ok {
				t.People = toStringSlice(people)
			}
			if topics, ok := obj["topics"].([]any); ok {
				t.Topics = toStringSlice(topics)
			}
			if items, ok := obj["action_items"].([]any); ok {
				t.ActionItems = toStringSlice(items)
			}

			newThoughts = append(newThoughts, t)
		}

		result, err := brain.Reflect(ctx, deleteIDs, newThoughts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("reflect failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toStringSlice(arr []any) []string {
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
```

**Step 4: Update semantic_search handler to pass type**

Update `semanticSearchHandler`:
```go
thoughtType := request.GetString("type", "")
results, err := brain.Search(ctx, query, limit, thoughtType)
```

**Step 5: Update list_recent handler to pass type**

Update `listRecentHandler`:
```go
thoughtType := request.GetString("type", "")
results, err := brain.ListRecent(ctx, since, limit, thoughtType)
```

**Step 6: Register MCP prompts in main.go**

Update `cmd/picobrain-mcp/main.go`:

```go
s := server.NewMCPServer("picobrain", "0.1.0",
	server.WithPromptCapabilities(false),
)

picobrain.RegisterMCPTools(s, brain)

// Register OM prompts
s.AddPrompt(
	mcp.NewPrompt("observe",
		mcp.WithPromptDescription("System prompt for compressing conversation messages into dense observations"),
	),
	func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult("System prompt for observational memory", []mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleSystem, mcp.NewTextContent(picobrain.ObserverPrompt)),
		}), nil
	},
)

s.AddPrompt(
	mcp.NewPrompt("reflect",
		mcp.WithPromptDescription("System prompt for consolidating and pruning observations"),
	),
	func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult("System prompt for memory reflection", []mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleSystem, mcp.NewTextContent(picobrain.ReflectorPrompt)),
		}), nil
	},
)
```

**Step 7: Verify build compiles**

Run: `go build ./...`
Expected: SUCCESS

**Step 8: Run all tests**

Run: `go test -v`
Expected: ALL PASS

**Step 9: Commit**

```bash
git add mcp.go cmd/picobrain-mcp/main.go
git commit -m "feat(mcp): register delete_thought, reflect tools and observe/reflect prompts"
```

---

### Task 9: End-to-end verification

**Step 1: Run full test suite**

Run: `go test ./... -v -count=1`
Expected: ALL PASS

**Step 2: Run linter**

Run: `go vet ./...`
Expected: no output (clean)

**Step 3: Verify MCP server starts**

Run: `go run ./cmd/picobrain-mcp/ --db :memory: --no-auto-download` then Ctrl+C
Expected: starts without error

**Step 4: Commit (if needed)**

```bash
git add -A
git commit -m "chore: final verification pass"
```

---

## Summary of Changes

**New files:** `prompts.go`, `prompts_test.go`

**Modified files:** `store.go`, `brain.go`, `mcp.go`, `cmd/picobrain-mcp/main.go`, `store_test.go`, `brain_test.go`

**New MCP tools:** `delete_thought`, `reflect`

**Enhanced MCP tools:** `semantic_search` (type filter), `list_recent` (type filter)

**New MCP prompts:** `observe`, `reflect`

**No schema changes. No new dependencies.**
