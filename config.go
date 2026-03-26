package picobrain

import (
	"os"
	"path/filepath"
)

type Config struct {
	DBPath     string
	OllamaURL  string
	EmbedModel string
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		DBPath:     filepath.Join(home, ".picobrain", "brain.db"),
		OllamaURL:  "http://localhost:11434",
		EmbedModel: "nomic-embed-text",
	}
}
