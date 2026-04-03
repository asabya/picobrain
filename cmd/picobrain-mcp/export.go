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
	fs             *flag.FlagSet
	format         string
	output         string
	dbPath         string
	embedModel     string
	modelCache     string
	noAutoDownload bool
	since          string
	until          string
	typ            string
	topics         string
	people         string
	source         string
}

func newExportCommand() *exportCommand {
	defaults := picobrain.DefaultConfig()
	cmd := &exportCommand{
		fs: flag.NewFlagSet("export", flag.ExitOnError),
	}
	cmd.fs.StringVar(&cmd.dbPath, "db", defaults.DBPath, "path to brain database")
	cmd.fs.StringVar(&cmd.embedModel, "embed-model", defaults.EmbedModel, "embedding model name")
	cmd.fs.StringVar(&cmd.modelCache, "model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	cmd.fs.BoolVar(&cmd.noAutoDownload, "no-auto-download", false, "disable automatic model download")
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

func (c *exportCommand) Run() error {
	if c.format != "jsonl" && c.format != "markdown" && c.format != "csv" {
		return fmt.Errorf("unsupported format: %s (use jsonl, markdown, or csv)", c.format)
	}

	cfg := picobrain.Config{
		DBPath:        c.dbPath,
		EmbedModel:    c.embedModel,
		ModelCacheDir: c.modelCache,
		AutoDownload:  !c.noAutoDownload,
	}

	brain, err := picobrain.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize brain: %w", err)
	}
	defer brain.Close()

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

	ctx := context.Background()
	if err := brain.Export(ctx, w, c.format, filter); err != nil {
		return fmt.Errorf("export: %w", err)
	}

	if c.output != "" {
		fmt.Fprintf(os.Stderr, "Exported to %s\n", c.output)
	}
	return nil
}
