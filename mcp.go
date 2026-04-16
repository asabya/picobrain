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
	s.AddTool(
		mcp.NewTool("store_thought",
			mcp.WithDescription("Store a structured thought record with a summary plus one or more atomic claims."),
			mcp.WithString("summary", mcp.Required(), mcp.Description("Human-readable summary of the record.")),
			mcp.WithArray("claims", mcp.Required(), mcp.Description("Atomic claims for the record. Each item must include subject, predicate, object, polarity, cardinality, and status.")),
			mcp.WithArray("people", mcp.Description("People mentioned in the record.")),
			mcp.WithArray("topics", mcp.Description("Topics for grouping and index output.")),
			mcp.WithString("type", mcp.Description("Record type, such as decision, insight, observation, or task.")),
			mcp.WithString("source", mcp.Description("Source system for the record.")),
			mcp.WithString("namespace", mcp.Description("Namespace for the record. Defaults to the configured namespace.")),
		),
		storeThoughtHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("semantic_search",
			mcp.WithDescription("Search records semantically within a namespace."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Natural language query.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 10).")),
			mcp.WithString("type", mcp.Description("Optional type filter.")),
			mcp.WithArray("topics", mcp.Description("Optional topic filters. All topics must match.")),
			mcp.WithArray("people", mcp.Description("Optional people filters. All people must match.")),
			mcp.WithString("before", mcp.Description("Optional RFC3339 upper time bound.")),
			mcp.WithString("after", mcp.Description("Optional RFC3339 lower time bound.")),
			mcp.WithString("time_filter", mcp.Description("Optional natural-language time filter.")),
			mcp.WithString("namespace", mcp.Description("Namespace to search. Defaults to the configured namespace.")),
		),
		semanticSearchHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("list_recent",
			mcp.WithDescription("List recent records within a namespace."),
			mcp.WithString("since", mcp.Description("RFC3339 lower bound. Defaults to 7 days ago.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 20).")),
			mcp.WithString("type", mcp.Description("Optional type filter.")),
			mcp.WithString("namespace", mcp.Description("Namespace to list. Defaults to the configured namespace.")),
		),
		listRecentHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("stats",
			mcp.WithDescription("Get namespace-scoped memory statistics."),
			mcp.WithString("namespace", mcp.Description("Namespace to summarize. Defaults to the configured namespace.")),
		),
		statsHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("bulk_import",
			mcp.WithDescription("Import canonical JSONL records transactionally. Records must already include thought IDs and claim IDs."),
			mcp.WithString("jsonl", mcp.Required(), mcp.Description("Canonical JSONL payload.")),
			mcp.WithString("namespace", mcp.Description("Optional default namespace applied when an imported record omits namespace.")),
		),
		bulkImportHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("delete_thought",
			mcp.WithDescription("Delete a record by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Record ID to delete.")),
		),
		deleteThoughtHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("reflect",
			mcp.WithDescription("Atomically delete old records and store new consolidated claim-bearing records."),
			mcp.WithArray("delete_ids", mcp.Required(), mcp.Description("IDs of thoughts to delete.")),
			mcp.WithArray("consolidated", mcp.Required(), mcp.Description("Consolidated structured records.")),
			mcp.WithString("namespace", mcp.Description("Optional default namespace applied when a consolidated record omits namespace.")),
		),
		reflectHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("lint",
			mcp.WithDescription("Run deterministic lint checks for contradictions, duplicates, superseded claims, and orphans within a namespace."),
			mcp.WithString("namespace", mcp.Description("Namespace to lint. Defaults to the configured namespace.")),
		),
		lintHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("index",
			mcp.WithDescription("Generate a structured on-demand index of records within a namespace."),
			mcp.WithString("namespace", mcp.Description("Namespace to index. Defaults to the configured namespace.")),
		),
		indexHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("health", mcp.WithDescription("Check whether picobrain is healthy.")),
		healthHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("create_edge",
			mcp.WithDescription("Create a directed relationship between two records."),
			mcp.WithString("source_id", mcp.Required(), mcp.Description("Source thought ID.")),
			mcp.WithString("target_id", mcp.Required(), mcp.Description("Target thought ID.")),
			mcp.WithString("relation_type", mcp.Required(), mcp.Description("Relationship type.")),
			mcp.WithNumber("weight", mcp.Description("Edge weight/strength 0.0-1.0 (default 1.0).")),
		),
		createEdgeHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("delete_edge",
			mcp.WithDescription("Remove a relationship between records."),
			mcp.WithString("edge_id", mcp.Required(), mcp.Description("The edge ID to delete.")),
		),
		deleteEdgeHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("get_neighbors",
			mcp.WithDescription("Get neighboring records for a thought ID."),
			mcp.WithString("thought_id", mcp.Required(), mcp.Description("Thought ID to inspect.")),
			mcp.WithString("direction", mcp.Description("Direction: out, in, or both.")),
			mcp.WithString("relation_type", mcp.Description("Optional relation filter.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of edges to return (default: 20).")),
		),
		getNeighborsHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("find_path",
			mcp.WithDescription("Find a graph path between two records."),
			mcp.WithString("source_id", mcp.Required(), mcp.Description("Start thought ID.")),
			mcp.WithString("target_id", mcp.Required(), mcp.Description("Target thought ID.")),
			mcp.WithNumber("max_depth", mcp.Description("Maximum path depth (default: 5).")),
		),
		findPathHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("graph_stats", mcp.WithDescription("Get graph statistics.")),
		graphStatsHandler(brain),
	)

	s.AddTool(
		mcp.NewTool("extract_triples",
			mcp.WithDescription("Extract dependency triples from text."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to parse.")),
			mcp.WithString("thought_id", mcp.Description("Optional thought ID to link extracted triples to.")),
		),
		extractTriplesHandler(brain),
	)
}

func storeThoughtHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		summary, err := request.RequireString("summary")
		if err != nil {
			return jsonToolError("validation_failed", "summary", "is required"), nil
		}
		claims, err := claimsFromRaw(request.GetArguments()["claims"])
		if err != nil {
			return validationToolError(err), nil
		}
		thought := &Thought{
			Summary:   summary,
			Claims:    claims,
			Type:      request.GetString("type", ""),
			Source:    request.GetString("source", ""),
			Namespace: request.GetString("namespace", ""),
			People:    stringSliceArg(request, "people"),
			Topics:    stringSliceArg(request, "topics"),
		}
		if err := brain.store(ctx, thought, true, false); err != nil {
			return validationToolError(err), nil
		}
		return jsonToolResult(map[string]any{
			"id":          thought.ID,
			"namespace":   thought.Namespace,
			"claim_ids":   thought.claimIDs(),
			"claim_count": len(thought.Claims),
			"status":      "stored",
		}), nil
	}
}

func semanticSearchHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return jsonToolError("validation_failed", "query", "is required"), nil
		}
		limit := request.GetInt("limit", 10)
		filters := SearchFilters{
			Type:      request.GetString("type", ""),
			Topics:    stringSliceArg(request, "topics"),
			People:    stringSliceArg(request, "people"),
			Namespace: request.GetString("namespace", brain.defaultNamespace()),
		}
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
		if timeFilter := request.GetString("time_filter", ""); timeFilter != "" {
			tr, err := ParseTimeExpression(timeFilter, time.Now())
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid time filter: %v", err)), nil
			}
			filters.Before = tr.End
			filters.After = tr.Start
		} else {
			result := ExtractTimeFilterFromQuery(query, time.Now())
			if result.HasFilter {
				query = result.CleanQuery
				filters.Before = result.End
				filters.After = result.Start
			}
		}
		results, err := brain.SearchWithFilters(ctx, query, limit, filters)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}
		out, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func listRecentHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		since := time.Now().Add(-7 * 24 * time.Hour)
		if sinceStr := request.GetString("since", ""); sinceStr != "" {
			if parsed, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				since = parsed
			}
		}
		results, err := brain.ListRecentWithNamespace(ctx, since, request.GetInt("limit", 20), request.GetString("type", ""), request.GetString("namespace", brain.defaultNamespace()))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list recent failed: %v", err)), nil
		}
		out, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func statsHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stats, err := brain.StatsByNamespace(ctx, request.GetString("namespace", brain.defaultNamespace()))
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
			return jsonToolError("validation_failed", "jsonl", "is required"), nil
		}
		results, err := brain.BulkImportDetailed(ctx, strings.NewReader(jsonl), request.GetString("namespace", ""))
		if err != nil {
			return validationToolError(err), nil
		}
		return jsonToolResult(map[string]any{
			"status":       "imported",
			"namespace":    request.GetString("namespace", brain.defaultNamespace()),
			"stored":       results,
			"stored_count": len(results),
		}), nil
	}
}

func deleteThoughtHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return jsonToolError("validation_failed", "id", "is required"), nil
		}
		if err := brain.Delete(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}
		return jsonToolResult(map[string]any{"deleted": true, "id": id}), nil
	}
}

func reflectHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		deleteIDs := stringSliceArg(request, "delete_ids")
		if len(deleteIDs) == 0 {
			return jsonToolError("validation_failed", "delete_ids", "must not be empty"), nil
		}
		consolidatedRaw, ok := request.GetArguments()["consolidated"]
		if !ok {
			return jsonToolError("validation_failed", "consolidated", "is required"), nil
		}
		consolidated, ok := consolidatedRaw.([]any)
		if !ok || len(consolidated) == 0 {
			return jsonToolError("validation_failed", "consolidated", "must be a non-empty array"), nil
		}
		defaultNamespace := request.GetString("namespace", "")
		newThoughts := make([]*Thought, 0, len(consolidated))
		batchClaims := map[string]Claim{}
		for i, item := range consolidated {
			thought, err := thoughtFromMap(item, defaultNamespace)
			if err != nil {
				return validationToolError(annotateRecordError(i, err)), nil
			}
			if err := prepareThoughtForStorage(thought, brain.defaultNamespace(), true, false); err != nil {
				return validationToolError(annotateRecordError(i, err)), nil
			}
			emb, err := brain.embedder.Embed(ctx, thought.canonicalText())
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("generate embedding: %v", err)), nil
			}
			thought.Embedding = emb
			newThoughts = append(newThoughts, thought)
			for _, claim := range thought.Claims {
				batchClaims[claim.ID] = claim
			}
		}
		tx, err := brain.db.Begin()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("begin transaction: %v", err)), nil
		}
		defer tx.Rollback()
		for i, thought := range newThoughts {
			if err := validateSupersessionReferences(tx, thought.Namespace, thought.Claims, batchClaims); err != nil {
				return validationToolError(annotateRecordError(i, err)), nil
			}
		}
		for _, id := range deleteIDs {
			if err := deleteThoughtTx(tx, id); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("delete thought %s: %v", id, err)), nil
			}
		}
		for _, thought := range newThoughts {
			if err := insertPreparedThoughtTx(tx, thought); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("insert reflected thought: %v", err)), nil
			}
		}
		if err := tx.Commit(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("commit reflection: %v", err)), nil
		}
		stored := make([]map[string]any, 0, len(newThoughts))
		for _, thought := range newThoughts {
			brain.cache.Put(*thought)
			stored = append(stored, map[string]any{"id": thought.ID, "claim_ids": thought.claimIDs()})
		}
		return jsonToolResult(map[string]any{
			"status":    "reflected",
			"namespace": request.GetString("namespace", brain.defaultNamespace()),
			"deleted":   deleteIDs,
			"stored":    stored,
		}), nil
	}
}

func lintHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		issues, err := brain.Lint(ctx, request.GetString("namespace", brain.defaultNamespace()))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("lint failed: %v", err)), nil
		}
		out, _ := json.MarshalIndent(issues, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func indexHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		index, err := brain.Index(ctx, request.GetString("namespace", brain.defaultNamespace()))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("index failed: %v", err)), nil
		}
		out, _ := json.MarshalIndent(index, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}
}

func healthHandler(brain *Brain) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stats, err := brain.StatsByNamespace(ctx, brain.defaultNamespace())
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("health check failed: %v", err)), nil
		}
		return jsonToolResult(map[string]any{
			"status":         "healthy",
			"namespace":      brain.defaultNamespace(),
			"total_thoughts": stats.TotalThoughts,
		}), nil
	}
}

func thoughtFromMap(raw any, defaultNamespace string) (*Thought, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, validationError("consolidated", "must contain objects")
	}
	claims, err := claimsFromRaw(obj["claims"])
	if err != nil {
		return nil, err
	}
	return &Thought{
		Summary:   getStringFromMap(obj, "summary"),
		Claims:    claims,
		Type:      getStringFromMap(obj, "type"),
		Source:    getStringFromMap(obj, "source"),
		Namespace: coalesceString(getStringFromMap(obj, "namespace"), defaultNamespace),
		People:    anyStringSlice(obj["people"]),
		Topics:    anyStringSlice(obj["topics"]),
	}, nil
}

func claimsFromRaw(raw any) ([]Claim, error) {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, validationError("claims", "must not be empty")
	}
	claims := make([]Claim, 0, len(arr))
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, validationError(fmt.Sprintf("claims[%d]", i), "must be an object")
		}
		claims = append(claims, Claim{
			Subject:           getStringFromMap(obj, "subject"),
			Predicate:         getStringFromMap(obj, "predicate"),
			Object:            getStringFromMap(obj, "object"),
			Polarity:          getStringFromMap(obj, "polarity"),
			Cardinality:       getStringFromMap(obj, "cardinality"),
			Status:            getStringFromMap(obj, "status"),
			SupersedesClaimID: getStringFromMap(obj, "supersedes_claim_id"),
			Confidence:        getStringFromMap(obj, "confidence"),
		})
	}
	return claims, nil
}

func validationToolError(err error) *mcp.CallToolResult {
	field, message, meta := parseValidationParts(err)
	payload := map[string]any{"error": "validation_failed", "field": field, "message": message}
	for k, v := range meta {
		payload[k] = v
	}
	return jsonToolResult(payload)
}

func parseValidationParts(err error) (string, string, map[string]any) {
	message := err.Error()
	parts := strings.Split(message, ":")
	meta := map[string]any{}
	if len(parts) >= 3 && parts[0] == "validation_failed" {
		for _, extra := range parts[3:] {
			kv := strings.SplitN(extra, "=", 2)
			if len(kv) == 2 {
				meta[kv[0]] = kv[1]
			}
		}
		return parts[1], parts[2], meta
	}
	return "", message, meta
}

func jsonToolError(kind, field, message string) *mcp.CallToolResult {
	return jsonToolResult(map[string]any{"error": kind, "field": field, "message": message})
}

func jsonToolResult(payload map[string]any) *mcp.CallToolResult {
	out, _ := json.Marshal(payload)
	return mcp.NewToolResultText(string(out))
}

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func anyStringSlice(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringSliceArg(req mcp.CallToolRequest, name string) []string {
	return anyStringSlice(req.GetArguments()[name])
}
