package picobrain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterMCPTools(s *server.MCPServer, brain *Brain) {
	// store_thought
	s.AddTool(
		mcp.NewTool("store_thought",
			mcp.WithDescription("Store a thought in your brain with optional metadata. The client is responsible for classifying and extracting metadata before calling this tool."),
			mcp.WithString("content", mcp.Required(), mcp.Description("The thought content to store")),
			mcp.WithArray("people", mcp.Description("People mentioned in the thought")),
			mcp.WithArray("topics", mcp.Description("Topics or themes of the thought")),
			mcp.WithString("type", mcp.Description("Type of thought: decision, insight, meeting, person_note, idea, task")),
			mcp.WithArray("action_items", mcp.Description("Action items extracted from the thought")),
			mcp.WithString("source", mcp.Description("Where the thought was captured: slack, claude, cli, cursor, etc.")),
		),
		storeThoughtHandler(brain),
	)

	// semantic_search
	s.AddTool(
		mcp.NewTool("semantic_search",
			mcp.WithDescription("Search your brain for thoughts by meaning. Uses vector similarity to find relevant thoughts even if they don't contain the exact words."),
			mcp.WithString("query", mcp.Required(), mcp.Description("The search query — describe what you're looking for")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 10)")),
		),
		semanticSearchHandler(brain),
	)

	// list_recent
	s.AddTool(
		mcp.NewTool("list_recent",
			mcp.WithDescription("List recently captured thoughts, ordered by newest first."),
			mcp.WithString("since", mcp.Description("ISO8601 datetime to list thoughts from (default: 7 days ago)")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 20)")),
		),
		listRecentHandler(brain),
	)

	// stats
	s.AddTool(
		mcp.NewTool("stats",
			mcp.WithDescription("Get statistics about your brain: total thoughts, top topics, top sources, date range, and average thoughts per day."),
		),
		statsHandler(brain),
	)

	// bulk_import
	s.AddTool(
		mcp.NewTool("bulk_import",
			mcp.WithDescription("Import multiple thoughts from JSONL format. Each line should be a JSON object with: content (required), people, topics, type, action_items, source. Embeddings are generated automatically."),
			mcp.WithString("jsonl", mcp.Required(), mcp.Description("JSONL content — one thought per line as JSON")),
		),
		bulkImportHandler(brain),
	)
}

func storeThoughtHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("content is required"), nil
		}

		t := &Thought{
			Content:     content,
			Type:        request.GetString("type", ""),
			Source:      request.GetString("source", ""),
			People:      stringSliceArg(request, "people"),
			Topics:      stringSliceArg(request, "topics"),
			ActionItems: stringSliceArg(request, "action_items"),
		}

		if err := brain.Store(ctx, t); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to store thought: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]string{
			"id":      t.ID,
			"status":  "stored",
			"message": fmt.Sprintf("Thought stored with ID %s", t.ID),
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func semanticSearchHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query is required"), nil
		}

		limit := request.GetInt("limit", 10)

		results, err := brain.Search(ctx, query, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func listRecentHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sinceStr := request.GetString("since", "")
		since := time.Now().Add(-7 * 24 * time.Hour)
		if sinceStr != "" {
			if parsed, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				since = parsed
			}
		}

		limit := request.GetInt("limit", 20)

		results, err := brain.ListRecent(ctx, since, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list recent failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func statsHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stats, err := brain.Stats(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("stats failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(stats, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func bulkImportHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jsonl, err := request.RequireString("jsonl")
		if err != nil {
			return mcp.NewToolResultError("jsonl is required"), nil
		}

		count, err := brain.BulkImport(ctx, strings.NewReader(jsonl))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("import failed after %d thoughts: %v", count, err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"imported": count,
			"status":   "complete",
			"message":  fmt.Sprintf("Successfully imported %d thoughts", count),
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

// stringSliceArg extracts a string slice from an MCP request argument.
func stringSliceArg(req mcp.CallToolRequest, name string) []string {
	v, ok := req.GetArguments()[name]
	if !ok || v == nil {
		return nil
	}

	arr, ok := v.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
