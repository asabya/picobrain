# Installing picobrain for Agents

This repo ships a local semantic memory brain that speaks MCP over HTTP. `INSTALL.md` bundles the steps an agent needs to:

1. Bootstrap the `picobrain` process (Docker or local binary).
2. Wire the agent or client into `http://localhost:8080/mcp`.

picobrain exposes 5 MCP tools: `store_thought`, `semantic_search`, `list_recent`, `stats`, `bulk_import`. MCP clients discover these automatically via the protocol — no additional skill files needed.

## Step 0: Prerequisites

- Docker + docker-compose (recommended).
- Go toolchain **with CGO enabled** and a `llama-server` binary on `PATH` if you choose the local build path.

## Step 1: Start the brain

### Preferred: Single Command Install

```bash
curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash
```

Then add to your PATH and run:

```bash
export PATH="$HOME/.picobrain/bin:$PATH"
picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

This downloads both `picobrain` and `llama-server` binaries to `~/.picobrain/bin`.

### Docker (keeps the embedder self-contained)

```bash
docker pull asabya/picobrain
docker run -d -v ./data:/app/data -p 8080:8080 asabya/picobrain
```

This pulls the pre-built image, stores the SQLite database in `./data/brain.db`, caches models in `./data/models`, and exposes MCP at `http://localhost:8080/mcp`. Logs stream to stdout, and the first run downloads `nomic-embed-text-v1.5.Q8_0.gguf` before MCP becomes available.

Alternatively, build locally:

```bash
./scripts/run-docker.sh
```

### Optional: Local binary (manual control)

```bash
go build -o picobrain ./cmd/picobrain-mcp
./picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

Set `PICOBRAIN_LLAMA_SERVER_BIN` if your `llama-server` binary lives outside `PATH`. Use `--no-auto-download` to prevent runtime downloads when running in air-gapped environments.

## Step 2: Verify the HTTP MCP endpoint

```bash
curl --fail http://localhost:8080/mcp
```

A healthy server replies with the MCP handshake payload. Once this succeeds, the brain is ready to receive tool calls.

## Step 3: Connect your agent or client

### Claude Desktop

Copy or reference `docs/claude-desktop-config.json` (exact JSON is mirrored in this repo) to the `mcpServers` block. That config launches `@modelcontextprotocol/server-streamable-http` pointed at `http://localhost:8080/mcp` so Claude Desktop can pipe its own MCP connection through the bridge.

### OpenClaw / PicoClaw / Codex / generic MCP clients

Define an MCP server named `picobrain` that targets `http://localhost:8080/mcp`. Here is the minimal JSON blob that works inside most MCP client configs:

```json
{
  "picobrain": {
    "enabled": true,
    "type": "http",
    "url": "http://localhost:8080/mcp"
  }
}
```

That entry keeps the agent focused on the HTTP transport rather than custom auth headers or ports.

### Verification (any client)

Invoke `store_thought`, then `semantic_search` or `list_recent` to confirm round trips. Bulk imports also work via `bulk_import` when onboarding historical notes.

## MCP Tools

picobrain registers 5 tools automatically discovered by any MCP client:

| Tool | Description |
|------|-------------|
| `store_thought` | Store a thought with optional metadata (people, topics, type, action_items, source) |
| `semantic_search` | Search memories by meaning using vector similarity |
| `list_recent` | List recently captured thoughts ordered by newest first |
| `stats` | Get brain statistics (total thoughts, top topics, sources, date range) |
| `bulk_import` | Import multiple thoughts from JSONL format |

## Updating & Uninstall

- Re-run the install command to update: `curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash`
- For Docker: `docker pull asabya/picobrain` or `./scripts/run-docker.sh`
- For local build: `go build` again and restart the server with the same flags.
- Stop the Docker stack with `docker compose down`.
- Remove the Docker volume or delete the database file to uninstall data; deleting the repo is optional.
- To uninstall the binary: `rm -rf ~/.picobrain`
