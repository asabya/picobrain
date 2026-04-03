package picobrain

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExportFilter defines filters for exporting thoughts
type ExportFilter struct {
	Since  *time.Time
	Until  *time.Time
	Type   string
	Topics []string
	People []string
	Source string
}

// Exporter defines the interface for exporting thoughts
type Exporter interface {
	Export(thoughts []Thought, w io.Writer) error
}

// JSONLExporter exports thoughts in JSON Lines format
type JSONLExporter struct{}

func (e *JSONLExporter) Export(thoughts []Thought, w io.Writer) error {
	encoder := json.NewEncoder(w)
	for _, t := range thoughts {
		// Clear embedding for export
		t.Embedding = nil
		t.Distance = 0
		if err := encoder.Encode(t); err != nil {
			return fmt.Errorf("encode thought: %w", err)
		}
	}
	return nil
}

// MarkdownExporter exports thoughts in Markdown format
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

// CSVExporter exports thoughts in CSV format
type CSVExporter struct{}

func (e *CSVExporter) Export(thoughts []Thought, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{"id", "content", "type", "people", "topics", "action_items", "source", "created_at"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

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

// Export exports thoughts to the specified format with optional filters
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

// queryForExport retrieves thoughts from the database with optional filters
func (b *Brain) queryForExport(filter ExportFilter) ([]Thought, error) {
	return queryThoughtsWithFilter(b.db, filter)
}

// Import imports thoughts from the specified format
func (b *Brain) Import(ctx context.Context, r io.Reader, format string) (int, error) {
	switch format {
	case "jsonl":
		return b.BulkImport(ctx, r)
	case "csv":
		return b.importCSV(ctx, r)
	default:
		return 0, fmt.Errorf("unsupported import format: %s", format)
	}
}

// importCSV imports thoughts from CSV format
func (b *Brain) importCSV(ctx context.Context, r io.Reader) (int, error) {
	reader := csv.NewReader(r)

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
