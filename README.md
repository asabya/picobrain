# picobrain

Local semantic memory for AI agents. Stores notes, decisions, and context in SQLite with local embeddings (`nomic-embed-text-v1.5`). Exposes memory over MCP HTTP.

## Inspiration

Picobrain is inspired by [OB1](https://github.com/NateBJones-Projects/OB1) and this video: [Build Your AI a Second Brain](https://www.youtube.com/watch?v=2JiMmye2ezg).

Thanks to Nate B Jones for sharing the project and walkthrough.

## Docker (Recommended)

```bash
docker pull asabya/picobrain:latest
docker run -d -p 8080:8080 -v ~/.picobrain:/data --name picobrain asabya/picobrain:latest
```

Or with compose:

```bash
docker compose up -d
docker compose logs -f
```

First startup downloads the embedding model. The DB is at `~/.picobrain/brain.db`.

## Client Configuration

```json
{
  "brain": {
    "enabled": true,
    "type": "http",
    "url": "http://localhost:8080/mcp"
  }
}
```

## Local Run Without Docker

### 1. Prerequisites

- CGO toolchain for SQLite
- `llama-server` on `PATH` (or set `PICOBRAIN_LLAMA_SERVER_BIN`)

```bash
# macOS
brew install llama.cpp

# Linux
apt-get install build-essential cmake
```

### 2. Clone & Build

```bash
git clone https://github.com/asabya/picobrain.git
cd picobrain
go build -o picobrain-mcp ./cmd/picobrain-mcp
```

### 3. Run

```bash
./picobrain-mcp --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

MCP endpoint: `http://localhost:8080/mcp`

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--db` | SQLite database path | required |
| `--embed-model` | Embedding model name | `nomic-embed-text-v1.5` |
| `--model-cache` | Model cache directory | required |
| `--no-auto-download` | Fail instead of downloading model | `false` |
| `--port` | HTTP listen port | `8080` |

## MCP Tools

`store_thought`, `semantic_search`, `list_recent`, `stats`, `bulk_import`

## Usage Tips

- Store concise facts and decisions, not raw transcripts
- Include `people`, `topics`, `type`, and `action_items` when available
- Search semantically before asking the user to repeat context

## Go Library

```bash
go get github.com/asabya/picobrain
```

```go
brain, err := picobrain.New(picobrain.DefaultConfig())
if err != nil {
    log.Fatal(err)
}
defer brain.Close()

err = brain.Store(context.Background(), &picobrain.Thought{
    Content: "Alice is leading the frontend redesign.",
    People:  []string{"Alice"},
    Topics:  []string{"frontend", "design"},
    Type:    "person_note",
    Source:  "app",
})
```

## Build & Test

```bash
go build -o picobrain-mcp ./cmd/picobrain-mcp
go test ./...
```
