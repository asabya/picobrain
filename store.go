package picobrain

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS thoughts (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			people TEXT,
			topics TEXT,
			type TEXT,
			priority TEXT,
			action_items TEXT,
			source TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create thoughts table: %w", err)
	}

	_, err = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS thought_vectors USING vec0(
			id TEXT PRIMARY KEY,
			embedding float[768] distance_metric=cosine
		)
	`)
	if err != nil {
		return fmt.Errorf("create thought_vectors table: %w", err)
	}

	return nil
}

type dbExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertThoughtTx(exec dbExecer, t *Thought) error {
	peopleJSON, _ := json.Marshal(t.People)
	topicsJSON, _ := json.Marshal(t.Topics)
	actionItemsJSON, _ := json.Marshal(t.ActionItems)

	createdAt := t.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := exec.Exec(`
		INSERT INTO thoughts (id, content, people, topics, type, priority, action_items, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Content, string(peopleJSON), string(topicsJSON),
		t.Type, t.Priority, string(actionItemsJSON), t.Source, createdAt)
	if err != nil {
		return fmt.Errorf("insert thought: %w", err)
	}

	vec, err := sqlite_vec.SerializeFloat32(t.Embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}

	_, err = exec.Exec(`
		INSERT INTO thought_vectors (id, embedding)
		VALUES (?, ?)
	`, t.ID, vec)
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return nil
}

func insertThought(db *sql.DB, t *Thought) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := insertThoughtTx(tx, t); err != nil {
		return err
	}

	return tx.Commit()
}

func deleteThoughtTx(exec dbExecer, id string) error {
	if _, err := exec.Exec("DELETE FROM thoughts WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete from thoughts: %w", err)
	}
	if _, err := exec.Exec("DELETE FROM thought_vectors WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete from thought_vectors: %w", err)
	}
	return nil
}

func reflectTx(db *sql.DB, deleteIDs []string, newThoughts []*Thought) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, id := range deleteIDs {
		if err := deleteThoughtTx(tx, id); err != nil {
			return fmt.Errorf("delete thought %s: %w", id, err)
		}
	}

	for _, t := range newThoughts {
		if err := insertThoughtTx(tx, t); err != nil {
			return fmt.Errorf("insert reflected thought: %w", err)
		}
	}

	return tx.Commit()
}

func deleteThought(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := deleteThoughtTx(tx, id); err != nil {
		return err
	}

	return tx.Commit()
}

func getThought(db *sql.DB, id string) (*Thought, error) {
	var t Thought
	var peopleStr, topicsStr, actionItemsStr, priorityStr sql.NullString
	var createdAt string

	err := db.QueryRow(`
		SELECT id, content, people, topics, type, priority, action_items, source, created_at
		FROM thoughts WHERE id = ?
	`, id).Scan(&t.ID, &t.Content, &peopleStr, &topicsStr,
		&t.Type, &priorityStr, &actionItemsStr, &t.Source, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get thought %s: %w", id, err)
	}

	if peopleStr.Valid {
		json.Unmarshal([]byte(peopleStr.String), &t.People)
	}
	if topicsStr.Valid {
		json.Unmarshal([]byte(topicsStr.String), &t.Topics)
	}
	if priorityStr.Valid {
		t.Priority = priorityStr.String
	}
	if actionItemsStr.Valid {
		json.Unmarshal([]byte(actionItemsStr.String), &t.ActionItems)
	}

	t.CreatedAt = parseTime(createdAt)

	return &t, nil
}

func searchByVector(db *sql.DB, embedding []float32, limit int, thoughtType string, timeRange *TimeRange) ([]Thought, error) {
	vec, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	searchLimit := limit
	if thoughtType != "" || timeRange != nil {
		searchLimit = limit * 3
	}

	var rows *sql.Rows
	if timeRange != nil {
		rows, err = db.Query(`
			SELECT v.id, v.distance,
			       t.content, t.people, t.topics, t.type, t.priority, t.action_items, t.source, t.created_at
			FROM thought_vectors v
			JOIN thoughts t ON t.id = v.id
			WHERE v.embedding MATCH ?
			AND k = ?
			AND t.created_at >= ?
			AND t.created_at < ?
			ORDER BY v.distance
		`, vec, searchLimit, timeRange.Start.Format("2006-01-02 15:04:05"), timeRange.End.Format("2006-01-02 15:04:05"))
	} else {
		rows, err = db.Query(`
			SELECT v.id, v.distance,
			       t.content, t.people, t.topics, t.type, t.priority, t.action_items, t.source, t.created_at
			FROM thought_vectors v
			JOIN thoughts t ON t.id = v.id
			WHERE v.embedding MATCH ?
			AND k = ?
			ORDER BY v.distance
		`, vec, searchLimit)
	}
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	all, err := scanThoughts(rows, true)
	if err != nil {
		return nil, err
	}

	if thoughtType == "" {
		if len(all) > limit {
			return all[:limit], nil
		}
		return all, nil
	}

	// Filter by type
	filtered := make([]Thought, 0, limit)
	for _, t := range all {
		if t.Type == thoughtType {
			filtered = append(filtered, t)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

func searchByVectorWithFilters(db *sql.DB, embedding []float32, limit int, filters SearchFilters) ([]Thought, error) {
	vec, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	// Calculate fetch limit - fetch more if we have filters to account for filtered-out results
	fetchLimit := limit
	if filters.Type != "" || len(filters.Topics) > 0 || len(filters.People) > 0 || !filters.Before.IsZero() || !filters.After.IsZero() {
		fetchLimit = limit * 5 // Fetch more to ensure we can apply all filters
	}

	// Build dynamic query with filters applied in SQL
	// This applies filters BEFORE vector ranking for efficiency
	query := `
		SELECT v.id, v.distance,
		       t.content, t.people, t.topics, t.type, t.priority, t.action_items, t.source, t.created_at
		FROM thought_vectors v
		JOIN thoughts t ON t.id = v.id
		WHERE v.embedding MATCH ?
		AND k = ?
	`
	args := []any{vec, fetchLimit}

	// Add filter conditions
	if filters.Type != "" {
		query += " AND t.type = ?"
		args = append(args, filters.Type)
	}

	if !filters.Before.IsZero() {
		query += " AND t.created_at <= ?"
		args = append(args, filters.Before.Format("2006-01-02 15:04:05"))
	}

	if !filters.After.IsZero() {
		query += " AND t.created_at >= ?"
		args = append(args, filters.After.Format("2006-01-02 15:04:05"))
	}

	// For topics and people, we need to check JSON arrays
	// We use json_each to expand the array and check if all required items exist
	for _, topic := range filters.Topics {
		query += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM json_each(t.topics) WHERE value = ?%d
		)`, len(args)+1)
		args = append(args, topic)
	}

	for _, person := range filters.People {
		query += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM json_each(t.people) WHERE value = ?%d
		)`, len(args)+1)
		args = append(args, person)
	}

	query += " ORDER BY v.distance LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search with filters: %w", err)
	}
	defer rows.Close()

	return scanThoughts(rows, true)
}

func listRecent(db *sql.DB, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	var rows *sql.Rows
	var err error
	if thoughtType != "" {
		rows, err = db.Query(`
			SELECT id, content, people, topics, type, priority, action_items, source, created_at
			FROM thoughts
			WHERE created_at >= ? AND type = ?
			ORDER BY created_at DESC
			LIMIT ?
		`, since.Format("2006-01-02 15:04:05"), thoughtType, limit)
	} else {
		rows, err = db.Query(`
			SELECT id, content, people, topics, type, priority, action_items, source, created_at
			FROM thoughts
			WHERE created_at >= ?
			ORDER BY created_at DESC
			LIMIT ?
		`, since.Format("2006-01-02 15:04:05"), limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	defer rows.Close()

	return scanThoughts(rows, false)
}

func getStats(db *sql.DB) (*BrainStats, error) {
	stats := &BrainStats{}

	// Total and this week
	err := db.QueryRow("SELECT COUNT(*) FROM thoughts").Scan(&stats.TotalThoughts)
	if err != nil {
		return nil, fmt.Errorf("count thoughts: %w", err)
	}

	if stats.TotalThoughts == 0 {
		return stats, nil
	}

	err = db.QueryRow(`
		SELECT COUNT(*) FROM thoughts
		WHERE created_at >= datetime('now', '-7 days')
	`).Scan(&stats.ThoughtsThisWeek)
	if err != nil {
		return nil, fmt.Errorf("count this week: %w", err)
	}

	// Top topics using json_each
	topicRows, err := db.Query(`
		SELECT value, COUNT(*) as cnt
		FROM thoughts, json_each(thoughts.topics)
		WHERE value IS NOT NULL
		GROUP BY value
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("top topics: %w", err)
	}
	defer topicRows.Close()

	for topicRows.Next() {
		var topic string
		var cnt int
		if err := topicRows.Scan(&topic, &cnt); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		stats.TopTopics = append(stats.TopTopics, topic)
	}

	// Top sources
	sourceRows, err := db.Query(`
		SELECT source, COUNT(*) as cnt
		FROM thoughts
		WHERE source IS NOT NULL AND source != ''
		GROUP BY source
		ORDER BY cnt DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, fmt.Errorf("top sources: %w", err)
	}
	defer sourceRows.Close()

	for sourceRows.Next() {
		var source string
		var cnt int
		if err := sourceRows.Scan(&source, &cnt); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		stats.TopSources = append(stats.TopSources, source)
	}

	// Date range
	var firstStr, lastStr string
	err = db.QueryRow(`
		SELECT MIN(created_at), MAX(created_at) FROM thoughts
	`).Scan(&firstStr, &lastStr)
	if err != nil {
		return nil, fmt.Errorf("date range: %w", err)
	}
	stats.FirstThought = parseTime(firstStr)
	stats.LastThought = parseTime(lastStr)

	// Average per day
	days := stats.LastThought.Sub(stats.FirstThought).Hours() / 24
	if days < 1 {
		days = 1
	}
	stats.AvgPerDay = float64(stats.TotalThoughts) / days

	return stats, nil
}

func scanThoughts(rows *sql.Rows, withDistance bool) ([]Thought, error) {
	var thoughts []Thought
	for rows.Next() {
		var t Thought
		var peopleStr, topicsStr, actionItemsStr, priorityStr sql.NullString
		var createdAt string

		var err error
		if withDistance {
			err = rows.Scan(&t.ID, &t.Distance,
				&t.Content, &peopleStr, &topicsStr,
				&t.Type, &priorityStr, &actionItemsStr, &t.Source, &createdAt)
		} else {
			err = rows.Scan(&t.ID, &t.Content, &peopleStr, &topicsStr,
				&t.Type, &priorityStr, &actionItemsStr, &t.Source, &createdAt)
		}
		if err != nil {
			return nil, fmt.Errorf("scan thought: %w", err)
		}

		if peopleStr.Valid {
			json.Unmarshal([]byte(peopleStr.String), &t.People)
		}
		if topicsStr.Valid {
			json.Unmarshal([]byte(topicsStr.String), &t.Topics)
		}
		if priorityStr.Valid {
			t.Priority = priorityStr.String
		}
		if actionItemsStr.Valid {
			json.Unmarshal([]byte(actionItemsStr.String), &t.ActionItems)
		}
		t.CreatedAt = parseTime(createdAt)

		thoughts = append(thoughts, t)
	}
	return thoughts, rows.Err()
}

func queryThoughtsWithFilter(db *sql.DB, filter ExportFilter) ([]Thought, error) {
	query := `
		SELECT id, content, people, topics, type, action_items, source, created_at
		FROM thoughts
		WHERE 1=1
	`
	args := []any{}

	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.Since.Format("2006-01-02 15:04:05"))
	}
	if filter.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, filter.Until.Format("2006-01-02 15:04:05"))
	}
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Source != "" {
		query += " AND source = ?"
		args = append(args, filter.Source)
	}
	if len(filter.Topics) > 0 {
		placeholders := make([]string, len(filter.Topics))
		for i := range filter.Topics {
			placeholders[i] = "?"
			args = append(args, filter.Topics[i])
		}
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(topics) WHERE value IN (%s))", strings.Join(placeholders, ","))
	}
	if len(filter.People) > 0 {
		placeholders := make([]string, len(filter.People))
		for i := range filter.People {
			placeholders[i] = "?"
			args = append(args, filter.People[i])
		}
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(people) WHERE value IN (%s))", strings.Join(placeholders, ","))
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query thoughts: %w", err)
	}
	defer rows.Close()

	return scanThoughts(rows, false)
}

func parseTime(s string) time.Time {
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// pruneOldThoughts deletes thoughts older than the given number of days,
// excluding critical priority thoughts. Returns the number of thoughts deleted.
func pruneOldThoughts(db *sql.DB, days int) (int, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	cutoffStr := cutoff.Format("2006-01-02 15:04:05")

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id FROM thoughts
		WHERE created_at < ?
		AND (priority IS NULL OR priority != 'critical')
	`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("query thoughts to prune: %w", err)
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan thought id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows error: %w", err)
	}

	for _, id := range ids {
		if err := deleteThoughtTx(tx, id); err != nil {
			return 0, fmt.Errorf("delete thought %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return len(ids), nil
}
