# Observational Memory for Picobrain — Design Doc

## Context

Picobrain is a local semantic memory server for AI agents (MCP over HTTP). It stores "thoughts" with embeddings for cross-session recall via semantic search.

Mastra's Observational Memory (OM) introduces two concepts absent from picobrain:
1. **Observation** — automatically compressing conversation history into dense, structured summaries
2. **Reflection** — periodically consolidating observations (merging similar ones, pruning irrelevant ones)

This design adds OM capabilities to picobrain without introducing LLM dependencies. Picobrain stays a storage/search layer. The agent (MCP client) handles LLM calls using prompts provided by picobrain.

## Approach

Extend the existing `thoughts` table (no new tables). Add two new tools, enhance two existing tools with type filtering, and register two MCP prompts.

## Changes

### 1. New MCP Tools

#### `delete_thought`
- **Params:** `id` (string, required)
- **Behavior:** Deletes thought from `thoughts` and `thought_vectors` tables in a transaction
- **Returns:** `{ "deleted": bool, "id": string }`
- **Purpose:** Enables reflector to remove old observations after consolidation

#### `reflect`
- **Params:**
  - `delete_ids` (string[], required) — thought IDs to remove
  - `consolidated` (object[], required) — new thoughts to store (same schema as `store_thought` input)
- **Behavior:** Atomic transaction — stores all new thoughts, deletes all old ones. All-or-nothing.
- **Returns:** `{ "stored": [new IDs], "deleted": [old IDs] }`
- **Purpose:** Core OM primitive — swap old observations for consolidated ones

### 2. Enhanced Existing Tools

#### `list_recent` — add `type` filter
- New optional param: `type` (string) — filter results by thought type (e.g., `"observation"`)
- When empty/omitted: returns all types (backward compatible)
- Storage: add `WHERE type = ?` clause when filter provided

#### `semantic_search` — add `type` filter
- New optional param: `type` (string) — filter results by thought type
- When empty/omitted: searches all types (backward compatible)
- Storage: filter after vector search via WHERE clause on joined thoughts table

### 3. MCP Prompts

#### `observe`
- **Description:** System prompt for compressing conversation messages into dense observations
- **Usage:** Agent fetches prompt, runs LLM with it as system prompt + conversation messages as user content, stores result via `store_thought` with `type: "observation"`

#### `reflect_prompt`
- **Description:** System prompt for consolidating and pruning observations
- **Usage:** Agent fetches prompt, runs LLM with it as system prompt + existing observations as user content, stores consolidated result via `reflect` tool

### 4. No Schema Changes

No new columns or tables. Observations are thoughts with `type: "observation"`. The existing `type`, `content`, `people`, `topics`, `action_items`, `source`, `created_at` fields carry all necessary context.

## OM Workflow

```
Observation Phase (during conversation):
  1. Agent calls: get_prompt("observe") -> system prompt
  2. Agent runs LLM: system=observer_prompt, user=messages
  3. Agent calls: store_thought(content=..., type="observation")

Reflection Phase (periodic):
  1. Agent calls: list_recent(type="observation") -> observations
  2. Agent calls: get_prompt("reflect_prompt") -> system prompt
  3. Agent runs LLM: system=reflector_prompt, user=observations
  4. Agent calls: reflect(delete_ids=[...], consolidated=[...])

Recall (existing, unchanged):
  Agent calls: semantic_search("what did we decide about X?")
```

## Files to Modify

| File | Change |
|------|--------|
| `thought.go` | No changes (type field already exists) |
| `store.go` | Add `Delete(id)` method, update `ListRecent` and `Search` to accept optional type filter |
| `brain.go` | Add `Delete(ctx, id)` and `Reflect(ctx, deleteIDs, newThoughts)` methods |
| `mcp.go` | Register `delete_thought` tool, `reflect` tool, `observe` prompt, `reflect_prompt` prompt; add type param to existing tools |
| `cmd/picobrain-mcp/main.go` | Enable prompt capabilities on MCP server |

## What This Does NOT Include

- No LLM dependencies in picobrain
- No conversation history management
- No automatic observation generation (agent decides when)
- No background buffering (agent controls timing)
- No token counting or compression metadata (unnecessary for core functionality)
