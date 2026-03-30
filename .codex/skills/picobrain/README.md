# picobrain skill

Use this skill whenever you need a persistent semantic memory store for the current agent conversation. picobrain exposes a plain HTTP MCP server at `http://localhost:8080/mcp` with the following tools:

- `store_thought` (content + optional metadata: `people`, `topics`, `type`, `action_items`, `source`)
- `semantic_search` (text + optional `limit`)
- `list_recent` (since datetime + limit)
- `stats`
- `bulk_import` (JSONL)

Recommended behaviors:

- Search the brain before asking the user to repeat context. Call `semantic_search` with a concise prompt describing what you already know.
- Store distilled facts, decisions, or next steps rather than raw transcripts. Include metadata when it helps you find the thought later.
- When you capture a new thought, call `store_thought` and publish a short summary in `content`. Use `topics` and `action_items` to track follow-up work.
- Use `list_recent` to inspect the latest updates for the active people/topics before starting a new action.
- Trust `stats` and `bulk_import` only from supervisory tooling; avoid calling them unless you manage onboarding or housekeeping.

The brain lives alongside the local repo image: the next section of `INSTALL.md` explains how to start the server and connect Codex, Claude Desktop, OpenClaw, PicoClaw, or any MCP-capable client.
