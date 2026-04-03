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
			mcp.WithDescription("CRITICAL: Store observations, facts, and decisions to your memory. Call this OFTEN - after every significant action, discovery, or decision. Do NOT wait until the end of a conversation. The client is responsible for classifying and extracting metadata before calling this tool."),
			mcp.WithString("content", mcp.Required(), mcp.Description("The thought content to store. Be SPECIFIC: include file names, function names, exact values, error messages. Dense, factual observations only.")),
			mcp.WithArray("people", mcp.Description("People mentioned in the thought (e.g., ['Alice', 'Bob'])")),
			mcp.WithArray("topics", mcp.Description("Topics or themes (e.g., ['auth', 'refactor', 'bugfix'])")),
			mcp.WithString("type", mcp.Description("Type of thought: decision (you made a choice), insight (you learned something), meeting (conversation summary), person_note (info about someone), idea (potential approach), task (work to do), observation (what you noticed)")),
			mcp.WithArray("action_items", mcp.Description("Action items extracted from the thought (e.g., ['Fix timeout issue', 'Update documentation'])")),
			mcp.WithString("source", mcp.Description("Where this was captured: claude, cursor, cli, slack, etc.")),
			mcp.WithString("namespace", mcp.Description("Namespace for multi-tenant memory spaces (e.g., 'project-alpha', 'team-beta'). Defaults to 'default'.")),
		),
		storeThoughtHandler(brain),
	)

	// semantic_search
	s.AddTool(
		mcp.NewTool("semantic_search",
			mcp.WithDescription("Search your memory for relevant thoughts, observations, and facts. Use this BEFORE asking the user to repeat information they may have already told you. Searches by semantic meaning, not just keywords. Supports natural time filters like 'today', 'yesterday', 'last week', '3 days ago' in the query, or use explicit time_filter parameter."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Describe what you're looking for in natural language. Be specific about context, not just keywords. Example: 'What was the decision about auth timeout?' not just 'timeout'. You can include time expressions like 'today', 'yesterday', 'last week' which will be automatically extracted.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 10)")),
			mcp.WithString("type", mcp.Description("Filter by thought type: decision, insight, meeting, person_note, idea, task, observation. Leave empty to search all types.")),
			mcp.WithArray("topics", mcp.Description("Filter by topics - only return thoughts that have ALL specified topics. Example: ['auth', 'security'] returns thoughts tagged with both auth AND security.")),
			mcp.WithArray("people", mcp.Description("Filter by people mentioned - only return thoughts that mention ALL specified people. Example: ['Alice', 'Bob'] returns thoughts mentioning both Alice AND Bob.")),
			mcp.WithString("before", mcp.Description("Filter thoughts created before this ISO8601 datetime. Example: 2024-01-15T10:30:00Z")),
			mcp.WithString("after", mcp.Description("Filter thoughts created after this ISO8601 datetime. Example: 2024-01-01T00:00:00Z")),
			mcp.WithString("time_filter", mcp.Description("Optional time filter for temporal queries. Supports: today, yesterday, this week, last week, this month, last month, N days/weeks/months ago, YYYY-MM-DD. Can also embed time expressions in the query itself (e.g., 'decisions from last week').")),
			mcp.WithString("namespace", mcp.Description("Filter by namespace. Leave empty to search across all namespaces.")),
		),
		semanticSearchHandler(brain),
	)

	// list_recent
	s.AddTool(
		mcp.NewTool("list_recent",
			mcp.WithDescription("List your recent observations and thoughts. Use this to review what you've learned and done recently, or to find something you stored earlier today."),
			mcp.WithString("since", mcp.Description("ISO8601 datetime to list thoughts from (default: 7 days ago). Example: 2024-01-15T10:30:00Z")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 20)")),
			mcp.WithString("type", mcp.Description("Filter by thought type: decision, insight, meeting, person_note, idea, task, observation. Leave empty for all types.")),
			mcp.WithString("namespace", mcp.Description("Filter by namespace. Leave empty to list across all namespaces.")),
		),
		listRecentHandler(brain),
	)

	// stats
	s.AddTool(
		mcp.NewTool("stats",
			mcp.WithDescription("Get statistics about your memory: total thoughts stored, recent activity, top topics, and sources. Use this to check if you're storing enough observations."),
			mcp.WithString("namespace", mcp.Description("Filter by namespace. Leave empty for stats across all namespaces.")),
		),
		statsHandler(brain),
	)

	// bulk_import
	s.AddTool(
		mcp.NewTool("bulk_import",
			mcp.WithDescription("Import multiple thoughts from JSONL format. Useful for migrating data from other systems or batch-loading historical notes. Each line is a JSON object. Embeddings are generated automatically."),
			mcp.WithString("jsonl", mcp.Required(), mcp.Description("JSONL content — one thought per line as JSON. Required fields: content. Optional: people, topics, type, action_items, source. Example: {'content': 'Fact here', 'type': 'observation', 'topics': ['project']}")),
		),
		bulkImportHandler(brain),
	)

	// delete_thought
	s.AddTool(
		mcp.NewTool("delete_thought",
			mcp.WithDescription("Delete a thought by ID. Normally used during reflection to remove stale observations. Use with caution - deletions are permanent."),
			mcp.WithString("id", mcp.Required(), mcp.Description("The ID of the thought to delete")),
		),
		deleteThoughtHandler(brain),
	)

	// reflect
	s.AddTool(
		mcp.NewTool("reflect",
			mcp.WithDescription("Consolidate and compress your observations. Run this periodically (after 20+ observations or at session end) to merge related thoughts and remove stale information. This keeps your memory efficient and relevant."),
			mcp.WithArray("delete_ids", mcp.Required(), mcp.Description("IDs of thoughts to delete (the old observations you're consolidating)")),
			mcp.WithArray("consolidated", mcp.Required(), mcp.Description("New consolidated thoughts to store. Each should be a dense combination of related old thoughts. Format: [{content: '...', people: [], topics: [], type: '...', action_items: [], source: '...'}]")),
		),
		reflectHandler(brain),
	)

	// health
	s.AddTool(
		mcp.NewTool("health",
			mcp.WithDescription("Check if picobrain is running and healthy. Use this to verify connectivity before starting work."),
		),
		healthHandler(brain),
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
			Namespace:   request.GetString("namespace", ""),
			People:      stringSliceArg(request, "people"),
			Topics:      stringSliceArg(request, "topics"),
			ActionItems: stringSliceArg(request, "action_items"),
		}

		if err := brain.Store(ctx, t); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to store thought: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]string{
			"id":        t.ID,
			"status":    "stored",
			"namespace": t.Namespace,
			"message":   fmt.Sprintf("Thought stored with ID %s in namespace '%s'", t.ID, t.Namespace),
			"reminder":  "Continue storing observations after every significant action!",
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
		thoughtType := request.GetString("type", "")
		timeFilter := request.GetString("time_filter", "")

		// Build filters from request parameters
		filters := SearchFilters{
			Type:   thoughtType,
			Topics: stringSliceArg(request, "topics"),
			People: stringSliceArg(request, "people"),
		}

		// Parse date filters if provided
		if beforeStr := request.GetString("before", ""); beforeStr != "" {
			if before, err := time.Parse(time.RFC3339, beforeStr); err == nil {
				filters.Before = before
			}
		}
		if afterStr := request.GetString("after", ""); afterStr != "" {
			if after, err := time.Parse(time.RFC3339, afterStr); err == nil {
				filters.After = after
			}
		}

		// Handle time_filter parameter or extract from query
		var timeRange *TimeRange
		cleanQuery := query

		if timeFilter != "" {
			tr, err := ParseTimeExpression(timeFilter, time.Now())
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid time filter: %v", err)), nil
			}
			timeRange = &tr
			// Override before/after if time_filter is provided
			filters.Before = timeRange.End
			filters.After = timeRange.Start
		} else {
			result := ExtractTimeFilterFromQuery(query, time.Now())
			if result.HasFilter {
				timeRange = &TimeRange{Start: result.Start, End: result.End}
				cleanQuery = result.CleanQuery
				// Override before/after if extracted from query
				filters.Before = timeRange.End
				filters.After = timeRange.Start
			}
		}

		// Check if we need to use the new filtered search or legacy search
		// Legacy search is used when only type is specified (for backward compatibility)
		var results []Thought
		if filters.Type != "" && len(filters.Topics) == 0 && len(filters.People) == 0 && filters.Before.IsZero() && filters.After.IsZero() {
			// Use legacy search for backward compatibility
			results, err = brain.Search(ctx, cleanQuery, limit, filters.Type, nil)
		} else if len(filters.Topics) > 0 || len(filters.People) > 0 || !filters.Before.IsZero() || !filters.After.IsZero() || filters.Type != "" {
			// Use new filtered search when any filter is specified
			results, err = brain.SearchWithFilters(ctx, cleanQuery, limit, filters)
		} else {
			// No filters at all - use legacy search
			results, err = brain.Search(ctx, cleanQuery, limit, "", nil)
		}
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
		thoughtType := request.GetString("type", "")

		results, err := brain.ListRecent(ctx, since, limit, thoughtType)
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

func deleteThoughtHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		if err := brain.Delete(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"deleted": true,
			"id":      id,
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func reflectHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		deleteIDs := stringSliceArg(request, "delete_ids")
		if len(deleteIDs) == 0 {
			return mcp.NewToolResultError("delete_ids is required and must not be empty"), nil
		}

		consolidatedRaw, ok := request.GetArguments()["consolidated"]
		if !ok {
			return mcp.NewToolResultError("consolidated is required"), nil
		}

		consolidatedArr, ok := consolidatedRaw.([]any)
		if !ok || len(consolidatedArr) == 0 {
			return mcp.NewToolResultError("consolidated must be a non-empty array"), nil
		}

		newThoughts := make([]*Thought, 0, len(consolidatedArr))
		for i, item := range consolidatedArr {
			obj, ok := item.(map[string]any)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("consolidated[%d] must be an object", i)), nil
			}

			content, _ := obj["content"].(string)
			if content == "" {
				return mcp.NewToolResultError(fmt.Sprintf("consolidated[%d].content is required", i)), nil
			}

			t := &Thought{
				Content: content,
				Type:    getStringFromMap(obj, "type"),
				Source:  getStringFromMap(obj, "source"),
			}
			if people, ok := obj["people"].([]any); ok {
				t.People = toStringSlice(people)
			}
			if topics, ok := obj["topics"].([]any); ok {
				t.Topics = toStringSlice(topics)
			}
			if items, ok := obj["action_items"].([]any); ok {
				t.ActionItems = toStringSlice(items)
			}

			newThoughts = append(newThoughts, t)
		}

		result, err := brain.Reflect(ctx, deleteIDs, newThoughts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("reflect failed: %v", err)), nil
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func healthHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stats, err := brain.Stats(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("health check failed: %v", err)), nil
		}

		result, _ := json.Marshal(map[string]any{
			"status":         "healthy",
			"total_thoughts": stats.TotalThoughts,
			"message":        "Picobrain is running and ready to store your observations",
		})
		return mcp.NewToolResultText(string(result)), nil
	}
}

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toStringSlice(arr []any) []string {
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
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
