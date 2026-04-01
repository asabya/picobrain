# Picobrain Memory Integration

This project uses [Picobrain](https://github.com/asabya/picobrain) for local semantic memory.

## Setup

1. Start picobrain server:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/asabya/picobrain/main/install | bash
   picobrain --db ~/.picobrain/brain.db --model-cache ~/.picobrain/models --port 8080
   ```

2. The server exposes MCP tools at `http://localhost:8080/mcp`

## Agent Instructions

### Store Observations Aggressively

After EVERY significant action, call `store_thought` to capture:
- What you did (file edits, commands, tool calls)
- What you learned (discoveries, patterns, "aha" moments)
- Decisions made and their reasoning
- Errors encountered and solutions
- Context about users, projects, constraints

### Search Before Asking

Before asking the user to repeat information:
1. Use `semantic_search` to find relevant memories
2. Use `list_recent` to review recent observations
3. Only ask if search returns nothing useful

### Reflect Periodically

Use the `reflect` tool to consolidate old observations:
- After accumulating 20+ thoughts
- At the end of work sessions
- When switching to different tasks

This merges related observations and removes stale information.

## Available Tools

- `store_thought` - Save observations with metadata (people, topics, type)
- `semantic_search` - Find memories by meaning, not keywords
- `list_recent` - Review recent thoughts
- `stats` - Check memory statistics
- `health` - Verify server connectivity
- `reflect` - Consolidate old observations
- `bulk_import` - Import historical data
