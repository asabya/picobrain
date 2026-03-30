# picobrain

`picobrain` is a local semantic memory system for AI agents.

It stores notes, decisions, facts, and working context in SQLite, generates embeddings locally with `nomic-embed-text-v1.5`, and exposes the memory over MCP HTTP.

## What You Get

- local embeddings with no Ollama
- local SQLite storage
- MCP server at `http://localhost:8080/mcp`
- automatic model download on first run
- persistent DB and model cache with Docker

## Fastest Onboarding

If you want `picobrain` running in the simplest possible way:

```bash
./scripts/run-docker.sh
```

That will:

- build the `amd64` Docker image
- start `picobrain` on port `8080`
- create `./data/brain.db`
- cache the embedding model in `./data/models`

On first startup, `picobrain` downloads `nomic-embed-text-v1.5.Q8_0.gguf`, starts a local `llama-server`, and only then starts serving MCP.

## Client Configuration

Use this in your client config:

```json
{
  "mcp": {
    ...
    "servers": {
      "brain": {
        "enabled": true,
        "type": "http",
        "url": "http://localhost:8080/mcp"
      }
    }
  }
}
```

If your client uses a different overall config shape, keep this server definition:

```json
{
  "brain": {
    "enabled": true,
    "type": "http",
    "url": "http://localhost:8080/mcp"
  }
}
```

## Docker

Docker is the recommended way to run `picobrain`.

Requirements:

- Docker
- an `amd64` Docker runtime target

Run:

```bash
./scripts/run-docker.sh
```

Or directly:

```bash
docker compose build --no-cache
docker compose up -d --force-recreate
```

Default compose behavior:

- binds `./data` to `/data`
- stores the DB at `/data/brain.db`
- stores models at `/data/models`
- serves MCP at `http://localhost:8080/mcp`

Watch startup logs:

```bash
docker compose logs -f
```

## Local Run Without Docker

If you do not want Docker, you can run `picobrain` directly.

### 1. Install Dependencies

`picobrain` requires:

- CGO toolchain for SQLite
- `llama-server` available on `PATH`, or `PICOBRAIN_LLAMA_SERVER_BIN` set explicitly

macOS with Homebrew:

```bash
brew install llama.cpp
```

Linux:

```bash
apt-get install build-essential cmake
```

### 2. Build

```bash
go build -o picobrain-mcp ./cmd/picobrain-mcp
```

### 3. Start

```bash
./picobrain-mcp --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

The MCP endpoint will be:

- `http://localhost:8080/mcp`

## Available Flags

- `--db`: SQLite database path
- `--embed-model`: embedding model name, default `nomic-embed-text-v1.5`
- `--model-cache`: model cache directory
- `--no-auto-download`: fail instead of downloading the model
- `--port`: HTTP listen port, default `8080`

## MCP Tools

`picobrain` registers 5 MCP tools:

- `store_thought`
- `semantic_search`
- `list_recent`
- `stats`
- `bulk_import`

## How Agents Should Use It

Good patterns:

- store concise facts and decisions, not raw transcripts
- include `people`, `topics`, `type`, and `action_items` when available
- search semantically before asking the user to repeat context
- import historical notes once, then add distilled updates

See `INSTALL.md` for the agent-focused bootstrap, discovery, and MCP wiring instructions that Codex, Claude Desktop, OpenClaw, PicoClaw, and other clients rely on.

## Model Caching

The local embedder uses:

- model repo: `nomic-ai/nomic-embed-text-v1.5-GGUF`
- model file: `nomic-embed-text-v1.5.Q8_0.gguf`

Default behavior:

- if the model is cached, startup works offline
- if the model is missing, startup downloads it unless `--no-auto-download` is set
- MCP does not start until the model is ready

## Go Library Usage

You can also embed `picobrain` directly in a Go application.

Install:

```bash
go get github.com/asabya/picobrain
```

Minimal example:

```go
package main

import (
	"context"
	"log"

	"github.com/asabya/picobrain"
)

func main() {
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
	if err != nil {
		log.Fatal(err)
	}
}
```

## Build Notes

- Docker support is `amd64` only
- runtime inference is local-only
- no Ollama is required
- `picobrain-mcp` serves MCP over HTTP only

Build:

```bash
go build -o picobrain-mcp ./cmd/picobrain-mcp
```

Test:

```bash
go test ./...
```
