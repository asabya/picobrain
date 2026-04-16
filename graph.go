package picobrain

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Edge represents a directed relationship between two thoughts.
type Edge struct {
	ID            string            `json:"id"`
	SourceID      string            `json:"source_id"`
	TargetID      string            `json:"target_id"`
	RelationType  string            `json:"relation_type"`
	Weight        float64           `json:"weight,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AutoExtracted bool              `json:"auto_extracted,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
}

// PathStep represents a single step in a path between thoughts.
type PathStep struct {
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	EdgeID    string `json:"edge_id"`
	Relation  string `json:"relation"`
	Depth     int    `json:"depth"`
}

// GraphStats contains statistics about the graph.
type GraphStats struct {
	TotalEdges    int            `json:"total_edges"`
	EdgesByType   map[string]int `json:"edges_by_type"`
	AutoExtracted int            `json:"auto_extracted"`
	ManualEdges   int            `json:"manual_edges"`
	AvgDegree     float64        `json:"avg_degree"`
}

func insertEdgeTx(exec dbExecer, e *Edge) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.Weight == 0 {
		e.Weight = 1.0
	}

	autoInt := 0
	if e.AutoExtracted {
		autoInt = 1
	}

	var metadataJSON []byte
	if e.Metadata != nil {
		metadataJSON, _ = json.Marshal(e.Metadata)
	}

	_, err := exec.Exec(`
		INSERT INTO thought_edges (id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.SourceID, e.TargetID, e.RelationType, e.Weight, string(metadataJSON), autoInt, e.CreatedAt.Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

func deleteEdgeTx(exec dbExecer, id string) error {
	_, err := exec.Exec("DELETE FROM thought_edges WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}
	return nil
}

func deleteEdgesByThoughtTx(exec dbExecer, thoughtID string) error {
	_, err := exec.Exec("DELETE FROM thought_edges WHERE source_id = ? OR target_id = ?", thoughtID, thoughtID)
	if err != nil {
		return fmt.Errorf("delete edges for thought %s: %w", thoughtID, err)
	}
	return nil
}

func getEdge(db *sql.DB, id string) (*Edge, error) {
	var e Edge
	var metadataStr sql.NullString
	var autoInt int
	var createdAt string

	err := db.QueryRow(`
		SELECT id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at
		FROM thought_edges WHERE id = ?
	`, id).Scan(&e.ID, &e.SourceID, &e.TargetID, &e.RelationType, &e.Weight, &metadataStr, &autoInt, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get edge %s: %w", id, err)
	}

	if metadataStr.Valid && metadataStr.String != "" {
		json.Unmarshal([]byte(metadataStr.String), &e.Metadata)
	}
	e.AutoExtracted = autoInt == 1
	e.CreatedAt = parseTime(createdAt)
	return &e, nil
}

func getNeighbors(db *sql.DB, thoughtID string, direction string, relationType string, limit int) ([]Edge, error) {
	if limit <= 0 {
		limit = 20
	}

	var query string
	args := []any{thoughtID}

	switch direction {
	case "out":
		query = `
			SELECT id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at
			FROM thought_edges WHERE source_id = ?`
	case "in":
		query = `
			SELECT id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at
			FROM thought_edges WHERE target_id = ?`
	default: // "both"
		query = `
			SELECT id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at
			FROM thought_edges WHERE source_id = ? OR target_id = ?`
		args = append(args, thoughtID)
	}

	if relationType != "" {
		query += " AND relation_type = ?"
		args = append(args, relationType)
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get neighbors: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func findPath(db *sql.DB, sourceID, targetID string, maxDepth int) ([]PathStep, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	// Recursive CTE with cycle detection.
	// Returns ALL edges along ALL paths, then we filter for the shortest path to target.
	query := `
		WITH RECURSIVE path_search(edge_id, from_id, to_id, relation, depth, path_ids, path_seq) AS (
			SELECT id, source_id, target_id, relation_type, 1,
				',' || source_id || ',' || target_id || ',',
				',' || id || ','
			FROM thought_edges
			WHERE source_id = ?
			UNION ALL
			SELECT e.id, e.source_id, e.target_id, e.relation_type, p.depth + 1,
				p.path_ids || e.target_id || ',',
				p.path_seq || e.id || ','
			FROM thought_edges e, path_search p
			WHERE e.source_id = p.to_id
				AND p.depth < ?
				AND p.path_ids NOT LIKE '%,' || e.target_id || ',%'
		)
		SELECT path_seq
		FROM path_search
		WHERE to_id = ?
		ORDER BY depth
		LIMIT 1`

	var pathSeq string
	err := db.QueryRow(query, sourceID, maxDepth, targetID).Scan(&pathSeq)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find path: %w", err)
	}

	// Parse the path sequence (comma-separated edge IDs)
	// path_seq format: ",edge1,edge2,"
	edgeIDs := splitPathSeq(pathSeq)
	if len(edgeIDs) == 0 {
		return nil, nil
	}

	// Fetch the actual edges in order
	steps := make([]PathStep, 0, len(edgeIDs))
	for i, edgeID := range edgeIDs {
		var s PathStep
		err := db.QueryRow(`
			SELECT id, source_id, target_id, relation_type
			FROM thought_edges WHERE id = ?
		`, edgeID).Scan(&s.EdgeID, &s.FromID, &s.ToID, &s.Relation)
		if err != nil {
			continue
		}
		s.Depth = i + 1
		steps = append(steps, s)
	}

	return steps, nil
}

// splitPathSeq parses ",id1,id2,id3," format into a slice of IDs.
func splitPathSeq(seq string) []string {
	var ids []string
	start := -1
	for i, c := range seq {
		if c == ',' {
			if start >= 0 && i > start {
				ids = append(ids, seq[start:i])
			}
			start = i + 1
		}
	}
	return ids
}

func getGraphStats(db *sql.DB) (*GraphStats, error) {
	stats := &GraphStats{
		EdgesByType: make(map[string]int),
	}

	err := db.QueryRow("SELECT COUNT(*) FROM thought_edges").Scan(&stats.TotalEdges)
	if err != nil {
		return nil, fmt.Errorf("count edges: %w", err)
	}

	if stats.TotalEdges == 0 {
		return stats, nil
	}

	// Count by type
	typeRows, err := db.Query(`
		SELECT relation_type, COUNT(*) as cnt
		FROM thought_edges
		GROUP BY relation_type
		ORDER BY cnt DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("edges by type: %w", err)
	}
	defer typeRows.Close()

	for typeRows.Next() {
		var relType string
		var cnt int
		if err := typeRows.Scan(&relType, &cnt); err != nil {
			return nil, fmt.Errorf("scan edge type: %w", err)
		}
		stats.EdgesByType[relType] = cnt
	}

	// Auto vs manual
	err = db.QueryRow("SELECT COUNT(*) FROM thought_edges WHERE auto_extracted = 1").Scan(&stats.AutoExtracted)
	if err != nil {
		return nil, fmt.Errorf("count auto edges: %w", err)
	}
	stats.ManualEdges = stats.TotalEdges - stats.AutoExtracted

	// Average degree: total edges * 2 / total distinct nodes
	var distinctNodes int
	err = db.QueryRow(`
		SELECT COUNT(DISTINCT node_id) FROM (
			SELECT source_id AS node_id FROM thought_edges
			UNION
			SELECT target_id AS node_id FROM thought_edges
		)
	`).Scan(&distinctNodes)
	if err != nil {
		return nil, fmt.Errorf("count distinct nodes: %w", err)
	}
	if distinctNodes > 0 {
		stats.AvgDegree = float64(stats.TotalEdges*2) / float64(distinctNodes)
	}

	return stats, nil
}

func scanEdges(rows *sql.Rows) ([]Edge, error) {
	var edges []Edge
	for rows.Next() {
		var e Edge
		var metadataStr sql.NullString
		var autoInt int
		var createdAt string

		err := rows.Scan(&e.ID, &e.SourceID, &e.TargetID, &e.RelationType, &e.Weight, &metadataStr, &autoInt, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}

		if metadataStr.Valid && metadataStr.String != "" {
			json.Unmarshal([]byte(metadataStr.String), &e.Metadata)
		}
		e.AutoExtracted = autoInt == 1
		e.CreatedAt = parseTime(createdAt)

		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func getEdgesForExport(db *sql.DB) ([]Edge, error) {
	rows, err := db.Query(`
		SELECT id, source_id, target_id, relation_type, weight, metadata, auto_extracted, created_at
		FROM thought_edges
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("get edges for export: %w", err)
	}
	defer rows.Close()
	return scanEdges(rows)
}