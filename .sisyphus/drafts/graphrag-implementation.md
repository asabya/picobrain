# Draft: GraphRAG Implementation for Picobrain

## Requirements (confirmed)
- **Data Source**: Use picobrain's existing thoughts as corpus
- **Scale**: 10,000+ thoughts
- **Goal**: Implement Vector + Graph combined with RRF (Full hybrid retrieval)
- **Testing**: TDD approach
- **Approach**: Build on existing codebase

## Technical Decisions
- **Entity extraction**: Use existing topics + people + content keywords as entities (lightweight)
- **Relation extraction**: Extract from content using patterns (not full NLP dependency parsing - too heavy)
- **Graph storage**: SQLite with adjacency list (not external graph DB)
- **RRF implementation**: Custom Go implementation
- **Embeddings**: Reuse existing embedder

## Architecture Components to Add
1. `graph.go` - Entity/Relation types and extraction logic
2. `graph_store.go` - Graph storage layer
3. Modify `brain.go` - Add hybrid search method
4. Modify `mcp.go` - Add hybrid_search tool

## Open Questions
- Exact RRF parameter (k=60 from paper or tune?)
- How to handle missing entity matches (fallback to vector)?

## Scope Boundaries
- **INCLUDE**: Entity table, Relation table, Hybrid search, MCP tool
- **EXCLUDE**: Full dependency parsing (keep simple pattern-based extraction)
- **EXCLUDE**: Multi-hop beyond 1-hop (keep simple)

## Implementation Order (TDD)
1. Write tests for entity extraction
2. Write tests for hybrid search
3. Implement entity types
4. Implement graph store
5. Implement hybrid search
6. Add MCP tool