# Installing picobrain for Agents

This repo ships a local semantic memory brain that speaks MCP over HTTP. `INSTALL.md` bundles the steps an agent needs to:

1. Bootstrap the `picobrain` process (Docker or local binary).
2. Wire the agent or client into `http://localhost:8080/mcp`.
3. Discover the repository’s built-in skill instructions so the agent knows *which tools to call* and *how to think about adding facts.*

## Step 0: Prerequisites

- Docker + docker-compose (recommended).
- Go toolchain **with CGO enabled** and a `llama-server` binary on `PATH` if you choose the local build path.

## Step 1: Start the brain

### Preferred: Docker (keeps the embedder self-contained)

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
go build -o picobrain-mcp ./cmd/picobrain-mcp
./picobrain-mcp --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

Set `PICOBRAIN_LLAMA_SERVER_BIN` if your `llama-server` binary lives outside `PATH`. Use `--no-auto-download` to prevent runtime downloads when running in air-gapped environments.

## Step 2: Verify the HTTP MCP endpoint

```bash
curl --fail http://localhost:8080/mcp
```

A healthy server replies with the MCP handshake payload. Once this succeeds, the brain is ready to receive tool calls.

## Step 3: Connect your agent or client

### Codex (native skill discovery)

1. Symlink the shipped skill so Codex can find it:

```bash
mkdir -p ~/.agents/skills
ln -s ~/path/to/picobrain/.codex/skills/picobrain ~/.agents/skills/picobrain
```

2. Restart Codex and let it read the skill instructions in `.codex/skills/picobrain/README.md`.
3. Invoke the `picobrain` skill when you need to store, search, or inspect memory facts. The skill explains which MCP tools should be called and how to think about facts, metadata, and freshness.

### Claude Desktop

Copy or reference `docs/claude-desktop-config.json` (exact JSON is mirrored in this repo) to the `mcpServers` block. That config launches `@modelcontextprotocol/server-streamable-http` pointed at `http://localhost:8080/mcp` so Claude Desktop can pipe its own MCP connection through the bridge.

### OpenClaw / PicoClaw / generic MCP clients

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

## Step 4: Install the agent skill

### For Codex agents

```bash
mkdir -p ~/.agents/skills/use-brain && \
curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/refs/heads/master/skills/use-brain/SKILL.md -o ~/.agents/skills/use-brain/SKILL.md
```

This downloads and installs the `use-brain` skill so Codex agents know when to search picobrain before asking and how to store distilled facts.

### For other agents

Copy or reference `skills/use-brain/SKILL.md` into your agent's skill discovery path.

## Step 5: Follow the skill guidance

Read the installed skill at `~/.agents/skills/use-brain/SKILL.md` for best practices: search before writing, store distilled metadata (people, topics, action items), and avoid saving raw transcripts. Call `stats` only from housekeeping contexts and let `bulk_import` handle migrations of large datasets.

## Updating & Uninstall

- Re-run `docker pull asabya/picobrain` or `./scripts/run-docker.sh` to update the Docker image.
- For local binaries, `go build` again and restart the server with the same flags.
- Stop the Docker stack with `docker compose down`.
- Remove the skill file (`rm ~/.agents/skills/use-brain/SKILL.md`) to uninstall the skill; deleting the repo is optional.
