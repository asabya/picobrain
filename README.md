# picobrain

Local semantic memory for AI agents. Stores notes, decisions, and context in SQLite with local embeddings (`nomic-embed-text-v1.5`). Exposes memory over MCP HTTP.

Inspired by [OB1](https://github.com/NateBJones-Projects/OB1) and [Build Your AI a Second Brain](https://www.youtube.com/watch?v=2JiMmye2ezg).

## Quick Start

### Single Command Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash
```

Then add to your PATH and run:

```bash
export PATH="$HOME/.picobrain/bin:$PATH"
picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

First startup downloads the embedding model (~500MB). Wait for the log line confirming the server is ready.

### Docker

```bash
docker pull asabya/picobrain:latest
docker run -d -p 8080:8080 -v ~/.picobrain:/data --name picobrain asabya/picobrain:latest
```

### Docker Compose

```bash
docker compose up -d
docker compose logs -f
```

### Verify it's running

```bash
curl http://localhost:8080/mcp
```

## Connecting Your Agent

Point any MCP client to `http://localhost:8080/mcp`. Example config:

```json
{
  "brain": {
    "enabled": true,
    "type": "http",
    "url": "http://localhost:8080/mcp"
  }
}
```

### Claude Desktop

Add to your `mcpServers` config:

```json
{
  "mcpServers": {
    "picobrain": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-streamable-http", "http://localhost:8080/mcp"]
    }
  }
}
```

### OpenClaw / Codex / Other MCP Clients

Just add `http://localhost:8080/mcp` as an HTTP MCP server in your client config.

## MCP Tools

Once connected, these tools are available automatically:

| Tool | What it does |
|------|-------------|
| `store_thought` | Save a memory with metadata (people, topics, type, action_items, source) |
| `semantic_search` | Search memories by meaning |
| `list_recent` | List latest captured thoughts |
| `delete_thought` | Delete a thought by ID |
| `reflect` | Consolidate observations — atomically delete old thoughts and store new consolidated ones |
| `stats` | Brain stats (total thoughts, top topics, sources) |
| `bulk_import` | Import thoughts from JSONL |

## MCP Prompts

| Prompt | Purpose |
|--------|---------|
| `observe` | System prompt for compressing conversation messages into dense, factual observations |
| `reflect` | System prompt for long-term memory consolidation — merge, drop, and reorganize observations |

## Local Build (Without Docker)

### Prerequisites

- CGO toolchain for SQLite
- `llama-server` on `PATH` (or set `PICOBRAIN_LLAMA_SERVER_BIN`)

```bash
# macOS
brew install llama.cpp

# Linux
apt-get install build-essential cmake
```

### Build & Run

```bash
git clone https://github.com/asabya/picobrain.git
cd picobrain
go build -o picobrain ./cmd/picobrain-mcp
./picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

## CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--db` | SQLite database path | required |
| `--embed-model` | Embedding model name | `nomic-embed-text-v1.5` |
| `--model-cache` | Model cache directory | required |
| `--no-auto-download` | Fail instead of downloading model | `false` |
| `--port` | HTTP listen port | `8080` |

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

## Tips

- Store concise facts and decisions, not raw transcripts
- Include `people`, `topics`, `type`, and `action_items` when available
- Search semantically before asking the user to repeat context
- Use `observe` prompt at the end of conversations to extract dense observations
- Use `reflect` prompt periodically to consolidate and prune stale memories

## Build & Test

```bash
go build -o picobrain ./cmd/picobrain-mcp
go test ./...
```
