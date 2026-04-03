package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/asabya/picobrain"
)

type importCommand struct {
	fs             *flag.FlagSet
	format         string
	input          string
	dbPath         string
	embedModel     string
	modelCache     string
	noAutoDownload bool
}

func newImportCommand() *importCommand {
	defaults := picobrain.DefaultConfig()
	cmd := &importCommand{
		fs: flag.NewFlagSet("import", flag.ExitOnError),
	}
	cmd.fs.StringVar(&cmd.dbPath, "db", defaults.DBPath, "path to brain database")
	cmd.fs.StringVar(&cmd.embedModel, "embed-model", defaults.EmbedModel, "embedding model name")
	cmd.fs.StringVar(&cmd.modelCache, "model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	cmd.fs.BoolVar(&cmd.noAutoDownload, "no-auto-download", false, "disable automatic model download")
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

func (c *importCommand) Run() error {
	if c.input == "" {
		return fmt.Errorf("--input is required")
	}

	if c.format != "jsonl" && c.format != "csv" {
		return fmt.Errorf("unsupported format: %s (use jsonl or csv)", c.format)
	}

	cfg := picobrain.Config{
		DBPath:        c.dbPath,
		EmbedModel:    c.embedModel,
		ModelCacheDir: c.modelCache,
		AutoDownload:  !c.noAutoDownload,
	}

	f, err := os.Open(c.input)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer f.Close()

	brain, err := picobrain.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize brain: %w", err)
	}
	defer brain.Close()

	ctx := context.Background()
	count, err := brain.Import(ctx, f, c.format)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Imported %d thoughts\n", count)
	return nil
}
