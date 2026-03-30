---
name: use-brain
description: Use when learning facts about the user or project, making decisions, tracking people or topics, solving problems, or before asking the user something you could look up. Also use when the user says remember, recall, discuss, decide, or when starting new tasks or investigating issues.
---

# Using Picobrain

## Core Principle

**Picobrain is your external semantic memory. Before asking the user, search picobrain. After learning something useful, store it.**

Picobrain lives at `http://localhost:8080/mcp` and persists facts across sessions. It remembers what you forget.

## Tools

| Tool | Purpose | When to Call |
|------|---------|--------------|
| `semantic_search` | Find related memories before acting | When you don't already know the answer |
| `store_thought` | Save facts, decisions, people, topics | After learning anything useful |
| `list_recent` | See recent memories for current work | When starting new tasks or investigating |
| `bulk_import` | Onboard historical notes | During onboarding or migrations |
| `stats` | Check brain health | Housekeeping contexts only |

## When to Search First (Search Before Asking)

**Use `semantic_search` when the user asks about:**
- "Did we discuss...", "what did we decide...", "remember when..."
- People: "who is...", "the user mentioned...", "they said..."
- Topics: "we looked at...", "the issue with...", "how we handle..."
- Prior work: "what's the status of...", "were we going to..."

**Use `semantic_search` before:**
- Asking the user to repeat context
- Starting work that might overlap with prior work
- Investigating a bug that could be known

**Search template:** `semantic_search` with a concise 1-2 sentence description of what you need.

## When to Store (Always After Learning)

**Use `store_thought` when you learn:**
- User's role, preferences, goals, or feedback
- Project context: bugs, decisions, architecture, conventions
- People mentioned and their attributes
- Action items, next steps, or commitments
- Solutions to problems (so you don't re-solve them)

**Distill, don't transcript.** Store the insight, not the raw conversation.

```json
{
  "content": "User prefers concise responses with no trailing summaries",
  "people": ["sabyasachipatra"],
  "topics": ["user-preference", "communication-style"],
  "type": "feedback"
}
```

## Common Mistakes

| Mistake | Instead |
|---------|---------|
| Asking the user to repeat context | Search picobrain first |
| Forgetting after the session ends | Store before ending |
| Storing raw transcripts | Distill to facts/decisions |
| Using `stats` during work | It's for housekeeping only |
| Not searching before investigating | Search first — it might be remembered |

## Quick Reference

```bash
# Search before asking
semantic_search(query="user preference for response style")

# Store after learning
store_thought(content="Decision: use goreleaser for cross-platform builds",
              topics=["build-tool", "release-process"],
              type="decision")

# Check recent for active work
list_recent(since="2026-03-29T00:00:00Z", limit=10)
```

## Red Flags — STOP

- Asking the user to repeat something you could search for
- "I'll remember that" (you won't — store it now)
- Forgetting facts between sessions because you didn't persist them
- Investigating something without searching first

**All of these mean: Use picobrain now.**
