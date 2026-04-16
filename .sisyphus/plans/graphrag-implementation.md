# GraphRAG Implementation Plan for Picobrain

## TL;DR

> Implement hybrid graph + vector retrieval with RRF (Reciprocal Rank Fusion) for picobrain's semantic search, using existing thought metadata (topics, people) as entities.

**Deliverables**:
- Entity/Relation types and extraction logic
- Graph storage layer in SQLite
- Hybrid search combining vector similarity + graph traversal
- MCP tool `hybrid_search` exposing the new search

**Estimated Effort**: Medium
**Parallel Execution**: NO - sequential (TDD)
**Critical Path**: Types → Store → Integration → Tests → MCP

---

## Context

### Original Request
Implement GraphRAG approach from arxiv.org/html/2507.03226 in picobrain - use existing thoughts as corpus, combine vector + graph search with RRF.

### Interview Summary
**Key Discussions**:
- Use picobrain's 10,000+ existing thoughts as the knowledge graph source
- Entities come from: topics, people, extracted keywords from content
- Relation extraction: Pattern-based from content (simple, not full NLP)
- RRF k parameter: Use 60 (from paper) as default
- TDD approach required

### Research Findings
- Existing code uses `vec0` for vector search
- `thoughts` table has: topics (JSON array), people (JSON array)
- These can serve as pre-built entities
- Need to add relation tables for graph edges

### Metis Review
**Identified Gaps** (addressed):
- Full dependency parsing is too heavy → Use pattern-based extraction from existing metadata
- External graph DB is overkill → Use SQLite adjacency approach
- Need backward compatibility → Keep existing `Search()` working, add new `HybridSearch()`

---

## Work Objectives

### Core Objective
Add hybrid retrieval to picobrain that combines:
1. **Vector search** (existing) - cosine similarity on thought embeddings
2. **Graph traversal** - 1-hop neighbors via topic/person overlap
3. **RRF** - Fuse ranked results for better context

### Concrete Deliverables
- `graph.go` - Entity, Relation, RRF types and functions
- `graph_store.go` - Graph storage (entities, relations tables)
- `brain.go` - Add `HybridSearch()` method
- `mcp.go` - Add `hybrid_search` tool
- Tests for all above

### Definition of Done
- [ ] `go test ./...` passes with new hybrid search tests
- [ ] `hybrid_search` MCP tool works with both vector and graph results
- [ ] Backward compatible: old `semantic_search` still works

### Must Have
- Entity extraction from existing thought metadata (topics, people)
- Relation building from content similarity + shared metadata
- RRF algorithm combining two ranked lists
- 1-hop graph traversal

### Must NOT Have (Guardrails)
- External graph DB dependencies (keep SQLite-only)
- Full NLP/dependency parsing (too heavy)
- Multi-hop beyond 1 (keep simple for performance)

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: YES (Go standard testing)
- **Automated tests**: YES (TDD)
- **Framework**: `go test`

### QA Policy
Every task includes agent-executed QA scenarios:
- Run `go test ./...` for unit tests
- Verify MCP tool registration
- Manual test via curl for hybrid_search

---

## Execution Strategy

### Sequential TDD Waves

```
Wave 1 (Foundation):
├── T1: Add entity/relation types + tests [quick]
├── T2: Add RRF algorithm + tests [quick]
└── T3: Graph store layer + tests [deep]

Wave 2 (Core + Integration):
├── T4: Brain HybridSearch method [deep]
├── T5: MCP tool hybrid_search [quick]
└── T6: Integration tests [unspecified-high]

Wave FINAL:
├── T7: Verify all tests pass
└── T8: Manual verification
```

---

## TODOs

- [ ] 1. **Entity/Relation Types + RRF Algorithm**

  **What to do**:
  - Define `Entity` struct: id, name, type, embedding
  - Define `Relation` struct: head_id, tail_id, rel_type, confidence
  - Define `RRFResult` struct for fused results
  - Implement `RecipRankFusion()` function combining two ranked lists
  - Write tests: entity creation, RRF edge cases

  **Must NOT do**:
  - Don't add external graph DB logic

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
  - **Justification**: Types + pure algorithm, no framework complexity

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Sequential**: Wave 1

  **References**:
  - `thought.go` - Existing thought types to follow

  **Acceptance Criteria**:
  - [ ] RRF produces correct fused rankings
  - [ ] Tests pass

  **QA Scenarios**:
  ```
  Scenario: RRF combines ranks correctly
    Tool: Bash (go test)
    Steps:
      1. Run test with ranks [1,3,5] and [2,3,4]
      2. Assert result order: 1,2,3,4,5
    Expected Result: Correct fusion
    Evidence: test output

  Scenario: Empty list handling
    Tool: Bash (go test)
    Steps:
      1. Pass empty list to RRF
      2. Assert returns non-empty result
    Expected Result: Handles gracefully
    Evidence: test output
  ```

  **Commit**: YES (group 1)
  - Message: `feat(graphrag): add entity types and RRF algorithm`
  - Files: `graph.go`

- [ ] 2. **Graph Storage Layer**

  **What to do**:
  - Add `entities` table: id, name, type, embedding_id
  - Add `relations` table: id, head_id, tail_id, rel_type, confidence
  - Implement `BuildGraph()` - extract entities from existing thoughts
  - Implement `GetNeighbors()` - 1-hop traversal
  - Write tests: entity extraction, relation building

  **Must NOT do**:
  - Don't use external graph DB

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []
  - **Justification**: Storage layer with SQL complexity

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Sequential**: Wave 1

  **References**:
  - `store.go` - Existing schema patterns
  - `thought.go` - Existing types

  **Acceptance Criteria**:
  - [ ] Entities extracted from topics + people
  - [ ] Relations built from shared metadata
  - [ ] 1-hop neighbors retrieved

  **QA Scenarios**:
  ```
  Scenario: Extract entities from thought
    Tool: Bash (go test)
    Steps:
      1. Create thought with topics: ["auth", "jwt"]
      2. Call BuildGraph
      3. Assert entities include "auth", "jwt"
    Expected Result: Entities found
    Evidence: test output

  Scenario: Build relations from shared topics
    Tool: Bash (go test)
    Steps:
      1. Two thoughts share topic "auth"
      2. Call BuildGraph
      3. Assert relation exists between them
    Expected Result: Relation created
    Evidence: test output
  ```

  **Commit**: YES (group 2)
  - Message: `feat(graphrag): add graph storage layer`
  - Files: `graph_store.go`

- [ ] 3. **Brain HybridSearch Method**

  **What to do**:
  - Add `HybridSearch()` method to Brain struct
  - Implement: noun phrase extraction → vector search → 1-hop neighbors → RRF
  - Return fused results with graph context
  - Write tests: full hybrid search flow

  **Must NOT do**:
  - Don't break existing Search() method

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []
  - **Justification**: Core algorithm integration

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Sequential**: Wave 2

  **References**:
  - `brain.go:133` - Existing Search() for pattern

  **Acceptance Criteria**:
  - [ ] HybridSearch returns results
  - [ ] Results include vector + graph components
  - [ ] Backward compatible with Search()

  **QA Scenarios**:
  ```
  Scenario: Hybrid search returns fused results
    Tool: Bash (go test)
    Steps:
      1. Seed with test thoughts
      2. Call HybridSearch("authentication")
      3. Assert results returned with both sources
    Expected Result: Fused results
    Evidence: test output
  ```

  **Commit**: YES (group 3)
  - Message: `feat(graphrag): add hybrid search method`
  - Files: `brain.go`

- [ ] 4. **MCP Tool hybrid_search**

  **What to do**:
  - Add `hybrid_search` MCP tool registration
  - Implement handler calling Brain.HybridSearch()
  - Return results with graph context metadata
  - Write tests: tool registration

  **Must NOT do**:
  - Don't remove existing semantic_search

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
  - **Justification**: Simple tool addition

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Sequential**: Wave 2

  **References**:
  - `mcp.go:31` - Existing semantic_search registration

  **Acceptance Criteria**:
  - [ ] Tool registered in MCP
  - [ ] Handler executes HybridSearch
  - [ ] Returns graph context

  **QA Scenarios**:
  ```
  Scenario: Tool executes successfully
    Tool: Bash (go test)
    Steps:
      1. Register tool
      2. Invoke hybrid_search
      3. Assert valid response
    Expected Result: Success
    Evidence: test output
  ```

  **Commit**: YES (group 4)
  - Message: `feat(graphrag): add hybrid_search MCP tool`
  - Files: `mcp.go`

- [ ] 5. **Integration Tests**

  **What to do**:
  - Full end-to-end test: store thought → hybrid search
  - Test with real embeddings
  - Test backward compatibility

  **Must NOT do**:
  - Don't skip edge cases

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: []
  - **Justification**: Full integration validation

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Sequential**: Wave 2

  **Acceptance Criteria**:
  - [ ] End-to-end works
  - [ ] All existing tests still pass

  **QA Scenarios**:
  ```
  Scenario: Full pipeline works
    Tool: Bash (go test ./...)
    Steps:
      1. Run all tests
      2. Assert all pass
    Expected Result: 100% pass
    Evidence: test output
  ```

  **Commit**: YES
  - Message: `feat(graphrag): add integration tests`

---

## Final Verification Wave

- [ ] F1. **Build Verification** — `oracle`
  Run `go build ./...` to verify no compilation errors.
  Output: `Build [PASS/FAIL]`

- [ ] F2. **Test Suite** — `unspecified-high`
  Run `go test ./...` - all tests must pass.
  Output: `[N/N] tests passed`

- [ ] F3. **MCP Tool Check** — `deep`
  Verify `hybrid_search` tool is registered and responds.
  Output: `Tool [REGISTERED/NOT FOUND]`

- [ ] F1. **Unit Tests** - `go test -v ./...` (oracle)
- [ ] F2. **Build** - `go build ./...` (unspecified-high)
- [ ] F3. **MCP Tool** - Verify tool registers (deep)

---

## Commit Strategy

- **1**: `feat(graphrag): add entity types and RRF` - graph.go
- **2**: `feat(graphrag): add graph storage` - graph_store.go  
- **3**: `feat(graphrag): add hybrid search method` - brain.go
- **4**: `feat(graphrag): add hybrid_search MCP tool` - mcp.go

---

## Success Criteria

### Verification Commands
```bash
go build ./...           # Expected: no errors
go test ./...           # Expected: all pass
curl http://localhost:8080/mcp  # Expected: hybrid_search in list
```