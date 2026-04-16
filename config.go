package picobrain

import (
	"os"
	"path/filepath"
)

type Config struct {
	DBPath             string
	EmbedModel         string
	ModelCacheDir      string
	AutoDownload       bool
	CacheSize          int
	AutoPruneDays      int
	DefaultNamespace   string
	EnableAutoGraph    bool
	SpacyCacheDir      string
	AutoInstallSpacy  bool
	AutoGraphThreshold float64
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		DBPath:             filepath.Join(home, ".picobrain", "brain.db"),
		EmbedModel:         "nomic-embed-text-v1.5",
		ModelCacheDir:      filepath.Join(home, ".picobrain", "models"),
		AutoDownload:       true,
		AutoPruneDays:      30,
		DefaultNamespace:   "default",
		EnableAutoGraph:    false,
		SpacyCacheDir:      filepath.Join(home, ".picobrain", "spacy"),
		AutoInstallSpacy:   false,
		AutoGraphThreshold: 0.7,
	}
}
