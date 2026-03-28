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
	ollamaURL := flag.String("ollama-url", defaults.OllamaURL, "Ollama API endpoint")
	embedModel := flag.String("embed-model", defaults.EmbedModel, "embedding model name")
	flag.Parse()

	cfg := picobrain.Config{
		DBPath:     *dbPath,
		OllamaURL:  *ollamaURL,
		EmbedModel: *embedModel,
	}

	brain, err := picobrain.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize brain: %v\n", err)
		os.Exit(1)
	}
	defer brain.Close()

	s := server.NewMCPServer("picobrain", "0.1.0")
	picobrain.RegisterMCPTools(s, brain)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
