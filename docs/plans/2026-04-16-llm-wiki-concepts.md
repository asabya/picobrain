# Applying Karpathy's LLM Wiki Concepts to Picobrain

## Context

Karpathy's LLM Wiki pattern describes a knowledge system where LLMs incrementally maintain a structured wiki rather than rediscovering knowledge from scratch on every query. Key concepts:

1. **Lint operation**: Detect contradictions, stale claims, orphan pages
2. **Log.md**: Append-only audit trail
3. **Index.md**: Categorized catalog

Picobrain already has:
- ✅ Persistent storage with vector search
- ✅ Consolidation via `reflect` (merge/drop/keep)
- ✅ Graph relationships (edges between thoughts)
- ✅ Source tracking and timestamps
- ✅ Namespace isolation

**User preferences confirmed**:
- Lint operation: **Manual only** (explicit `mcp__brain__lint` call)
- Raw source anchoring: **Out of scope** (skip for now)
- Priority: **Lint first** (contradiction detection is highest value)

---

## Implementation Plan

### Phase 1: Lint Tool (Contradiction Detection) - **HIGHEST PRIORITY**

Add a manual `mcp__brain__lint` tool that analyzes thoughts for issues.

#### New Files/Structures

**`thought.go`** - Add LintIssue struct:
```go
type LintIssue struct {
    Type       string   `json:"type"`        // "contradiction", "superseded", "stale", "orphan"
    ThoughtIDs []string `json:"thought_ids"` // Thoughts involved
    Reason     string   `json:"reason"`      // Explanation
    Severity   string   `json:"severity"`    // "high", "medium", "low"
}
```

**`brain.go`** - Add `Lint()` method:
```go
func (b *Brain) Lint(ctx context.Context, opts LintOptions) ([]LintIssue, error)

type LintOptions struct {
    CheckContradictions bool     // Find semantically conflicting thoughts
    CheckSuperseded      bool     // Find decisions that may be outdated
    CheckOrphans         bool     // Find disconnected thoughts
    CheckStale           bool     // Find old observations that may be irrelevant
    Namespace            string   // Filter to namespace
    MaxAge               time.Duration // For stale check
}
```

#### Detection Strategies

**1. Contradiction Detection** (semantic):
- Find thought pairs with similar topics but opposing sentiments
- Use vector search to find semantically similar thoughts
- Check for negation patterns, opposing claims
- Example: "X is deprecated" vs "Use X for Y"

**2. Superseded Detection** (temporal):
- Find decisions that have newer decisions on same topic
- Look for edges with `relation_type = "supersedes"`
- Check for temporal patterns: older decision + newer decision on same topic

**3. Orphan Detection** (structural):
- Find thoughts with no edges (incoming or outgoing)
- Optional: Find thoughts with no semantic neighbors within threshold

**4. Stale Detection** (temporal + type):
- Find old `task` thoughts not marked complete
- Find old `observation` thoughts beyond threshold age
- Find `decision` thoughts that may have context changes

#### MCP Tool Definition

**`mcp.go`** - Add lint tool:
```go
s.AddTool(
    mcp.NewTool("lint",
        mcp.WithDescription("Analyze your memory for issues: contradictions between thoughts, superseded decisions, orphan thoughts with no connections, and stale observations. Run this periodically to keep your memory clean."),
        mcp.WithBoolean("check_contradictions", mcp.Description("Find thoughts that may contradict each other (default: true)")),
        mcp.WithBoolean("check_superseded", mcp.Description("Find decisions that may have been superseded (default: true)")),
        mcp.WithBoolean("check_orphans", mcp.Description("Find thoughts with no graph connections (default: true)")),
        mcp.WithBoolean("check_stale", mcp.Description("Find old observations that may be outdated (default: false)")),
        mcp.WithString("namespace", mcp.Description("Filter by namespace. Leave empty to lint all namespaces.")),
        mcp.WithString("max_age", mcp.Description("For stale check: max age as duration (e.g., '30d', '90d')")),
    ),
    lintHandler(brain),
)
```

---

### Phase 2: Audit Log

Add append-only logging of all operations.

#### Database Schema

**`store.go`** - Add `operations_log` table in `initSchema()`:
```sql
CREATE TABLE IF NOT EXISTS operations_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL,      -- "store", "search", "reflect", "delete", "lint"
    thought_id TEXT,              -- Optional: related thought ID
    details JSON,                 -- Operation-specific metadata
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ops_type ON operations_log(operation);
CREATE INDEX IF NOT EXISTS idx_ops_time ON operations_log(created_at);
```

#### Logging Integration

**`brain.go`** - Add logging to existing methods:
- `Store()` → log operation="store", thought_id, topics
- `Search()` → log operation="search", query details
- `Reflect()` → log operation="reflect", delete_count, store_count
- `Delete()` → log operation="delete", thought_id

#### MCP Tool

**`mcp__brain__log`** - Query operation history:
```go
mcp.WithString("operation", mcp.Description("Filter by operation type: store, search, reflect, delete, lint")),
mcp.WithString("since", mcp.Description("ISO8601 datetime to start from")),
mcp.WithNumber("limit", mcp.Description("Max results (default: 100)")),
```

---

### Phase 3: Thought Index/Catalog

Generate structured overview of stored thoughts.

**`brain.go`** - Add `Index()` method:
```go
type ThoughtSummary struct {
    ID        string    `json:"id"`
    Content   string    `json:"content"`   // First 200 chars
    Type      string    `json:"type"`
    Topics    []string  `json:"topics"`
    CreatedAt time.Time `json:"created_at"`
}

type ThoughtIndex struct {
    ByTopic   map[string][]ThoughtSummary `json:"by_topic"`
    ByType    map[string][]ThoughtSummary `json:"by_type"`
    BySource  map[string][]ThoughtSummary `json:"by_source"`
    Recent    []ThoughtSummary             `json:"recent"`     // Last 10
    Orphans   []ThoughtSummary             `json:"orphans"`    // No edges
    Stats     BrainStats                   `json:"stats"`
}

func (b *Brain) Index(ctx context.Context, namespace string) (*ThoughtIndex, error)
```

**`mcp__brain__index`** - MCP tool to get structured overview.

---

## Critical Files

| File | Changes |
|------|---------|
| `thought.go` | Add `LintIssue`, `LintOptions`, `ThoughtSummary`, `ThoughtIndex` structs |
| `brain.go` | Add `Lint()`, `Index()`, `LogOperation()` methods |
| `store.go` | Add `operations_log` table schema, query functions for lint |
| `mcp.go` | Add `lint`, `log`, `index` MCP tool definitions and handlers |

---

## Verification Plan

1. **Unit tests** (`brain_test.go`):
   - `TestLintContradictions` - Store conflicting thoughts, verify detection
   - `TestLintOrphans` - Store thought with no edges, verify detection
   - `TestAuditLog` - Verify operations are logged
   - `TestIndex` - Verify categorized output

2. **Integration tests**:
   - Store thoughts with known issues
   - Call `lint` MCP tool
   - Verify issues returned match expectations

3. **Manual testing**:
   - Run against existing picobrain database
   - Verify lint returns meaningful issues
   - Check log entries appear in operations_log

---

## Implementation Order

1. **Phase 1**: Lint tool (contradiction detection) ← **START HERE**
2. **Phase 2**: Audit log
3. **Phase 3**: Thought index