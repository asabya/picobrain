# AGENTS.md

Guidelines for agentic coding agents working in this repository.

## Project Overview

picobrain is a local semantic memory service for AI agents. It stores thoughts, decisions, and context in SQLite with vector embeddings (nomic-embed-text-v1.5) and exposes memory operations via MCP HTTP.

- **Language**: Go 1.25.5
- **Architecture**: Library package + MCP HTTP server binary
- **Key Dependencies**: 
  - `github.com/mark3labs/mcp-go` - MCP protocol implementation
  - `github.com/asg017/sqlite-vec-go-bindings` - Vector search in SQLite
  - `github.com/mattn/go-sqlite3` - SQLite driver (CGO)
  - `github.com/google/uuid` - UUID generation

## Build Commands

```bash
# Build the MCP server binary
go build -o picobrain-mcp ./cmd/picobrain-mcp

# Build with CGO enabled (required for SQLite)
CGO_ENABLED=1 go build -o picobrain-mcp ./cmd/picobrain-mcp

# Cross-compile release (uses goreleaser-cross via Docker)
make release-dry-run
```

## Test Commands

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -run TestBrainStore -v ./...

# Run tests in a specific file
go test -run TestBrain -v ./brain_test.go

# Run tests with race detection
go test -race ./...

# Run tests in package directory
cd /path/to/dir && go test .
```

## Lint and Format Commands

```bash
# Format code
go fmt ./...

# Vet (static analysis)
go vet ./...

# Run both
go fmt ./... && go vet ./...
```

## Code Style Guidelines

### Imports

- Standard library imports first, separated by blank line
- Third-party imports second, separated by blank line
- Local imports last
- Use underscore imports for database drivers: `_ "github.com/mattn/go-sqlite3"`

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/mark3labs/mcp-go/server"
    "github.com/google/uuid"

    sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)
```

### Naming Conventions

- **Types**: PascalCase (`Brain`, `Thought`, `BrainStats`)
- **Interfaces**: PascalCase with `-er` suffix (`Embedder`, `dbExecer`)
- **Functions/Methods**: PascalCase if exported, camelCase if unexported
- **Constants**: camelCase for private, PascalCase for exported
- **Acronyms**: Keep consistent casing - `HTTPServer`, `toHTTPClient` (not `HttpServer`)

### Error Handling

- Wrap errors with context using `fmt.Errorf("operation description: %w", err)`
- Return errors early; avoid deeply nested if-else
- Use custom error variables for sentinel errors: `var errModelNotFound = errors.New("model not found")`
- Always close resources with `defer` after checking for errors

```go
func (b *Brain) Store(ctx context.Context, t *Thought) error {
    emb, err := b.embedder.Embed(ctx, t.Content)
    if err != nil {
        return fmt.Errorf("generate embedding: %w", err)
    }
    // ...
}
```

### Structs and Types

- Prefer small, focused structs
- Use `json` struct tags for API serialization
- Use `json:",omitempty"` for optional fields
- Use `json:"-"` for fields that should not be serialized (like `Embedding`)

```go
type Thought struct {
    ID          string    `json:"id,omitempty"`
    Content     string    `json:"content"`
    Embedding   []float32 `json:"-"`
    People      []string  `json:"people,omitempty"`
    CreatedAt   time.Time `json:"created_at,omitempty"`
}
```

### Functions

- Prefer pure functions where possible
- Use context.Context as first parameter for I/O operations
- Return errors as the last return value
- Use meaningful parameter names in function signatures

### Testing

- Place tests in same package (`package picobrain`)
- Use `t.Helper()` for test helper functions
- Create test helpers with descriptive names: `testBrain(t *testing.T)`
- Use table-driven tests for multiple cases
- Test error paths, not just happy paths
- Mock external dependencies (see `mockEmbedder` in `brain_test.go`)

```go
func testBrain(t *testing.T) *Brain {
    t.Helper()
    cfg := Config{
        DBPath:       ":memory:",
        EmbedModel:   "mock",
        AutoDownload: false,
    }
    brain, err := NewWithEmbedder(cfg, &mockEmbedder{dim: 768})
    if err != nil {
        t.Fatalf("New brain: %v", err)
    }
    t.Cleanup(func() { brain.Close() })
    return brain
}
```

### Database Patterns

- Use transactions for multi-statement operations
- Always defer `tx.Rollback()` after `db.Begin()`
- Use prepared statements for repeated queries
- Store JSON arrays as TEXT with `json.Marshal`/`Unmarshal`

### MCP Tool Handlers

- Return `(*mcp.CallToolResult, error)` signature
- Use `request.RequireString()` for required parameters
- Use `request.GetString()` with defaults for optional parameters
- Return errors via `mcp.NewToolResultError()` (not Go errors)
- Return success via `mcp.NewToolResultText(jsonString)`

### Comments

- Write comments for exported types and functions
- Use `//` for single-line comments
- No need for comments on obvious/unexported code
- Document complex algorithms and non-obvious behavior

## Running Locally

```bash
# Prerequisites: llama-server binary on PATH
# macOS: brew install llama.cpp
# Linux: apt-get install build-essential cmake

# Run with local database
./picobrain-mcp --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080

# Verify
curl http://localhost:8080/mcp
```

## Key Files

| File | Purpose |
|------|---------|
| `brain.go` | Core Brain struct, Store/Search/Reflect operations |
| `store.go` | SQLite database layer, schema, queries |
| `embed.go` | Local/Ollama embedder implementations |
| `mcp.go` | MCP tool registrations and handlers |
| `prompts.go` | Observer and Reflector system prompts |
| `thought.go` | Thought and BrainStats struct definitions |
| `config.go` | Configuration struct and defaults |
| `cmd/picobrain-mcp/main.go` | MCP HTTP server entry point |

## Common Patterns

### Adding a new MCP tool

1. Add tool registration in `RegisterMCPTools()` in `mcp.go`
2. Create handler function following the signature pattern
3. Add method to `Brain` struct if new operation needed

### Adding a new Thought field

1. Add to `Thought` struct in `thought.go` with JSON tag
2. Update schema in `initSchema()` in `store.go`
3. Update INSERT/SELECT queries in `store.go`
4. Update tests in `brain_test.go`

### Working with embeddings

- All embeddings are 768-dimensional (`ExpectedEmbeddingDim`)
- Use `sqlite_vec.SerializeFloat32()` before storing
- Use `brain.embedder.Embed(ctx, text)` to generate