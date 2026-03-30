package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/asabya/picobrain"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	defaults := picobrain.DefaultConfig()

	dbPath := flag.String("db", defaults.DBPath, "path to brain database")
	embedModel := flag.String("embed-model", defaults.EmbedModel, "embedding model name (e.g. nomic-embed-text-v1.5)")
	modelCache := flag.String("model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	noAutoDownload := flag.Bool("no-auto-download", false, "disable automatic model download (fail if model not cached)")
	port := flag.String("port", "8080", "HTTP listen port")
	flag.Parse()

	cfg := picobrain.Config{
		DBPath:        *dbPath,
		EmbedModel:    *embedModel,
		ModelCacheDir: *modelCache,
		AutoDownload:  !*noAutoDownload,
	}

	brain, err := picobrain.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize brain: %v\n", err)
		os.Exit(1)
	}
	defer brain.Close()

	s := server.NewMCPServer("picobrain", "0.1.0")
	picobrain.RegisterMCPTools(s, brain)

	httpServer := server.NewStreamableHTTPServer(s)
	if err := httpServer.Start(":" + *port); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
