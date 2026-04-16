package picobrain

import (
	"context"
	"testing"
)

func TestCreateEdge(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Create two thoughts first
	t1 := &Thought{Content: "First thought", Type: "observation"}
	t2 := &Thought{Content: "Second thought", Type: "observation"}
	if err := brain.Store(ctx, t1); err != nil {
		t.Fatalf("store t1: %v", err)
	}
	if err := brain.Store(ctx, t2); err != nil {
		t.Fatalf("store t2: %v", err)
	}

	// Create an edge
	edge := &Edge{
		SourceID:     t1.ID,
		TargetID:     t2.ID,
		RelationType: "references",
	}
	if err := brain.CreateEdge(ctx, edge); err != nil {
		t.Fatalf("create edge: %v", err)
	}

	if edge.ID == "" {
		t.Error("expected edge ID to be set")
	}
	if edge.Weight != 1.0 {
		t.Errorf("expected default weight 1.0, got %f", edge.Weight)
	}
}

func TestCreateEdgeValidatesThoughts(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	edge := &Edge{
		SourceID:     "nonexistent",
		TargetID:     "also-nonexistent",
		RelationType: "references",
	}
	if err := brain.CreateEdge(ctx, edge); err == nil {
		t.Error("expected error for nonexistent thoughts")
	}
}

func TestCreateEdgeValidatesFields(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	edge := &Edge{SourceID: "", TargetID: "x", RelationType: "y"}
	if err := brain.CreateEdge(ctx, edge); err == nil {
		t.Error("expected error for missing source_id")
	}

	edge = &Edge{SourceID: "x", TargetID: "", RelationType: "y"}
	if err := brain.CreateEdge(ctx, edge); err == nil {
		t.Error("expected error for missing target_id")
	}

	edge = &Edge{SourceID: "x", TargetID: "y", RelationType: ""}
	if err := brain.CreateEdge(ctx, edge); err == nil {
		t.Error("expected error for missing relation_type")
	}
}

func TestDeleteEdge(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "First thought"}
	t2 := &Thought{Content: "Second thought"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)

	edge := &Edge{
		SourceID:     t1.ID,
		TargetID:     t2.ID,
		RelationType: "references",
	}
	brain.CreateEdge(ctx, edge)

	if err := brain.DeleteEdge(ctx, edge.ID); err != nil {
		t.Fatalf("delete edge: %v", err)
	}

	// Verify it's gone
	neighbors, err := brain.GetNeighbors(ctx, t1.ID, "out", "", 10)
	if err != nil {
		t.Fatalf("get neighbors: %v", err)
	}
	if len(neighbors) != 0 {
		t.Errorf("expected 0 neighbors after delete, got %d", len(neighbors))
	}
}

func TestGetNeighbors(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	t3 := &Thought{Content: "Thought C"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)
	brain.Store(ctx, t3)

	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "references"})
	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t3.ID, RelationType: "causes"})
	brain.CreateEdge(ctx, &Edge{SourceID: t3.ID, TargetID: t1.ID, RelationType: "elaborates"})

	// Outgoing edges from t1
	out, err := brain.GetNeighbors(ctx, t1.ID, "out", "", 10)
	if err != nil {
		t.Fatalf("get outgoing: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 outgoing edges, got %d", len(out))
	}

	// Incoming edges to t1
	in, err := brain.GetNeighbors(ctx, t1.ID, "in", "", 10)
	if err != nil {
		t.Fatalf("get incoming: %v", err)
	}
	if len(in) != 1 {
		t.Errorf("expected 1 incoming edge, got %d", len(in))
	}

	// Both directions
	both, err := brain.GetNeighbors(ctx, t1.ID, "both", "", 10)
	if err != nil {
		t.Fatalf("get both: %v", err)
	}
	if len(both) != 3 {
		t.Errorf("expected 3 total edges, got %d", len(both))
	}

	// Filter by relation type
	refs, err := brain.GetNeighbors(ctx, t1.ID, "out", "references", 10)
	if err != nil {
		t.Fatalf("get references: %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("expected 1 reference edge, got %d", len(refs))
	}
}

func TestFindPath(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Create a chain: A -> B -> C
	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	t3 := &Thought{Content: "Thought C"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)
	brain.Store(ctx, t3)

	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "leads_to"})
	brain.CreateEdge(ctx, &Edge{SourceID: t2.ID, TargetID: t3.ID, RelationType: "leads_to"})

	// Find path from A to C
	steps, err := brain.FindPath(ctx, t1.ID, t3.ID, 5)
	if err != nil {
		t.Fatalf("find path: %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("expected path length 2, got %d", len(steps))
	}
	if len(steps) >= 1 {
		if steps[0].FromID != t1.ID || steps[0].ToID != t2.ID {
			t.Errorf("expected first step A->B, got %s->%s", steps[0].FromID, steps[0].ToID)
		}
	}
	if len(steps) >= 2 {
		if steps[1].FromID != t2.ID || steps[1].ToID != t3.ID {
			t.Errorf("expected second step B->C, got %s->%s", steps[1].FromID, steps[1].ToID)
		}
	}

	// No path when none exists
	steps, err = brain.FindPath(ctx, t3.ID, t1.ID, 5)
	if err != nil {
		t.Fatalf("find path (no path): %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for no path, got %d", len(steps))
	}
}

func TestCascadeDeleteEdges(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)

	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "references"})

	// Delete thought t1 — should also delete its edges
	if err := brain.Delete(ctx, t1.ID); err != nil {
		t.Fatalf("delete thought: %v", err)
	}

	// Verify edges are gone
	neighbors, err := brain.GetNeighbors(ctx, t2.ID, "in", "", 10)
	if err != nil {
		t.Fatalf("get neighbors: %v", err)
	}
	if len(neighbors) != 0 {
		t.Errorf("expected 0 edges after thought delete, got %d", len(neighbors))
	}
}

func TestGraphStats(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	// Empty stats
	stats, err := brain.GraphStats(ctx)
	if err != nil {
		t.Fatalf("graph stats: %v", err)
	}
	if stats.TotalEdges != 0 {
		t.Errorf("expected 0 edges, got %d", stats.TotalEdges)
	}

	// Add some thoughts and edges
	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	t3 := &Thought{Content: "Thought C"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)
	brain.Store(ctx, t3)

	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "references"})
	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t3.ID, RelationType: "causes"})
	brain.CreateEdge(ctx, &Edge{SourceID: t2.ID, TargetID: t3.ID, RelationType: "references", AutoExtracted: true})

	stats, err = brain.GraphStats(ctx)
	if err != nil {
		t.Fatalf("graph stats: %v", err)
	}
	if stats.TotalEdges != 3 {
		t.Errorf("expected 3 edges, got %d", stats.TotalEdges)
	}
	if stats.AutoExtracted != 1 {
		t.Errorf("expected 1 auto-extracted edge, got %d", stats.AutoExtracted)
	}
	if stats.ManualEdges != 2 {
		t.Errorf("expected 2 manual edges, got %d", stats.ManualEdges)
	}
	if stats.EdgesByType["references"] != 2 {
		t.Errorf("expected 2 'references' edges, got %d", stats.EdgesByType["references"])
	}
	if stats.EdgesByType["causes"] != 1 {
		t.Errorf("expected 1 'causes' edge, got %d", stats.EdgesByType["causes"])
	}
}

func TestUniqueEdgeConstraint(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)

	// Create first edge
	brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "references"})

	// Try to create duplicate — should fail
	err := brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "references"})
	if err == nil {
		t.Error("expected error for duplicate edge")
	}

	// Same thoughts but different relation type — should succeed
	err = brain.CreateEdge(ctx, &Edge{SourceID: t1.ID, TargetID: t2.ID, RelationType: "causes"})
	if err != nil {
		t.Errorf("expected success for different relation type, got: %v", err)
	}
}

func TestAutoExtractedEdgeFlag(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)

	// Create auto-extracted edge
	edge := &Edge{
		SourceID:      t1.ID,
		TargetID:      t2.ID,
		RelationType:  "launched",
		AutoExtracted: true,
	}
	brain.CreateEdge(ctx, edge)

	// Retrieve and verify
	retrieved, err := brain.GetEdge(ctx, edge.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if !retrieved.AutoExtracted {
		t.Error("expected auto_extracted flag to be true")
	}

	// Create manual edge
	edge2 := &Edge{
		SourceID:     t1.ID,
		TargetID:     t2.ID,
		RelationType: "manual_ref",
	}
	brain.CreateEdge(ctx, edge2)

	retrieved2, err := brain.GetEdge(ctx, edge2.ID)
	if err != nil {
		t.Fatalf("get edge2: %v", err)
	}
	if retrieved2.AutoExtracted {
		t.Error("expected auto_extracted flag to be false for manual edge")
	}
}

func TestEdgeWeight(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	t1 := &Thought{Content: "Thought A"}
	t2 := &Thought{Content: "Thought B"}
	brain.Store(ctx, t1)
	brain.Store(ctx, t2)

	edge := &Edge{
		SourceID:     t1.ID,
		TargetID:     t2.ID,
		RelationType: "references",
		Weight:       0.75,
	}
	brain.CreateEdge(ctx, edge)

	retrieved, err := brain.GetEdge(ctx, edge.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if retrieved.Weight != 0.75 {
		t.Errorf("expected weight 0.75, got %f", retrieved.Weight)
	}
}