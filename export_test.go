package picobrain

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBrainExport(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store test thoughts
	brain.Store(ctx, &Thought{Content: "Thought 1", People: []string{"Alice"}, Topics: []string{"work"}, Type: "insight", Source: "test"})
	brain.Store(ctx, &Thought{Content: "Thought 2", People: []string{"Bob"}, Topics: []string{"life"}, Type: "decision", Source: "test"})

	var buf bytes.Buffer
	filter := ExportFilter{}
	err := brain.Export(ctx, &buf, "jsonl", filter)

	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Thought 1") || !strings.Contains(output, "Thought 2") {
		t.Error("Export should contain both thoughts")
	}
}

func TestJSONLExporter(t *testing.T) {
	thoughts := []Thought{
		{ID: "1", Content: "Test", People: []string{"Alice"}, Topics: []string{"work"}, Type: "insight", Source: "test"},
	}

	var buf bytes.Buffer
	exporter := &JSONLExporter{}
	err := exporter.Export(thoughts, &buf)

	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var decoded Thought
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Content != "Test" {
		t.Errorf("expected content 'Test', got %s", decoded.Content)
	}
}

func TestMarkdownExporter(t *testing.T) {
	thoughts := []Thought{
		{ID: "1", Content: "Test thought", People: []string{"Alice"}, Topics: []string{"work"}, Type: "insight", Source: "test", CreatedAt: time.Now()},
	}

	var buf bytes.Buffer
	exporter := &MarkdownExporter{}
	err := exporter.Export(thoughts, &buf)

	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "# Picobrain Export") {
		t.Error("Markdown should contain header")
	}
	if !strings.Contains(output, "Test thought") {
		t.Error("Markdown should contain thought content")
	}
}

func TestCSVExporter(t *testing.T) {
	thoughts := []Thought{
		{ID: "1", Content: "Test", People: []string{"Alice", "Bob"}, Topics: []string{"work"}, Type: "insight", Source: "test", CreatedAt: time.Now()},
	}

	var buf bytes.Buffer
	exporter := &CSVExporter{}
	err := exporter.Export(thoughts, &buf)

	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Read CSV: %v", err)
	}

	if len(records) != 2 { // header + 1 row
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestExportWithFilters(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store thoughts with different attributes
	brain.Store(ctx, &Thought{Content: "Work thought", Topics: []string{"work"}, Type: "insight", Source: "slack"})
	brain.Store(ctx, &Thought{Content: "Personal thought", Topics: []string{"life"}, Type: "decision", Source: "cli"})

	// Test type filter
	var buf bytes.Buffer
	filter := ExportFilter{Type: "insight"}
	err := brain.Export(ctx, &buf, "jsonl", filter)
	if err != nil {
		t.Fatalf("Export with type filter: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Work thought") {
		t.Error("Should contain work thought")
	}
	if strings.Contains(output, "Personal thought") {
		t.Error("Should not contain personal thought")
	}
}

func TestImportExportedJSONL(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Store and export
	brain.Store(ctx, &Thought{Content: "Test thought", People: []string{"Alice"}, Topics: []string{"work"}})

	var exportBuf bytes.Buffer
	filter := ExportFilter{}
	brain.Export(ctx, &exportBuf, "jsonl", filter)

	// Create new brain and import
	brain2 := testBrain(t)
	count, err := brain2.Import(ctx, &exportBuf, "jsonl")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 imported, got %d", count)
	}

	// Verify
	stats, _ := brain2.Stats(ctx)
	if stats.TotalThoughts != 1 {
		t.Errorf("expected 1 thought after import, got %d", stats.TotalThoughts)
	}
}

func TestExportWithEdgesRespectsThoughtFilter(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	inA := &Thought{Content: "A", Type: "insight"}
	inB := &Thought{Content: "B", Type: "insight"}
	outC := &Thought{Content: "C", Type: "note"}
	for _, th := range []*Thought{inA, inB, outC} {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	if err := brain.CreateEdge(ctx, &Edge{SourceID: inA.ID, TargetID: inB.ID, RelationType: "keeps"}); err != nil {
		t.Fatalf("CreateEdge(in-scope): %v", err)
	}
	if err := brain.CreateEdge(ctx, &Edge{SourceID: inA.ID, TargetID: outC.ID, RelationType: "cross"}); err != nil {
		t.Fatalf("CreateEdge(cross-scope): %v", err)
	}

	var buf bytes.Buffer
	filter := ExportFilter{Type: "insight", IncludeEdges: true}
	if err := brain.Export(ctx, &buf, "jsonl", filter); err != nil {
		t.Fatalf("Export: %v", err)
	}

	for i, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("line %d unmarshal: %v", i+1, err)
		}
		if typ, _ := raw["_type"].(string); typ == "edge" {
			t.Fatalf("canonical JSONL must not export edge entries: %s", line)
		}
	}
}

func TestExportImportJSONLWithEdgesRoundTrip(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Root"}
	t2 := &Thought{Content: "Leaf"}
	if err := brain.Store(ctx, t1); err != nil {
		t.Fatalf("Store t1: %v", err)
	}
	if err := brain.Store(ctx, t2); err != nil {
		t.Fatalf("Store t2: %v", err)
	}
	if err := brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "relates_to", Weight: 0.7}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	var out bytes.Buffer
	if err := brain.Export(ctx, &out, "jsonl", ExportFilter{IncludeEdges: true}); err != nil {
		t.Fatalf("Export with edges: %v", err)
	}

	if strings.Contains(out.String(), `"_type":"edge"`) {
		t.Fatalf("canonical JSONL should not contain edge entries")
	}

	brain2 := testBrain(t)
	importedThoughts, err := brain2.Import(ctx, bytes.NewReader(out.Bytes()), "jsonl")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if importedThoughts != 2 {
		t.Fatalf("expected 2 imported thoughts, got %d", importedThoughts)
	}

	stats, err := brain2.GraphStats(ctx)
	if err != nil {
		t.Fatalf("GraphStats: %v", err)
	}
	if stats.TotalEdges != 0 {
		t.Fatalf("expected 0 imported edges, got %d", stats.TotalEdges)
	}
}
