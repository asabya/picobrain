package picobrain

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func toolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}}
}

func decodeToolText(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected tool result content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("decode tool payload: %v", err)
	}
	return payload
}

func structuredClaim(subject, predicate, object string) map[string]any {
	return map[string]any{
		"subject":     subject,
		"predicate":   predicate,
		"object":      object,
		"polarity":    "affirmed",
		"cardinality": "many",
		"status":      "active",
	}
}

func TestStoreThoughtHandlerStoresStructuredThought(t *testing.T) {
	brain := testBrain(t)
	handler := storeThoughtHandler(brain)
	result, err := handler(context.Background(), toolRequest("store_thought", map[string]any{
		"summary": "Picobrain startup requires SpaCy.",
		"claims":  []any{structuredClaim("picobrain_startup", "requires", "spacy_parser")},
		"type":    "decision",
		"topics":  []any{"depgraph"},
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload := decodeToolText(t, result)
	if payload["status"] != "stored" {
		t.Fatalf("expected stored status, got %v", payload)
	}
	id := payload["id"].(string)
	stored, err := brain.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("brain.Get: %v", err)
	}
	if stored.Summary != "Picobrain startup requires SpaCy." {
		t.Fatalf("unexpected summary: %q", stored.Summary)
	}
	if stored.Namespace != "default" {
		t.Fatalf("expected default namespace, got %q", stored.Namespace)
	}
	if len(stored.Claims) != 1 || stored.Claims[0].ID == "" {
		t.Fatalf("expected stored claim with generated ID, got %+v", stored.Claims)
	}
}

func TestStoreThoughtHandlerRejectsInvalidEnum(t *testing.T) {
	brain := testBrain(t)
	handler := storeThoughtHandler(brain)
	result, err := handler(context.Background(), toolRequest("store_thought", map[string]any{
		"summary": "bad polarity",
		"claims": []any{map[string]any{
			"subject":     "x",
			"predicate":   "is",
			"object":      "y",
			"polarity":    "sideways",
			"cardinality": "many",
			"status":      "active",
		}},
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload := decodeToolText(t, result)
	if payload["error"] != "validation_failed" {
		t.Fatalf("expected validation error, got %v", payload)
	}
	if payload["field"] != "claims[0].polarity" {
		t.Fatalf("unexpected field: %v", payload)
	}
}

func TestReflectHandlerTransactionalFailurePreservesExistingThoughts(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()
	first := &Thought{Summary: "first", Claims: []Claim{{Subject: "a", Predicate: "b", Object: "c", Polarity: "affirmed", Cardinality: "many", Status: "active"}}}
	second := &Thought{Summary: "second", Claims: []Claim{{Subject: "d", Predicate: "e", Object: "f", Polarity: "affirmed", Cardinality: "many", Status: "active"}}}
	if err := brain.store(ctx, first, true, false); err != nil {
		t.Fatalf("store first: %v", err)
	}
	if err := brain.store(ctx, second, true, false); err != nil {
		t.Fatalf("store second: %v", err)
	}
	handler := reflectHandler(brain)
	result, err := handler(ctx, toolRequest("reflect", map[string]any{
		"delete_ids": []any{first.ID, second.ID},
		"consolidated": []any{map[string]any{
			"summary": "invalid consolidated",
			"claims":  []any{},
		}},
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload := decodeToolText(t, result)
	if payload["error"] != "validation_failed" {
		t.Fatalf("expected validation failure, got %v", payload)
	}
	if _, err := brain.Get(ctx, first.ID); err != nil {
		t.Fatalf("first thought should still exist: %v", err)
	}
	if _, err := brain.Get(ctx, second.ID); err != nil {
		t.Fatalf("second thought should still exist: %v", err)
	}
}

func TestBulkImportHandlerPreservesClaimIDsAndIsTransactional(t *testing.T) {
	brain := testBrain(t)
	handler := bulkImportHandler(brain)
	jsonl := strings.Join([]string{
		`{"id":"thought-1","summary":"first","claims":[{"id":"claim-1","subject":"svc","predicate":"uses","object":"db","polarity":"affirmed","cardinality":"many","status":"active"}]}`,
		`{"id":"thought-2","summary":"second","claims":[{"id":"claim-2","subject":"svc","predicate":"uses","object":"cache","polarity":"affirmed","cardinality":"many","status":"active","supersedes_claim_id":"claim-1"}]}`,
	}, "\n")
	result, err := handler(context.Background(), toolRequest("bulk_import", map[string]any{"jsonl": jsonl}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload := decodeToolText(t, result)
	if payload["status"] != "imported" {
		t.Fatalf("expected import success, got %v", payload)
	}
	stored := payload["stored"].([]any)
	if len(stored) != 2 {
		t.Fatalf("expected 2 stored rows, got %v", payload)
	}
	thought, err := brain.Get(context.Background(), "thought-2")
	if err != nil {
		t.Fatalf("get imported thought: %v", err)
	}
	if thought.Claims[0].ID != "claim-2" || thought.Claims[0].SupersedesClaimID != "claim-1" {
		t.Fatalf("expected preserved claim IDs, got %+v", thought.Claims)
	}

	bad := strings.Join([]string{
		`{"id":"thought-a","summary":"valid","claims":[{"id":"claim-a","subject":"a","predicate":"b","object":"c","polarity":"affirmed","cardinality":"many","status":"active"}]}`,
		`{"id":"thought-b","summary":"invalid","claims":[]}`,
	}, "\n")
	result, err = handler(context.Background(), toolRequest("bulk_import", map[string]any{"jsonl": bad}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload = decodeToolText(t, result)
	if payload["error"] != "validation_failed" {
		t.Fatalf("expected validation failure, got %v", payload)
	}
	if payload["line"] != "2" {
		t.Fatalf("expected line metadata, got %v", payload)
	}
	stats, err := brain.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalThoughts != 2 {
		t.Fatalf("expected failed import to be transactional; total thoughts=%d", stats.TotalThoughts)
	}
}

func TestImportExportPreservesClaimIDsAndRejectsCSV(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()
	thought := &Thought{ID: "thought-export", Summary: "export me", Claims: []Claim{{ID: "claim-export", Subject: "svc", Predicate: "uses", Object: "db", Polarity: "affirmed", Cardinality: "many", Status: "active"}}, Namespace: "default"}
	if err := brain.store(ctx, thought, true, true); err != nil {
		t.Fatalf("store strict: %v", err)
	}
	var buf bytes.Buffer
	if err := brain.Export(ctx, &buf, "jsonl", ExportFilter{Namespace: "default"}); err != nil {
		t.Fatalf("export: %v", err)
	}
	brain2 := testBrain(t)
	count, err := brain2.Import(ctx, bytes.NewReader(buf.Bytes()), "jsonl")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported thought, got %d", count)
	}
	imported, err := brain2.Get(ctx, "thought-export")
	if err != nil {
		t.Fatalf("get imported: %v", err)
	}
	if len(imported.Claims) != 1 || imported.Claims[0].ID != "claim-export" {
		t.Fatalf("expected preserved claim IDs, got %+v", imported.Claims)
	}
	if _, err := brain.Import(ctx, strings.NewReader("id,summary\n1,test\n"), "csv"); err == nil {
		t.Fatal("expected csv import to be unsupported")
	}
}

func TestLintAndIndexDeterministicBehavior(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()
	mustStore := func(tw *Thought) {
		t.Helper()
		if err := brain.store(ctx, tw, true, true); err != nil {
			t.Fatalf("store strict: %v", err)
		}
	}
	mustStore(&Thought{ID: "t1", Summary: "svc uses db", Namespace: "default", Topics: []string{"infra"}, Claims: []Claim{{ID: "c1", Subject: "svc", Predicate: "uses", Object: "db", Polarity: "affirmed", Cardinality: "one", Status: "active"}}})
	mustStore(&Thought{ID: "t2", Summary: "svc uses cache", Namespace: "default", Topics: []string{"infra"}, Claims: []Claim{{ID: "c2", Subject: "svc", Predicate: "uses", Object: "cache", Polarity: "affirmed", Cardinality: "one", Status: "active"}}})
	mustStore(&Thought{ID: "t3", Summary: "svc uses db duplicate", Namespace: "default", Claims: []Claim{{ID: "c3", Subject: "svc", Predicate: "uses", Object: "db", Polarity: "affirmed", Cardinality: "one", Status: "active"}}})
	mustStore(&Thought{ID: "t4", Summary: "svc used api", Namespace: "default", Claims: []Claim{{ID: "c4", Subject: "svc", Predicate: "uses", Object: "api-v1", Polarity: "affirmed", Cardinality: "many", Status: "superseded"}}})
	mustStore(&Thought{ID: "t5", Summary: "svc uses api-v2", Namespace: "default", Claims: []Claim{{ID: "c5", Subject: "svc", Predicate: "uses", Object: "api-v2", Polarity: "affirmed", Cardinality: "many", Status: "active", SupersedesClaimID: "c4"}}})
	mustStore(&Thought{ID: "t6", Summary: "orphan", Namespace: "default", Claims: []Claim{{ID: "c6", Subject: "loner", Predicate: "exists", Object: "thing", Polarity: "affirmed", Cardinality: "many", Status: "active"}}})
	issues, err := brain.Lint(ctx, "default")
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	seen := map[string]bool{}
	for _, issue := range issues {
		seen[issue.Type] = true
	}
	for _, want := range []string{"contradiction", "duplicate", "superseded", "orphan"} {
		if !seen[want] {
			t.Fatalf("expected lint issue %q in %+v", want, issues)
		}
	}
	index, err := brain.Index(ctx, "default")
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if len(index.ByTopic["infra"]) == 0 {
		t.Fatalf("expected topic grouping in index")
	}
	if len(index.ConflictBuckets["contradiction"]) == 0 {
		t.Fatalf("expected contradiction conflict bucket in index: %+v", index.ConflictBuckets)
	}
}

func TestLintAndIndexAreDeterministic(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()
	items := []*Thought{
		{ID: "a1", Summary: "svc uses db", Namespace: "default", Topics: []string{"infra"}, Claims: []Claim{{ID: "ca1", Subject: "svc", Predicate: "uses", Object: "db", Polarity: "affirmed", Cardinality: "one", Status: "active"}}},
		{ID: "a2", Summary: "svc uses cache", Namespace: "default", Topics: []string{"infra"}, Claims: []Claim{{ID: "ca2", Subject: "svc", Predicate: "uses", Object: "cache", Polarity: "affirmed", Cardinality: "one", Status: "active"}}},
		{ID: "a3", Summary: "svc duplicate db", Namespace: "default", Claims: []Claim{{ID: "ca3", Subject: "svc", Predicate: "uses", Object: "db", Polarity: "affirmed", Cardinality: "one", Status: "active"}}},
	}
	for _, item := range items {
		if err := brain.store(ctx, item, true, true); err != nil {
			t.Fatalf("store: %v", err)
		}
	}
	firstIssues, err := brain.Lint(ctx, "default")
	if err != nil {
		t.Fatalf("lint first: %v", err)
	}
	secondIssues, err := brain.Lint(ctx, "default")
	if err != nil {
		t.Fatalf("lint second: %v", err)
	}
	firstJSON, _ := json.Marshal(firstIssues)
	secondJSON, _ := json.Marshal(secondIssues)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("lint output should be deterministic: %s vs %s", firstJSON, secondJSON)
	}
	firstIndex, err := brain.Index(ctx, "default")
	if err != nil {
		t.Fatalf("index first: %v", err)
	}
	secondIndex, err := brain.Index(ctx, "default")
	if err != nil {
		t.Fatalf("index second: %v", err)
	}
	firstIndexJSON, _ := json.Marshal(firstIndex)
	secondIndexJSON, _ := json.Marshal(secondIndex)
	if string(firstIndexJSON) != string(secondIndexJSON) {
		t.Fatalf("index output should be deterministic: %s vs %s", firstIndexJSON, secondIndexJSON)
	}
}

func TestCanonicalJSONLDoesNotRoundTripEdges(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()
	left := &Thought{ID: "edge-left", Summary: "left", Claims: []Claim{{ID: "edge-claim-left", Subject: "left", Predicate: "connects", Object: "right", Polarity: "affirmed", Cardinality: "many", Status: "active"}}}
	right := &Thought{ID: "edge-right", Summary: "right", Claims: []Claim{{ID: "edge-claim-right", Subject: "right", Predicate: "connects", Object: "left", Polarity: "affirmed", Cardinality: "many", Status: "active"}}}
	if err := brain.store(ctx, left, true, true); err != nil {
		t.Fatalf("store left: %v", err)
	}
	if err := brain.store(ctx, right, true, true); err != nil {
		t.Fatalf("store right: %v", err)
	}
	if err := brain.CreateEdge(ctx, &Edge{SourceID: left.ID, TargetID: right.ID, RelationType: "relates_to"}); err != nil {
		t.Fatalf("create edge: %v", err)
	}
	var buf bytes.Buffer
	if err := brain.Export(ctx, &buf, "jsonl", ExportFilter{IncludeEdges: true}); err != nil {
		t.Fatalf("export: %v", err)
	}
	if strings.Contains(buf.String(), `"_type":"edge"`) {
		t.Fatalf("canonical JSONL should not contain edge records: %s", buf.String())
	}
	brain2 := testBrain(t)
	count, err := brain2.Import(ctx, bytes.NewReader(buf.Bytes()), "jsonl")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported records, got %d", count)
	}
	stats, err := brain2.GraphStats(ctx)
	if err != nil {
		t.Fatalf("graph stats: %v", err)
	}
	if stats.TotalEdges != 0 {
		t.Fatalf("expected 0 imported edges, got %d", stats.TotalEdges)
	}
}
