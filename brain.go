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
	embedder *OllamaEmbedder
	config   Config
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

	embedder := NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbedModel)

	return &Brain{db: db, embedder: embedder, config: cfg}, nil
}

func (b *Brain) Close() error {
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

	return insertThought(b.db, t)
}

func (b *Brain) Search(ctx context.Context, query string, limit int) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return searchByVector(b.db, queryEmb, limit)
}

func (b *Brain) ListRecent(ctx context.Context, since time.Time, limit int) ([]Thought, error) {
	if limit <= 0 {
		limit = 20
	}
	return listRecent(b.db, since, limit)
}

func (b *Brain) Stats(ctx context.Context) (*BrainStats, error) {
	return getStats(b.db)
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
