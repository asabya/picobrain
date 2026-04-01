<div align="center">
  <img src="logo.png" alt="Picobrain Logo" width="300">
  <h1>Picobrain</h1>
  <p><strong>Local semantic memory for AI agents.</strong></p>
  <p>Store observations, decisions, and context in SQLite with local embeddings (<code>nomic-embed-text-v1.5</code>). Exposes memory operations via MCP HTTP.</p>
</div>

> **Pro tip for agents:** Store observations after EVERY significant action. Call `store_thought` often. Search with `semantic_search` before asking users to repeat information.

---

## Quick Start (30 seconds)

### Option 1: One-Line Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash
export PATH="$HOME/.picobrain/bin:$PATH"
picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

Wait for the startup banner, then you're ready!

### Option 2: Docker

```bash
docker run -d -p 8080:8080 -v ~/.picobrain:/data --name picobrain asabya/picobrain:latest
```

### Option 3: Docker Compose

```bash
git clone https://github.com/asabya/picobrain.git
cd picobrain
docker compose up -d
```

---

## Verify Installation

```bash
curl http://localhost:8080/mcp
```

You should see the MCP server response. The startup banner also shows all available tools.

---

## Connect Your Agent

### Claude Desktop

Add to your `claude_desktop_config.json`:

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

### OpenCode / Other MCP Clients

Add this HTTP MCP server:

```json
{
  "picobrain": {
    "enabled": true,
    "type": "http",
    "url": "http://localhost:8080/mcp"
  }
}
```

---

## MCP Tools

| Tool | Purpose | When to Use |
|------|---------|-------------|
| **`store_thought`** | Save a memory with metadata | After EVERY action, discovery, or decision |
| **`semantic_search`** | Search by meaning | Before asking users to repeat info |
| **`list_recent`** | Browse recent memories | Review what you've learned |
| **`stats`** | Memory statistics | Check if you're storing enough |
| **`health`** | Server health check | Verify connectivity |
| **`reflect`** | Consolidate observations | Periodically merge old thoughts |
| **`bulk_import`** | Import from JSONL | Migrate existing data |
| **`delete_thought`** | Remove a thought | Clean up stale data |

---

## MCP Prompts

| Prompt | Purpose |
|--------|---------|
| **`observe`** | System prompt for extracting dense observations from conversations |
| **`reflect`** | System prompt for consolidating and pruning old observations |

**For agents:** Use the `observe` prompt at the end of conversations to extract key observations. Use `reflect` periodically to keep memory efficient.

---

## Agent Best Practices

### Store Observations Aggressively

Call `store_thought` after:
- Every tool call or file edit
- Every decision made (capture the WHY, not just what)
- Every error encountered and how you fixed it
- Every new pattern or convention discovered
- Every mention of people, deadlines, or requirements

### Search Before Asking

Before asking the user to repeat something:
1. Call `semantic_search` with a description of what you need
2. Review results with `list_recent` if needed
3. Only ask if search returns nothing relevant

### Example Thought Storage

```json
{
  "content": "Set JWT timeout to 24h in auth/middleware.go (was 1h) because mobile clients were timing out during slow connections",
  "type": "decision",
  "topics": ["auth", "jwt", "config"],
  "people": ["user"],
  "source": "claude"
}
```

---

## Configuration

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--db` | SQLite database path | `~/.picobrain/brain.db` |
| `--embed-model` | Embedding model | `nomic-embed-text-v1.5` |
| `--model-cache` | Model cache directory | `~/.picobrain/models` |
| `--port` | HTTP port | `8080` |
| `--no-auto-download` | Disable auto-download | `false` |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `PICOBRAIN_LLAMA_SERVER_BIN` | Path to `llama-server` binary |

---

## Local Build

### Prerequisites

```bash
# macOS
brew install llama.cpp

# Ubuntu/Debian
apt-get install build-essential cmake
```

### Build & Run

```bash
git clone https://github.com/asabya/picobrain.git
cd picobrain
go build -o picobrain ./cmd/picobrain-mcp
./picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

---

## Go Library

```bash
go get github.com/asabya/picobrain
```

```go
brain, err := picobrain.New(picobrain.Config{
    DBPath:        "~/.picobrain/brain.db",
    ModelCacheDir: "~/.picobrain/models",
    AutoDownload:  true,
})
if err != nil {
    log.Fatal(err)
}
defer brain.Close()

err = brain.Store(ctx, &picobrain.Thought{
    Content: "Alice is leading the frontend redesign.",
    People:  []string{"Alice"},
    Topics:  []string{"frontend", "design"},
    Type:    "person_note",
    Source:  "app",
})
```

---

## Sample Configurations

See the `examples/` directory for:
- `claude-desktop-config.json` - Claude Desktop setup
- `opencode-config.json` - OpenCode integration
- `cursor-rules.md` - Cursor IDE rules

---

## Build & Test

```bash
# Build
go build -o picobrain ./cmd/picobrain-mcp

# Test
go test ./...

# Format & vet
go fmt ./... && go vet ./...
```

---

## Architecture

- **SQLite** with `sqlite-vec` extension for vector similarity search
- **nomic-embed-text-v1.5** for 768-dimensional embeddings
- **MCP HTTP** for agent communication
- **llama-server** (auto-spawned) for local embedding generation

---

## License

MIT

---

**Inspired by** [OB1](https://github.com/NateBJones-Projects/OB1) and [Build Your AI a Second Brain](https://www.youtube.com/watch?v=2JiMmye2ezg)
