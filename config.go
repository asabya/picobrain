package picobrain

import (
	"os"
	"path/filepath"
)

type Config struct {
	DBPath        string
	EmbedModel    string
	ModelCacheDir string
	AutoDownload  bool
	CacheSize     int
	AutoPruneDays int
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		DBPath:        filepath.Join(home, ".picobrain", "brain.db"),
		EmbedModel:    "nomic-embed-text-v1.5",
		ModelCacheDir: filepath.Join(home, ".picobrain", "models"),
		AutoDownload:  true,
		AutoPruneDays: 30,
	}
}
