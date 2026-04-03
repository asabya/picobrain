package picobrain

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/google/uuid"
)

type Brain struct {
	db       *sql.DB
	embedder Embedder
	config   Config
	cache    *ThoughtCache
}

func New(cfg Config) (*Brain, error) {
	sqlite_vec.Auto()

	if cfg.DBPath != ":memory:" {
		dir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.DBPath != ":memory:" {
		db.Exec("PRAGMA journal_mode=WAL")
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	embedder, err := NewLocalEmbedder(cfg.EmbedModel, cfg.ModelCacheDir, cfg.AutoDownload)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	return &Brain{
		db:       db,
		embedder: embedder,
		config:   cfg,
		cache:    NewThoughtCache(cfg.CacheSize),
	}, nil
}

// NewWithEmbedder creates a Brain with a provided embedder.
// This is useful for testing with mock embedders.
func NewWithEmbedder(cfg Config, emb Embedder) (*Brain, error) {
	sqlite_vec.Auto()

	if cfg.DBPath != ":memory:" {
		dir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.DBPath != ":memory:" {
		db.Exec("PRAGMA journal_mode=WAL")
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &Brain{
		db:       db,
		embedder: emb,
		config:   cfg,
		cache:    NewThoughtCache(cfg.CacheSize),
	}, nil
}

func (b *Brain) Close() error {
	if b.embedder != nil {
		b.embedder.Close()
	}
	return b.db.Close()
}

func (b *Brain) Store(ctx context.Context, t *Thought) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	if t.Embedding == nil {
		emb, err := b.embedder.Embed(ctx, t.Content)
		if err != nil {
			return fmt.Errorf("generate embedding: %w", err)
		}
		t.Embedding = emb
	}

	if err := insertThought(b.db, t); err != nil {
		return err
	}

	b.cache.Put(*t)
	return nil
}

func (b *Brain) Search(ctx context.Context, query string, limit int, thoughtType string) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return searchByVector(b.db, queryEmb, limit, thoughtType)
}

func (b *Brain) SearchWithFilters(ctx context.Context, query string, limit int, filters SearchFilters) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return searchByVectorWithFilters(b.db, queryEmb, limit, filters)
}

func (b *Brain) ListRecent(ctx context.Context, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	if limit <= 0 {
		limit = 20
	}

	thoughts, err := listRecent(b.db, since, limit, thoughtType)
	if err != nil {
		return nil, err
	}

	for i := range thoughts {
		b.cache.Put(thoughts[i])
	}

	return thoughts, nil
}

func (b *Brain) Stats(ctx context.Context) (*BrainStats, error) {
	return getStats(b.db)
}

func (b *Brain) Get(ctx context.Context, id string) (*Thought, error) {
	if thought, found := b.cache.Get(id); found {
		return &thought, nil
	}

	thought, err := getThought(b.db, id)
	if err != nil {
		return nil, err
	}

	b.cache.Put(*thought)
	return thought, nil
}

func (b *Brain) GetRecent(limit int) []Thought {
	if limit <= 0 {
		limit = 20
	}
	return b.cache.GetRecent(limit)
}

func (b *Brain) Delete(ctx context.Context, id string) error {
	if err := deleteThought(b.db, id); err != nil {
		return err
	}
	b.cache.Remove(id)
	return nil
}

type ReflectResult struct {
	Stored  []string `json:"stored"`
	Deleted []string `json:"deleted"`
}

func (b *Brain) Reflect(ctx context.Context, deleteIDs []string, newThoughts []*Thought) (*ReflectResult, error) {
	for _, t := range newThoughts {
		if t.ID == "" {
			t.ID = uuid.New().String()
		}
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		if t.Embedding == nil {
			emb, err := b.embedder.Embed(ctx, t.Content)
			if err != nil {
				return nil, fmt.Errorf("generate embedding: %w", err)
			}
			t.Embedding = emb
		}
	}

	if err := reflectTx(b.db, deleteIDs, newThoughts); err != nil {
		return nil, err
	}

	for _, id := range deleteIDs {
		b.cache.Remove(id)
	}
	for _, t := range newThoughts {
		b.cache.Put(*t)
	}

	stored := make([]string, len(newThoughts))
	for i, t := range newThoughts {
		stored[i] = t.ID
	}

	return &ReflectResult{Stored: stored, Deleted: deleteIDs}, nil
}

func (b *Brain) BulkImport(ctx context.Context, r io.Reader) (int, error) {
	scanner := bufio.NewScanner(r)
	count := 0

	tx, err := b.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var t Thought
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return count, fmt.Errorf("parse line %d: %w", count+1, err)
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

	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("read input: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return count, nil
}

// Prune deletes thoughts older than the specified number of days,
// excluding critical priority thoughts. Returns the number of thoughts deleted.
func (b *Brain) Prune(ctx context.Context, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}

	deleted, err := pruneOldThoughts(b.db, days)
	if err != nil {
		return 0, fmt.Errorf("prune thoughts: %w", err)
	}

	if deleted > 0 {
		b.cache.Clear()
	}

	return deleted, nil
}
