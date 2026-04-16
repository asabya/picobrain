package picobrain

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ExportFilter defines filters for exporting thoughts.
type ExportFilter struct {
	Since        *time.Time
	Until        *time.Time
	Type         string
	Topics       []string
	People       []string
	Source       string
	Namespace    string
	IncludeEdges bool
}

// Exporter defines the interface for exporting thoughts.
type Exporter interface {
	Export(thoughts []Thought, w io.Writer) error
}

// JSONLExporter exports thoughts in canonical JSONL format.
type JSONLExporter struct{}

func (e *JSONLExporter) Export(thoughts []Thought, w io.Writer) error {
	encoder := json.NewEncoder(w)
	for _, t := range thoughts {
		t.syncSummaryContent()
		t.Embedding = nil
		t.Distance = 0
		if err := encoder.Encode(t); err != nil {
			return fmt.Errorf("encode thought: %w", err)
		}
	}
	return nil
}

// MarkdownExporter exports thoughts in a lossy Markdown report format.
type MarkdownExporter struct{}

func (e *MarkdownExporter) Export(thoughts []Thought, w io.Writer) error {
	fmt.Fprintf(w, "# Picobrain Export\n\n")
	fmt.Fprintf(w, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Total thoughts: %d\n\n", len(thoughts))
	fmt.Fprintf(w, "---\n\n")
	for i, t := range thoughts {
		t.syncSummaryContent()
		fmt.Fprintf(w, "## Thought %d\n\n", i+1)
		fmt.Fprintf(w, "**ID:** %s\n\n", t.ID)
		fmt.Fprintf(w, "**Summary:**\n%s\n\n", t.Summary)
		if len(t.Claims) > 0 {
			fmt.Fprintf(w, "**Claims:**\n")
			for _, claim := range t.Claims {
				fmt.Fprintf(w, "- %s %s %s [%s/%s/%s]\n", claim.Subject, claim.Predicate, claim.Object, claim.Polarity, claim.Cardinality, claim.Status)
			}
			fmt.Fprintln(w)
		}
		if t.Type != "" {
			fmt.Fprintf(w, "**Type:** %s\n\n", t.Type)
		}
		if len(t.People) > 0 {
			fmt.Fprintf(w, "**People:** %s\n\n", strings.Join(t.People, ", "))
		}
		if len(t.Topics) > 0 {
			fmt.Fprintf(w, "**Topics:** %s\n\n", strings.Join(t.Topics, ", "))
		}
		if t.Source != "" {
			fmt.Fprintf(w, "**Source:** %s\n\n", t.Source)
		}
		fmt.Fprintf(w, "**Namespace:** %s\n\n", t.Namespace)
		fmt.Fprintf(w, "**Created:** %s\n\n", t.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "---\n\n")
	}
	return nil
}

// CSVExporter exports thoughts in a lossy CSV report format.
type CSVExporter struct{}

func (e *CSVExporter) Export(thoughts []Thought, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()
	header := []string{"id", "summary", "type", "people", "topics", "source", "namespace", "created_at", "claim_count"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, t := range thoughts {
		t.syncSummaryContent()
		row := []string{t.ID, t.Summary, t.Type, strings.Join(t.People, "|"), strings.Join(t.Topics, "|"), t.Source, t.Namespace, t.CreatedAt.Format(time.RFC3339), fmt.Sprintf("%d", len(t.Claims))}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return nil
}

func (b *Brain) Export(ctx context.Context, w io.Writer, format string, filter ExportFilter) error {
	thoughts, err := b.queryForExport(filter)
	if err != nil {
		return fmt.Errorf("query thoughts: %w", err)
	}
	switch format {
	case "jsonl":
		return (&JSONLExporter{}).Export(thoughts, w)
	case "markdown":
		return (&MarkdownExporter{}).Export(thoughts, w)
	case "csv":
		return (&CSVExporter{}).Export(thoughts, w)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

func (b *Brain) queryForExport(filter ExportFilter) ([]Thought, error) {
	return queryThoughtsWithFilter(b.db, filter)
}

func (b *Brain) Import(ctx context.Context, r io.Reader, format string) (int, error) {
	switch format {
	case "jsonl":
		results, err := b.BulkImportDetailed(ctx, r, "")
		if err != nil {
			return len(results), err
		}
		return len(results), nil
	case "csv":
		return 0, fmt.Errorf("unsupported import format: csv")
	default:
		return 0, fmt.Errorf("unsupported import format: %s", format)
	}
}
