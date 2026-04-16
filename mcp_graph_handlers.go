package picobrain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func createEdgeHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sourceID, err := request.RequireString("source_id")
		if err != nil {
			return mcp.NewToolResultError("source_id is required"), nil
		}
		targetID, err := request.RequireString("target_id")
		if err != nil {
			return mcp.NewToolResultError("target_id is required"), nil
		}
		relationType, err := request.RequireString("relation_type")
		if err != nil {
			return mcp.NewToolResultError("relation_type is required"), nil
		}

		e := &Edge{
			SourceID:     sourceID,
			TargetID:     targetID,
			RelationType: relationType,
			Weight:       request.GetFloat("weight", 1.0),
		}

		if err := brain.CreateEdge(ctx, e); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create edge failed: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"id":            e.ID,
			"source_id":     e.SourceID,
			"target_id":     e.TargetID,
			"relation_type": e.RelationType,
			"weight":        e.Weight,
			"status":        "created",
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func deleteEdgeHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("edge_id")
		if err != nil {
			return mcp.NewToolResultError("edge_id is required"), nil
		}

		if err := brain.DeleteEdge(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete edge failed: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"deleted": true,
			"id":      id,
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func getNeighborsHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		thoughtID, err := request.RequireString("thought_id")
		if err != nil {
			return mcp.NewToolResultError("thought_id is required"), nil
		}

		direction := request.GetString("direction", "both")
		relationType := request.GetString("relation_type", "")
		limit := request.GetInt("limit", 20)

		edges, err := brain.GetNeighbors(ctx, thoughtID, direction, relationType, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get neighbors failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(edges, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func findPathHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sourceID, err := request.RequireString("source_id")
		if err != nil {
			return mcp.NewToolResultError("source_id is required"), nil
		}
		targetID, err := request.RequireString("target_id")
		if err != nil {
			return mcp.NewToolResultError("target_id is required"), nil
		}

		maxDepth := request.GetInt("max_depth", 5)

		steps, err := brain.FindPath(ctx, sourceID, targetID, maxDepth)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("find path failed: %v", err)), nil
		}

		if len(steps) == 0 {
			result, _ := json.Marshal(map[string]any{
				"found":   false,
				"message": fmt.Sprintf("No path found between %s and %s within depth %d", sourceID, targetID, maxDepth),
			})
			return mcp.NewToolResultText(string(result)), nil
		}

		out, _ := json.MarshalIndent(steps, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func graphStatsHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stats, err := brain.GraphStats(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("graph stats failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(stats, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func extractTriplesHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := request.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError("text is required"), nil
		}

		triples, err := brain.ExtractTriples(ctx, text)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("extract triples failed: %v", err)), nil
		}

		thoughtID := request.GetString("thought_id", "")
		if thoughtID != "" && len(triples) > 0 {
			brain.autoLinkTriples(ctx, thoughtID, triples)
		}

		out, _ := json.MarshalIndent(triples, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}