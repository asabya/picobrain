# Lightweight Export/Import Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement lightweight export (Markdown/CSV) and import functionality for picobrain with CLI support and filters.

**Architecture:** 
- Add `Export` method to Brain with filter support
- Create `Exporter` interface with `MarkdownExporter` and `CSVExporter` implementations  
- Extend CLI with subcommand support (export/import commands)
- Export formats include all thought metadata
- Import can read exported formats back

**Tech Stack:** Go standard library (encoding/csv, text/template), existing picobrain patterns

---

## Task 1: Write Export Tests (TDD - Red Phase)

**Files:**
- Create: `export_test.go`

**Step 1: Write failing test for Export method**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBrainExport -v ./...`
Expected: FAIL - "brain.Export undefined"

**Step 3: Commit**

```bash
git add export_test.go
git commit -m "test: add export test (TDD red phase)"
```

---

## Task 2: Implement Export Core Functionality (Green Phase)

**Files:**
- Create: `export.go`
- Modify: `brain.go` (add Export method)

**Step 1: Create export.go with core types**

```go
package picobrain

import (
	"context"
	"fmt"
	"io"
	"time"
)

type ExportFilter struct {
	Since      *time.Time
	Until      *time.Time
	Type       string
	Topics     []string
	People     []string
	Source     string
}

type Exporter interface {
	Export(thoughts []Thought, w io.Writer) error
}
```

**Step 2: Add Export method to Brain**

In `brain.go`, add:
```go
func (b *Brain) Export(ctx context.Context, w io.Writer, format string, filter ExportFilter) error {
	thoughts, err := b.queryForExport(filter)
	if err != nil {
		return fmt.Errorf("query thoughts: %w", err)
	}

	var exporter Exporter
	switch format {
	case "jsonl":
		exporter = &JSONLExporter{}
	case "markdown":
		exporter = &MarkdownExporter{}
	case "csv":
		exporter = &CSVExporter{}
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}

	return exporter.Export(thoughts, w)
}

func (b *Brain) queryForExport(filter ExportFilter) ([]Thought, error) {
	// Query all thoughts with filter
	return queryThoughtsWithFilter(b.db, filter)
}
```

**Step 3: Run test to verify it passes**

Run: `go test -run TestBrainExport -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add export.go brain.go
git commit -m "feat: add Export method with filter support (TDD green phase)"
```

---

## Task 3: Implement JSONL Exporter

**Files:**
- Modify: `export.go`

**Step 1: Add JSONLExporter**

```go
type JSONLExporter struct{}

func (e *JSONLExporter) Export(thoughts []Thought, w io.Writer) error {
	encoder := json.NewEncoder(w)
	for _, t := range thoughts {
		// Clear embedding for export
		t.Embedding = nil
		if err := encoder.Encode(t); err != nil {
			return fmt.Errorf("encode thought: %w", err)
		}
	}
	return nil
}
```

**Step 2: Add test**

```go
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
```

**Step 3: Run tests**

Run: `go test -run TestJSONLExporter -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add export.go export_test.go
git commit -m "feat: add JSONL exporter"
```

---

## Task 4: Implement Markdown Exporter

**Files:**
- Modify: `export.go`
- Modify: `export_test.go`

**Step 1: Add MarkdownExporter**

```go
type MarkdownExporter struct{}

func (e *MarkdownExporter) Export(thoughts []Thought, w io.Writer) error {
	fmt.Fprintf(w, "# Picobrain Export\n\n")
	fmt.Fprintf(w, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Total thoughts: %d\n\n", len(thoughts))
	fmt.Fprintf(w, "---\n\n")

	for i, t := range thoughts {
		fmt.Fprintf(w, "## Thought %d\n\n", i+1)
		fmt.Fprintf(w, "**ID:** %s\n\n", t.ID)
		fmt.Fprintf(w, "**Content:**\n%s\n\n", t.Content)
		
		if t.Type != "" {
			fmt.Fprintf(w, "**Type:** %s\n\n", t.Type)
		}
		if len(t.People) > 0 {
			fmt.Fprintf(w, "**People:** %s\n\n", strings.Join(t.People, ", "))
		}
		if len(t.Topics) > 0 {
			fmt.Fprintf(w, "**Topics:** %s\n\n", strings.Join(t.Topics, ", "))
		}
		if len(t.ActionItems) > 0 {
			fmt.Fprintf(w, "**Action Items:**\n")
			for _, item := range t.ActionItems {
				fmt.Fprintf(w, "- %s\n", item)
			}
			fmt.Fprintln(w)
		}
		if t.Source != "" {
			fmt.Fprintf(w, "**Source:** %s\n\n", t.Source)
		}
		fmt.Fprintf(w, "**Created:** %s\n\n", t.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "---\n\n")
	}
	
	return nil
}
```

**Step 2: Add test**

```go
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
```

**Step 3: Run tests**

Run: `go test -run TestMarkdownExporter -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add export.go export_test.go
git commit -m "feat: add Markdown exporter"
```

---

## Task 5: Implement CSV Exporter

**Files:**
- Modify: `export.go`
- Modify: `export_test.go`

**Step 1: Add CSVExporter**

```go
type CSVExporter struct{}

func (e *CSVExporter) Export(thoughts []Thought, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{"id", "content", "type", "people", "topics", "action_items", "source", "created_at"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write rows
	for _, t := range thoughts {
		row := []string{
			t.ID,
			t.Content,
			t.Type,
			strings.Join(t.People, "|"),
			strings.Join(t.Topics, "|"),
			strings.Join(t.ActionItems, "|"),
			t.Source,
			t.CreatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}

	return nil
}
```

**Step 2: Add test**

```go
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
```

**Step 3: Run tests**

Run: `go test -run TestCSVExporter -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add export.go export_test.go
git commit -m "feat: add CSV exporter"
```

---

## Task 6: Implement Query with Filters in Store

**Files:**
- Modify: `store.go`

**Step 1: Add queryThoughtsWithFilter function**

```go
func queryThoughtsWithFilter(db *sql.DB, filter ExportFilter) ([]Thought, error) {
	query := `
		SELECT id, content, people, topics, type, action_items, source, created_at
		FROM thoughts
		WHERE 1=1
	`
	args := []any{}
	
	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.Since.Format("2006-01-02 15:04:05"))
	}
	if filter.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, filter.Until.Format("2006-01-02 15:04:05"))
	}
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Source != "" {
		query += " AND source = ?"
		args = append(args, filter.Source)
	}
	if len(filter.Topics) > 0 {
		// Use json_each to check if any topic matches
		placeholders := make([]string, len(filter.Topics))
		for i := range filter.Topics {
			placeholders[i] = "?"
			args = append(args, filter.Topics[i])
		}
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(topics) WHERE value IN (%s))", strings.Join(placeholders, ","))
	}
	if len(filter.People) > 0 {
		placeholders := make([]string, len(filter.People))
		for i := range filter.People {
			placeholders[i] = "?"
			args = append(args, filter.People[i])
		}
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(people) WHERE value IN (%s))", strings.Join(placeholders, ","))
	}
	
	query += " ORDER BY created_at DESC"
	
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query thoughts: %w", err)
	}
	defer rows.Close()
	
	return scanThoughts(rows, false)
}
```

**Step 2: Add test for filters**

```go
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
```

**Step 3: Run tests**

Run: `go test -run TestExportWithFilters -v ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add store.go export_test.go
git commit -m "feat: add queryThoughtsWithFilter for export filtering"
```

---

## Task 7: Create CLI Export Command

**Files:**
- Create: `cmd/picobrain-mcp/export.go`
- Modify: `cmd/picobrain-mcp/main.go`

**Step 1: Create export command file**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/asabya/picobrain"
)

type exportCommand struct {
	fs     *flag.FlagSet
	format string
	output string
	since  string
	until  string
	typ    string
	topics string
	people string
	source string
}

func newExportCommand() *exportCommand {
	cmd := &exportCommand{
		fs: flag.NewFlagSet("export", flag.ExitOnError),
	}
	cmd.fs.StringVar(&cmd.format, "format", "jsonl", "Export format: jsonl, markdown, csv")
	cmd.fs.StringVar(&cmd.output, "output", "", "Output file (default: stdout)")
	cmd.fs.StringVar(&cmd.since, "since", "", "Export thoughts created after this date (YYYY-MM-DD)")
	cmd.fs.StringVar(&cmd.until, "until", "", "Export thoughts created before this date (YYYY-MM-DD)")
	cmd.fs.StringVar(&cmd.typ, "type", "", "Filter by thought type")
	cmd.fs.StringVar(&cmd.topics, "topics", "", "Filter by topics (comma-separated)")
	cmd.fs.StringVar(&cmd.people, "people", "", "Filter by people (comma-separated)")
	cmd.fs.StringVar(&cmd.source, "source", "", "Filter by source")
	return cmd
}

func (c *exportCommand) Name() string {
	return "export"
}

func (c *exportCommand) Parse(args []string) error {
	return c.fs.Parse(args)
}

func (c *exportCommand) Run(cfg picobrain.Config) error {
	// Validate format
	if c.format != "jsonl" && c.format != "markdown" && c.format != "csv" {
		return fmt.Errorf("unsupported format: %s (use jsonl, markdown, or csv)", c.format)
	}

	// Initialize brain
	brain, err := picobrain.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize brain: %w", err)
	}
	defer brain.Close()

	// Build filter
	filter := picobrain.ExportFilter{}
	if c.since != "" {
		t, err := time.Parse("2006-01-02", c.since)
		if err != nil {
			return fmt.Errorf("parse since date: %w", err)
		}
		filter.Since = &t
	}
	if c.until != "" {
		t, err := time.Parse("2006-01-02", c.until)
		if err != nil {
			return fmt.Errorf("parse until date: %w", err)
		}
		filter.Until = &t
	}
	if c.typ != "" {
		filter.Type = c.typ
	}
	if c.topics != "" {
		filter.Topics = strings.Split(c.topics, ",")
	}
	if c.people != "" {
		filter.People = strings.Split(c.people, ",")
	}
	if c.source != "" {
		filter.Source = c.source
	}

	// Open output
	var w *os.File
	if c.output == "" {
		w = os.Stdout
	} else {
		f, err := os.Create(c.output)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	// Export
	ctx := context.Background()
	if err := brain.Export(ctx, w, c.format, filter); err != nil {
		return fmt.Errorf("export: %w", err)
	}

	if c.output != "" {
		fmt.Fprintf(os.Stderr, "Exported to %s\n", c.output)
	}
	return nil
}
```

**Step 2: Modify main.go to support subcommands**

Replace main() with:
```go
func main() {
	if len(os.Args) < 2 {
		// No subcommand, run MCP server
		runServer(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "export":
		runExport(os.Args[2:])
	case "import":
		runImport(os.Args[2:])
	case "serve", "server":
		runServer(os.Args[2:])
	default:
		// Check if it's a flag (starts with -)
		if strings.HasPrefix(os.Args[1], "-") {
			runServer(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			fmt.Fprintf(os.Stderr, "Usage: picobrain [command] [options]\n")
			fmt.Fprintf(os.Stderr, "Commands: serve, export, import\n")
			os.Exit(1)
		}
	}
}

func runServer(args []string) {
	// Move current main() logic here
	defaults := picobrain.DefaultConfig()
	
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", defaults.DBPath, "path to brain database")
	embedModel := fs.String("embed-model", defaults.EmbedModel, "embedding model name")
	modelCache := fs.String("model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	noAutoDownload := fs.Bool("no-auto-download", false, "disable automatic model download")
	port := fs.String("port", "8080", "HTTP listen port")
	fs.Parse(args)
	
	cfg := picobrain.Config{
		DBPath:        *dbPath,
		EmbedModel:    *embedModel,
		ModelCacheDir: *modelCache,
		AutoDownload:  !*noAutoDownload,
	}
	
	// ... rest of server initialization
}

func runExport(args []string) {
	defaults := picobrain.DefaultConfig()
	cmd := newExportCommand()
	if err := cmd.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Run(defaults); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Build and test CLI**

Run: `go build -o picobrain ./cmd/picobrain-mcp && ./picobrain export --help`
Expected: Shows export usage

**Step 4: Commit**

```bash
git add cmd/picobrain-mcp/export.go cmd/picobrain-mcp/main.go
git commit -m "feat: add CLI export command with filters"
```

---

## Task 8: Write Import Tests for Exported Formats

**Files:**
- Modify: `export_test.go` (add import tests)

**Step 1: Write test for importing exported JSONL**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestImportExportedJSONL -v ./...`
Expected: FAIL - "brain.Import undefined"

**Step 3: Commit**

```bash
git add export_test.go
git commit -m "test: add import test for exported formats (TDD red phase)"
```

---

## Task 9: Implement Import Method

**Files:**
- Modify: `export.go` (add Import method)
- Modify: `brain.go` (add Import wrapper)

**Step 1: Add Import method to export.go**

```go
func (b *Brain) Import(ctx context.Context, r io.Reader, format string) (int, error) {
	switch format {
	case "jsonl":
		return b.importJSONL(ctx, r)
	case "csv":
		return b.importCSV(ctx, r)
	default:
		return 0, fmt.Errorf("unsupported import format: %s", format)
	}
}

func (b *Brain) importJSONL(ctx context.Context, r io.Reader) (int, error) {
	// Reuse BulkImport logic
	return b.BulkImport(ctx, r)
}

func (b *Brain) importCSV(ctx context.Context, r io.Reader) (int, error) {
	reader := csv.NewReader(r)
	
	// Read header
	_, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("read CSV header: %w", err)
	}

	count := 0
	tx, err := b.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("read CSV row: %w", err)
		}

		// Parse record
		t := Thought{
			ID:      record[0],
			Content: record[1],
			Type:    record[2],
		}
		if record[3] != "" {
			t.People = strings.Split(record[3], "|")
		}
		if record[4] != "" {
			t.Topics = strings.Split(record[4], "|")
		}
		if record[5] != "" {
			t.ActionItems = strings.Split(record[5], "|")
		}
		t.Source = record[6]
		if record[7] != "" {
			t.CreatedAt, _ = time.Parse(time.RFC3339, record[7])
		}

		// Generate embedding and store
		if t.ID == "" {
			t.ID = uuid.New().String()
		}
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		
		emb, err := b.embedder.Embed(ctx, t.Content)
		if err != nil {
			return count, fmt.Errorf("embed thought %d: %w", count+1, err)
		}
		t.Embedding = emb

		if err := insertThoughtTx(tx, &t); err != nil {
			return count, fmt.Errorf("insert thought %d: %w", count+1, err)
		}

		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return count, nil
}
```

**Step 2: Run test to verify it passes**

Run: `go test -run TestImportExportedJSONL -v ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add export.go brain.go
git commit -m "feat: add Import method for JSONL and CSV (TDD green phase)"
```

---

## Task 10: Create CLI Import Command

**Files:**
- Create: `cmd/picobrain-mcp/import.go`
- Modify: `cmd/picobrain-mcp/main.go`

**Step 1: Create import command file**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/asabya/picobrain"
)

type importCommand struct {
	fs     *flag.FlagSet
	format string
	input  string
}

func newImportCommand() *importCommand {
	cmd := &importCommand{
		fs: flag.NewFlagSet("import", flag.ExitOnError),
	}
	cmd.fs.StringVar(&cmd.format, "format", "jsonl", "Import format: jsonl, csv")
	cmd.fs.StringVar(&cmd.input, "input", "", "Input file (required)")
	return cmd
}

func (c *importCommand) Name() string {
	return "import"
}

func (c *importCommand) Parse(args []string) error {
	return c.fs.Parse(args)
}

func (c *importCommand) Run(cfg picobrain.Config) error {
	if c.input == "" {
		return fmt.Errorf("--input is required")
	}

	// Validate format
	if c.format != "jsonl" && c.format != "csv" {
		return fmt.Errorf("unsupported format: %s (use jsonl or csv)", c.format)
	}

	// Open input file
	f, err := os.Open(c.input)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer f.Close()

	// Initialize brain
	brain, err := picobrain.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize brain: %w", err)
	}
	defer brain.Close()

	// Import
	ctx := context.Background()
	count, err := brain.Import(ctx, f, c.format)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Imported %d thoughts\n", count)
	return nil
}
```

**Step 2: Update main.go runImport function**

```go
func runImport(args []string) {
	defaults := picobrain.DefaultConfig()
	cmd := newImportCommand()
	if err := cmd.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Run(defaults); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Build and test**

Run: `go build -o picobrain ./cmd/picobrain-mcp && ./picobrain import --help`
Expected: Shows import usage

**Step 4: Commit**

```bash
git add cmd/picobrain-mcp/import.go cmd/picobrain-mcp/main.go
git commit -m "feat: add CLI import command"
```

---

## Task 11: Run All Tests and Verify

**Step 1: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Build and manual test**

```bash
go build -o picobrain ./cmd/picobrain-mcp

# Test export
./picobrain export --format markdown --output test.md
./picobrain export --format csv --output test.csv
./picobrain export --format jsonl --output test.jsonl

# Test import
./picobrain import --format jsonl --input test.jsonl
./picobrain import --format csv --input test.csv
```

**Step 3: Run linter**

Run: `go fmt ./... && go vet ./...`
Expected: No errors

**Step 4: Commit**

```bash
git add -A
git commit -m "test: verify all export/import functionality"
```

---

## Task 12: Final Review and Documentation

**Step 1: Update README with export/import commands**

Add to README.md:
```markdown
## CLI Commands

### Export

Export all thoughts to various formats:

```bash
# Export to Markdown
picobrain export --format markdown --output thoughts.md

# Export to CSV
picobrain export --format csv --output thoughts.csv

# Export to JSONL (re-importable)
picobrain export --format jsonl --output thoughts.jsonl

# Export with filters
picobrain export --format markdown --since 2024-01-01 --type insight --topics work,ai
```

### Import

Import thoughts from exported files:

```bash
# Import from JSONL
picobrain import --format jsonl --input thoughts.jsonl

# Import from CSV
picobrain import --format csv --input thoughts.csv
```
```

**Step 2: Final commit**

```bash
git add README.md
git commit -m "docs: add export/import documentation"
```

---

## Summary

This implementation adds:
- `Brain.Export()` method with filter support
- `Brain.Import()` method for JSONL and CSV
- `MarkdownExporter` and `CSVExporter` implementations
- CLI `export` command with format, output, and filter flags
- CLI `import` command with format and input flags
- Comprehensive tests following TDD

Filters supported:
- Date range (--since, --until)
- Type (--type)
- Topics (--topics comma-separated)
- People (--people comma-separated)
- Source (--source)
