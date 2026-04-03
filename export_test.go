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
