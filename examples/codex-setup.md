# Picobrain for Codex / GitHub Copilot

## MCP Configuration

Add to your Codex configuration:

```json
{
  "mcpServers": {
    "picobrain": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

## Quick Start

```bash
# Install picobrain
curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash

# Start the server
export PATH="$HOME/.picobrain/bin:$PATH"
picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
```

## Agent Behavior

### Always Store Observations

Store thoughts after:
- Reading or modifying files
- Running commands or tests
- Making decisions about architecture
- Learning about codebase patterns
- Encountering errors or edge cases

### Search Memory First

Before asking users:
```
semantic_search: "What was decided about the auth system?"
semantic_search: "What patterns exist in the codebase?"
semantic_search: "What errors have we seen before?"
```

### Use Rich Metadata

```json
{
  "content": "Moved auth logic to middleware package",
  "type": "decision",
  "topics": ["auth", "refactor", "architecture"],
  "people": ["user"],
  "action_items": ["Update API docs"]
}
```

## Tips

- Call `stats` to check if you're storing enough observations
- Use `health` to verify the server is running
- Run `reflect` periodically to consolidate old memories
