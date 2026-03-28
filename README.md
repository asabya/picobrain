# picobrain

`picobrain` is a local semantic memory system for AI agents.

It stores short facts, decisions, meeting notes, and working context in SQLite with vector search via `sqlite-vec`, and uses Ollama to generate embeddings. You can use it either as:

- an MCP server for agent tools
- a Go library inside your own application

## What It Does

`picobrain` gives an agent a single local "brain" it can write to and search later.

Typical use cases:

- remember decisions made across sessions
- store people, topics, and action items alongside notes
- search by meaning instead of exact keywords
- import existing memory dumps from JSONL
- expose memory to any MCP-compatible client over stdio

## Quickstart For AI Agents

### 1. Start Ollama

`picobrain` expects a local Ollama server and defaults to `nomic-embed-text`.

```bash
ollama pull nomic-embed-text
ollama serve
```

### 2. Build The MCP Server

```bash
go build -o picobrain-mcp ./cmd/picobrain-mcp
```

### 3. Run It

```bash
./picobrain-mcp --db ~/.picobrain/brain.db
```

Available flags:

- `--db`: SQLite database path
- `--ollama-url`: Ollama base URL, default `http://localhost:11434`
- `--embed-model`: embedding model, default `nomic-embed-text`

### 4. Verify MCP Tool Registration

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./picobrain-mcp --db /tmp/test-brain.db
```

You should see 5 tools:

- `store_thought`
- `semantic_search`
- `list_recent`
- `stats`
- `bulk_import`

## MCP Client Configuration

Example stdio MCP config:

```json
{
  "servers": {
    "picobrain": {
      "type": "stdio",
      "command": "/absolute/path/to/picobrain-mcp",
      "args": ["--db", "/Users/you/.picobrain/brain.db"]
    }
  }
}
```

If your MCP client supports environment variables or working directories, keep the command absolute and the DB path explicit. Agents are more reliable when setup is deterministic.

## How Agents Should Use It

Use `picobrain` as durable working memory, not as a raw log sink.

Good patterns:

- store concise, reusable facts instead of full transcripts
- include `people`, `topics`, `type`, and `action_items` whenever available
- store decisions and constraints immediately after they are discovered
- query semantically before asking the human to repeat context
- import historical notes once, then add only distilled new thoughts

Avoid:

- dumping every token of a conversation
- storing secrets unless you explicitly trust the local machine and database
- using semantic search for exact structured lookups that should be modeled elsewhere

Recommended workflow for agents:

1. On important new information, call `store_thought`.
2. Before planning or answering, call `semantic_search` for related context.
3. Use `list_recent` to rebuild short-term context after restarts.
4. Use `stats` for high-level memory inspection.
5. Use `bulk_import` only for one-time or batched backfills.

## MCP Tools

### `store_thought`

Stores a thought and generates an embedding automatically.

Input:

```json
{
  "content": "Sarah is considering leaving her job for consulting.",
  "people": ["Sarah"],
  "topics": ["career", "consulting"],
  "type": "person_note",
  "action_items": ["Follow up next week"],
  "source": "slack"
}
```

Returns a JSON string with the stored thought ID.

### `semantic_search`

Searches thoughts by meaning.

Input:

```json
{
  "query": "What do I know about Sarah and consulting?",
  "limit": 5
}
```

Returns a JSON array of matching thoughts ordered by ascending distance.

### `list_recent`

Lists recent thoughts ordered newest-first.

Input:

```json
{
  "since": "2026-03-01T00:00:00Z",
  "limit": 20
}
```

If `since` is omitted, it defaults to the last 7 days.

### `stats`

Returns aggregate memory stats:

- total thoughts
- thoughts this week
- top topics
- top sources
- first thought timestamp
- last thought timestamp
- average thoughts per day

### `bulk_import`

Imports newline-delimited JSON objects and generates embeddings for each line.

Input:

```json
{
  "jsonl": "{\"content\":\"met Alice\",\"source\":\"import\"}\n{\"content\":\"API moved to GraphQL\",\"topics\":[\"engineering\"],\"source\":\"import\"}"
}
```

## Go Library Usage

You can also use `picobrain` directly from Go.

### Install

```bash
go get github.com/asabya/picobrain
```

### Minimal Example

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/asabya/picobrain"
)

func main() {
	cfg := picobrain.DefaultConfig()

	brain, err := picobrain.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer brain.Close()

	ctx := context.Background()

	err = brain.Store(ctx, &picobrain.Thought{
		Content: "Alice is leading the frontend redesign.",
		People:  []string{"Alice"},
		Topics:  []string{"frontend", "design"},
		Type:    "person_note",
		Source:  "app",
	})
	if err != nil {
		log.Fatal(err)
	}

	results, err := brain.Search(ctx, "Who is working on frontend design?", 3)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("results: %+v\n", results)

	recent, err := brain.ListRecent(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("recent: %d\n", len(recent))
}
```

### Public API

Main types:

- `Config`
- `Thought`
- `BrainStats`
- `Brain`

Main methods on `Brain`:

- `New(cfg Config) (*Brain, error)`
- `Close() error`
- `Store(ctx context.Context, t *Thought) error`
- `Search(ctx context.Context, query string, limit int) ([]Thought, error)`
- `ListRecent(ctx context.Context, since time.Time, limit int) ([]Thought, error)`
- `Stats(ctx context.Context) (*BrainStats, error)`
- `BulkImport(ctx context.Context, r io.Reader) (int, error)`

## Data Model

Each stored thought contains:

- `id`
- `content`
- `people`
- `topics`
- `type`
- `action_items`
- `source`
- `created_at`
- `distance` on search results

Embeddings are stored internally and are not exposed in JSON output.

## Operational Notes

- The database defaults to `~/.picobrain/brain.db`.
- Non-memory databases enable SQLite WAL mode.
- Embeddings are generated through Ollama on each store/search/import operation unless a precomputed embedding is already present on the `Thought`.
- This project uses CGo through `sqlite-vec` and `go-sqlite3`.
- On macOS, you may see deprecation warnings from SQLite auto-extension APIs during build or test. Those warnings do not by themselves indicate failure.

## Development

Run tests:

```bash
go test ./...
```

Build the MCP binary:

```bash
go build -o picobrain-mcp ./cmd/picobrain-mcp
```
