package picobrain

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS thoughts (
			id TEXT PRIMARY KEY,
			summary TEXT NOT NULL,
			people TEXT,
			topics TEXT,
			type TEXT,
			priority TEXT,
			action_items TEXT,
			source TEXT,
			namespace TEXT DEFAULT 'default',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create thoughts table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS thought_claims (
			id TEXT PRIMARY KEY,
			thought_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			predicate TEXT NOT NULL,
			object_value TEXT NOT NULL,
			polarity TEXT NOT NULL,
			cardinality TEXT NOT NULL,
			status TEXT NOT NULL,
			supersedes_claim_id TEXT,
			confidence TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (thought_id) REFERENCES thoughts(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("create thought_claims table: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_claims_thought_id ON thought_claims(thought_id)`)
	if err != nil {
		return fmt.Errorf("create idx_claims_thought_id: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_claims_subject_predicate ON thought_claims(subject, predicate)`)
	if err != nil {
		return fmt.Errorf("create idx_claims_subject_predicate: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_claims_status ON thought_claims(status)`)
	if err != nil {
		return fmt.Errorf("create idx_claims_status: %w", err)
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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS thought_edges (
			id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation_type TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			metadata TEXT,
			auto_extracted INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (source_id) REFERENCES thoughts(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES thoughts(id) ON DELETE CASCADE,
			UNIQUE(source_id, target_id, relation_type)
		)
	`)
	if err != nil {
		return fmt.Errorf("create thought_edges table: %w", err)
	}

	indexes := []struct {
		query string
		name  string
	}{
		{"CREATE INDEX IF NOT EXISTS idx_edges_source ON thought_edges(source_id)", "idx_edges_source"},
		{"CREATE INDEX IF NOT EXISTS idx_edges_target ON thought_edges(target_id)", "idx_edges_target"},
		{"CREATE INDEX IF NOT EXISTS idx_edges_type ON thought_edges(relation_type)", "idx_edges_type"},
		{"CREATE INDEX IF NOT EXISTS idx_edges_auto ON thought_edges(auto_extracted)", "idx_edges_auto"},
		{"CREATE INDEX IF NOT EXISTS idx_thoughts_namespace ON thoughts(namespace)", "idx_thoughts_namespace"},
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx.query); err != nil {
			return fmt.Errorf("create %s: %w", idx.name, err)
		}
	}

	if err := migrateSchema(db); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}

	return nil
}

func migrateSchema(db *sql.DB) error {
	if err := ensureThoughtColumn(db, "namespace", "TEXT DEFAULT 'default'"); err != nil {
		return fmt.Errorf("ensure namespace column: %w", err)
	}
	if err := ensureThoughtColumn(db, "summary", "TEXT"); err != nil {
		return fmt.Errorf("ensure summary column: %w", err)
	}
	if err := ensureThoughtColumn(db, "updated_at", "TIMESTAMP DEFAULT CURRENT_TIMESTAMP"); err != nil {
		return fmt.Errorf("ensure updated_at column: %w", err)
	}

	if hasColumn(db, "thoughts", "content") {
		if _, err := db.Exec(`UPDATE thoughts SET summary = content WHERE (summary IS NULL OR summary = '') AND content IS NOT NULL`); err != nil {
			return fmt.Errorf("backfill summary from content: %w", err)
		}
	}
	if _, err := db.Exec(`UPDATE thoughts SET updated_at = created_at WHERE updated_at IS NULL OR updated_at = ''`); err != nil {
		return fmt.Errorf("backfill updated_at: %w", err)
	}

	return nil
}

func hasColumn(db *sql.DB, table, column string) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count)
	if err != nil {
		// pragma_table_info doesn't parameterize table name in sqlite, fallback below.
		query := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, table)
		if err2 := db.QueryRow(query, column).Scan(&count); err2 != nil {
			return false
		}
	}
	return count > 0
}

func ensureThoughtColumn(db *sql.DB, column, definition string) error {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('thoughts') WHERE name = ?`)
	var count int
	if err := db.QueryRow(query, column).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE thoughts ADD COLUMN %s %s`, column, definition)); err != nil {
			return err
		}
	}
	return nil
}

type dbExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type dbQueryExecer interface {
	dbExecer
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func insertThoughtTx(exec dbQueryExecer, t *Thought, strict bool) error {
	if err := prepareThoughtForStorage(t, "default", strict, false); err != nil {
		return err
	}
	if err := validateSupersessionReferences(exec, t.Namespace, t.Claims, nil); err != nil {
		return err
	}
	return insertPreparedThoughtTx(exec, t)
}

func insertPreparedThoughtTx(exec dbExecer, t *Thought) error {
	peopleJSON, _ := json.Marshal(t.People)
	topicsJSON, _ := json.Marshal(t.Topics)
	actionItemsJSON, _ := json.Marshal(t.ActionItems)

	_, err := exec.Exec(`
		INSERT INTO thoughts (id, summary, people, topics, type, priority, action_items, source, namespace, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Summary, string(peopleJSON), string(topicsJSON), t.Type, t.Priority, string(actionItemsJSON), t.Source, t.Namespace, formatDBTime(t.CreatedAt), formatDBTime(t.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert thought: %w", err)
	}

	for _, claim := range t.Claims {
		_, err := exec.Exec(`
			INSERT INTO thought_claims (id, thought_id, subject, predicate, object_value, polarity, cardinality, status, supersedes_claim_id, confidence, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, claim.ID, t.ID, claim.Subject, claim.Predicate, claim.Object, claim.Polarity, claim.Cardinality, claim.Status, nullableString(claim.SupersedesClaimID), nullableString(claim.Confidence), formatDBTime(claim.CreatedAt))
		if err != nil {
			return fmt.Errorf("insert claim: %w", err)
		}
	}

	vec, err := sqlite_vec.SerializeFloat32(t.Embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}
	if _, err := exec.Exec(`INSERT INTO thought_vectors (id, embedding) VALUES (?, ?)`, t.ID, vec); err != nil {
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

	if err := insertThoughtTx(tx, t, false); err != nil {
		return err
	}
	return tx.Commit()
}

func prepareThoughtForStorage(t *Thought, defaultNamespace string, strict bool, requireIDs bool) error {
	if t == nil {
		return fmt.Errorf("thought is required")
	}
	t.syncSummaryContent()
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.Namespace == "" {
		t.Namespace = defaultNamespace
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	if t.Summary == "" {
		return validationError("summary", "is required")
	}
	if len(t.Claims) == 0 {
		if strict {
			return validationError("claims", "must not be empty")
		}
		t.Claims = []Claim{{
			ID:          uuid.New().String(),
			Subject:     t.Summary,
			Predicate:   "states",
			Object:      t.Summary,
			Polarity:    "affirmed",
			Cardinality: "many",
			Status:      "active",
			CreatedAt:   t.CreatedAt,
		}}
	}
	for i := range t.Claims {
		claim := &t.Claims[i]
		if claim.ID == "" {
			if requireIDs {
				return validationError(fmt.Sprintf("claims[%d].id", i), "is required")
			}
			claim.ID = uuid.New().String()
		}
		if claim.CreatedAt.IsZero() {
			claim.CreatedAt = t.CreatedAt
		}
		if claim.Subject == "" {
			if strict {
				return validationError(fmt.Sprintf("claims[%d].subject", i), "is required")
			}
			claim.Subject = t.Summary
		}
		if claim.Predicate == "" {
			if strict {
				return validationError(fmt.Sprintf("claims[%d].predicate", i), "is required")
			}
			claim.Predicate = "states"
		}
		if claim.Object == "" {
			if strict {
				return validationError(fmt.Sprintf("claims[%d].object", i), "is required")
			}
			claim.Object = t.Summary
		}
		if claim.Polarity == "" {
			claim.Polarity = "affirmed"
		}
		if claim.Cardinality == "" {
			claim.Cardinality = "many"
		}
		if claim.Status == "" {
			claim.Status = "active"
		}
		if err := validateEnum(fmt.Sprintf("claims[%d].polarity", i), claim.Polarity, []string{"affirmed", "negated"}); err != nil {
			return err
		}
		if err := validateEnum(fmt.Sprintf("claims[%d].cardinality", i), claim.Cardinality, []string{"one", "many"}); err != nil {
			return err
		}
		if err := validateEnum(fmt.Sprintf("claims[%d].status", i), claim.Status, []string{"active", "superseded"}); err != nil {
			return err
		}
		if claim.Confidence != "" {
			if err := validateEnum(fmt.Sprintf("claims[%d].confidence", i), claim.Confidence, []string{"low", "medium", "high"}); err != nil {
				return err
			}
		}
	}
	if t.Embedding == nil {
		t.Embedding = make([]float32, 0)
	}
	return nil
}

func validateEnum(field, value string, allowed []string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return validationError(field, fmt.Sprintf("must be one of %s", strings.Join(allowed, "|")))
}

func validateSupersessionReferences(exec dbQueryExecer, namespace string, claims []Claim, batchClaims map[string]Claim) error {
	for _, claim := range claims {
		if claim.SupersedesClaimID == "" {
			continue
		}
		target, ok := batchClaims[claim.SupersedesClaimID]
		if ok {
			if target.Subject != claim.Subject || target.Predicate != claim.Predicate {
				return validationError("supersedes_claim_id", "must reference a claim with the same subject and predicate")
			}
			continue
		}
		var subject, predicate, targetNamespace string
		err := exec.QueryRow(`
			SELECT tc.subject, tc.predicate, t.namespace
			FROM thought_claims tc
			JOIN thoughts t ON t.id = tc.thought_id
			WHERE tc.id = ?
		`, claim.SupersedesClaimID).Scan(&subject, &predicate, &targetNamespace)
		if err != nil {
			if err == sql.ErrNoRows {
				return validationError("supersedes_claim_id", "must reference an existing claim in the same namespace")
			}
			return fmt.Errorf("lookup supersedes reference: %w", err)
		}
		if targetNamespace != namespace {
			return validationError("supersedes_claim_id", "must reference an existing claim in the same namespace")
		}
		if subject != claim.Subject || predicate != claim.Predicate {
			return validationError("supersedes_claim_id", "must reference a claim with the same subject and predicate")
		}
	}
	return nil
}

func validationError(field, msg string) error {
	return fmt.Errorf("validation_failed:%s:%s", field, msg)
}

func parseValidationError(err error) (field, msg string, ok bool) {
	parts := strings.SplitN(err.Error(), ":", 3)
	if len(parts) != 3 || parts[0] != "validation_failed" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func deleteThoughtTx(exec dbExecer, id string) error {
	if _, err := exec.Exec("DELETE FROM thought_edges WHERE source_id = ? OR target_id = ?", id, id); err != nil {
		return fmt.Errorf("delete from thought_edges: %w", err)
	}
	if _, err := exec.Exec("DELETE FROM thought_claims WHERE thought_id = ?", id); err != nil {
		return fmt.Errorf("delete from thought_claims: %w", err)
	}
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

	batchClaims := map[string]Claim{}
	for i, t := range newThoughts {
		if err := prepareThoughtForStorage(t, "default", false, false); err != nil {
			return fmt.Errorf("prepare reflected thought %d: %w", i, err)
		}
		for _, claim := range t.Claims {
			batchClaims[claim.ID] = claim
		}
	}
	for i, t := range newThoughts {
		if err := validateSupersessionReferences(tx, t.Namespace, t.Claims, batchClaims); err != nil {
			return fmt.Errorf("validate reflected thought %d: %w", i, err)
		}
	}

	for _, id := range deleteIDs {
		if err := deleteThoughtTx(tx, id); err != nil {
			return fmt.Errorf("delete thought %s: %w", id, err)
		}
	}
	for _, t := range newThoughts {
		if err := insertPreparedThoughtTx(tx, t); err != nil {
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
	var peopleStr, topicsStr, actionItemsStr, priorityStr, namespaceStr sql.NullString
	var createdAt, updatedAt string

	err := db.QueryRow(`
		SELECT id, summary, people, topics, type, priority, action_items, source, namespace, created_at, updated_at
		FROM thoughts WHERE id = ?
	`, id).Scan(&t.ID, &t.Summary, &peopleStr, &topicsStr, &t.Type, &priorityStr, &actionItemsStr, &t.Source, &namespaceStr, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("get thought %s: %w", id, err)
	}
	decodeThoughtMetadata(&t, peopleStr, topicsStr, actionItemsStr, priorityStr, namespaceStr, createdAt, updatedAt)
	result := []Thought{t}
	if err := hydrateClaims(db, result); err != nil {
		return nil, err
	}
	return &result[0], nil
}

func decodeThoughtMetadata(t *Thought, peopleStr, topicsStr, actionItemsStr, priorityStr, namespaceStr sql.NullString, createdAt, updatedAt string) {
	if peopleStr.Valid {
		_ = json.Unmarshal([]byte(peopleStr.String), &t.People)
	}
	if topicsStr.Valid {
		_ = json.Unmarshal([]byte(topicsStr.String), &t.Topics)
	}
	if priorityStr.Valid {
		t.Priority = priorityStr.String
	}
	if actionItemsStr.Valid {
		_ = json.Unmarshal([]byte(actionItemsStr.String), &t.ActionItems)
	}
	if namespaceStr.Valid {
		t.Namespace = namespaceStr.String
	}
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	t.syncSummaryContent()
}

func searchByVector(db *sql.DB, embedding []float32, limit int, thoughtType string, timeRange *TimeRange) ([]Thought, error) {
	return searchByVectorWithFilters(db, embedding, limit, SearchFilters{Type: thoughtType, After: zeroTimeFromRange(timeRange, true), Before: zeroTimeFromRange(timeRange, false)})
}

func zeroTimeFromRange(tr *TimeRange, after bool) time.Time {
	if tr == nil {
		return time.Time{}
	}
	if after {
		return tr.Start
	}
	return tr.End
}

func searchByVectorWithFilters(db *sql.DB, embedding []float32, limit int, filters SearchFilters) ([]Thought, error) {
	vec, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}
	fetchLimit := limit
	if fetchLimit <= 0 {
		fetchLimit = 10
	}
	if filters.Type != "" || len(filters.Topics) > 0 || len(filters.People) > 0 || !filters.Before.IsZero() || !filters.After.IsZero() || filters.Namespace != "" {
		fetchLimit *= 5
	}

	query := `
		SELECT v.id, v.distance,
		       t.summary, t.people, t.topics, t.type, t.priority, t.action_items, t.source, t.namespace, t.created_at, t.updated_at
		FROM thought_vectors v
		JOIN thoughts t ON t.id = v.id
		WHERE v.embedding MATCH ?
		AND k = ?
	`
	args := []any{vec, fetchLimit}
	if filters.Type != "" {
		query += " AND t.type = ?"
		args = append(args, filters.Type)
	}
	if filters.Namespace != "" {
		query += " AND t.namespace = ?"
		args = append(args, filters.Namespace)
	}
	if !filters.Before.IsZero() {
		query += " AND t.created_at <= ?"
		args = append(args, formatDBTime(filters.Before))
	}
	if !filters.After.IsZero() {
		query += " AND t.created_at >= ?"
		args = append(args, formatDBTime(filters.After))
	}
	for _, topic := range filters.Topics {
		query += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM json_each(t.topics) WHERE value = ?%d)`, len(args)+1)
		args = append(args, topic)
	}
	for _, person := range filters.People {
		query += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM json_each(t.people) WHERE value = ?%d)`, len(args)+1)
		args = append(args, person)
	}
	query += " ORDER BY v.distance ASC, t.id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search with filters: %w", err)
	}
	defer rows.Close()

	thoughts, err := scanThoughtRows(rows, true)
	if err != nil {
		return nil, err
	}
	if err := hydrateClaims(db, thoughts); err != nil {
		return nil, err
	}
	return thoughts, nil
}

func listRecent(db *sql.DB, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	return listRecentWithNamespace(db, since, limit, thoughtType, "")
}

func listRecentWithNamespace(db *sql.DB, since time.Time, limit int, thoughtType string, namespace string) ([]Thought, error) {
	query := `
		SELECT id, summary, people, topics, type, priority, action_items, source, namespace, created_at, updated_at
		FROM thoughts
		WHERE created_at >= ?
	`
	args := []any{formatDBTime(since)}
	if thoughtType != "" {
		query += " AND type = ?"
		args = append(args, thoughtType)
	}
	if namespace != "" {
		query += " AND namespace = ?"
		args = append(args, namespace)
	}
	query += " ORDER BY created_at DESC, id ASC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	defer rows.Close()
	thoughts, err := scanThoughtRows(rows, false)
	if err != nil {
		return nil, err
	}
	if err := hydrateClaims(db, thoughts); err != nil {
		return nil, err
	}
	return thoughts, nil
}

func getStats(db *sql.DB) (*BrainStats, error) {
	return getStatsByNamespace(db, "")
}

func getStatsByNamespace(db *sql.DB, namespace string) (*BrainStats, error) {
	stats := &BrainStats{}
	where := ""
	args := []any{}
	if namespace != "" {
		where = " WHERE namespace = ?"
		args = append(args, namespace)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM thoughts"+where, args...).Scan(&stats.TotalThoughts); err != nil {
		return nil, fmt.Errorf("count thoughts: %w", err)
	}
	if stats.TotalThoughts == 0 {
		return stats, nil
	}
	weekQuery := "SELECT COUNT(*) FROM thoughts WHERE created_at >= datetime('now', '-7 days')"
	weekArgs := []any{}
	if namespace != "" {
		weekQuery += " AND namespace = ?"
		weekArgs = append(weekArgs, namespace)
	}
	if err := db.QueryRow(weekQuery, weekArgs...).Scan(&stats.ThoughtsThisWeek); err != nil {
		return nil, fmt.Errorf("count this week: %w", err)
	}

	topicQuery := `
		SELECT value, COUNT(*) as cnt
		FROM thoughts, json_each(thoughts.topics)
		WHERE value IS NOT NULL
	`
	topicArgs := []any{}
	if namespace != "" {
		topicQuery += " AND thoughts.namespace = ?"
		topicArgs = append(topicArgs, namespace)
	}
	topicQuery += " GROUP BY value ORDER BY cnt DESC, value ASC LIMIT 10"
	topicRows, err := db.Query(topicQuery, topicArgs...)
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

	sourceQuery := `
		SELECT source, COUNT(*) as cnt
		FROM thoughts
		WHERE source IS NOT NULL AND source != ''
	`
	sourceArgs := []any{}
	if namespace != "" {
		sourceQuery += " AND namespace = ?"
		sourceArgs = append(sourceArgs, namespace)
	}
	sourceQuery += " GROUP BY source ORDER BY cnt DESC, source ASC LIMIT 5"
	sourceRows, err := db.Query(sourceQuery, sourceArgs...)
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

	rangeQuery := "SELECT MIN(created_at), MAX(created_at) FROM thoughts"
	rangeArgs := []any{}
	if namespace != "" {
		rangeQuery += " WHERE namespace = ?"
		rangeArgs = append(rangeArgs, namespace)
	}
	var firstStr, lastStr string
	if err := db.QueryRow(rangeQuery, rangeArgs...).Scan(&firstStr, &lastStr); err != nil {
		return nil, fmt.Errorf("date range: %w", err)
	}
	stats.FirstThought = parseTime(firstStr)
	stats.LastThought = parseTime(lastStr)
	days := stats.LastThought.Sub(stats.FirstThought).Hours() / 24
	if days < 1 {
		days = 1
	}
	stats.AvgPerDay = float64(stats.TotalThoughts) / days
	return stats, nil
}

func scanThoughtRows(rows *sql.Rows, withDistance bool) ([]Thought, error) {
	var thoughts []Thought
	for rows.Next() {
		var t Thought
		var peopleStr, topicsStr, actionItemsStr, priorityStr, namespaceStr sql.NullString
		var createdAt, updatedAt string
		var err error
		if withDistance {
			err = rows.Scan(&t.ID, &t.Distance, &t.Summary, &peopleStr, &topicsStr, &t.Type, &priorityStr, &actionItemsStr, &t.Source, &namespaceStr, &createdAt, &updatedAt)
		} else {
			err = rows.Scan(&t.ID, &t.Summary, &peopleStr, &topicsStr, &t.Type, &priorityStr, &actionItemsStr, &t.Source, &namespaceStr, &createdAt, &updatedAt)
		}
		if err != nil {
			return nil, fmt.Errorf("scan thought: %w", err)
		}
		decodeThoughtMetadata(&t, peopleStr, topicsStr, actionItemsStr, priorityStr, namespaceStr, createdAt, updatedAt)
		thoughts = append(thoughts, t)
	}
	return thoughts, rows.Err()
}

func hydrateClaims(db *sql.DB, thoughts []Thought) error {
	if len(thoughts) == 0 {
		return nil
	}
	placeholders := make([]string, len(thoughts))
	args := make([]any, len(thoughts))
	indexByID := make(map[string]int, len(thoughts))
	for i, thought := range thoughts {
		placeholders[i] = "?"
		args[i] = thought.ID
		indexByID[thought.ID] = i
	}
	query := fmt.Sprintf(`
		SELECT id, thought_id, subject, predicate, object_value, polarity, cardinality, status, supersedes_claim_id, confidence, created_at
		FROM thought_claims
		WHERE thought_id IN (%s)
		ORDER BY created_at ASC, id ASC
	`, strings.Join(placeholders, ","))
	rows, err := db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("query claims: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var claim Claim
		var thoughtID string
		var supersedes, confidence sql.NullString
		var createdAt string
		if err := rows.Scan(&claim.ID, &thoughtID, &claim.Subject, &claim.Predicate, &claim.Object, &claim.Polarity, &claim.Cardinality, &claim.Status, &supersedes, &confidence, &createdAt); err != nil {
			return fmt.Errorf("scan claim: %w", err)
		}
		if supersedes.Valid {
			claim.SupersedesClaimID = supersedes.String
		}
		if confidence.Valid {
			claim.Confidence = confidence.String
		}
		claim.CreatedAt = parseTime(createdAt)
		idx := indexByID[thoughtID]
		thoughts[idx].Claims = append(thoughts[idx].Claims, claim)
	}
	return rows.Err()
}

func queryThoughtsWithFilter(db *sql.DB, filter ExportFilter) ([]Thought, error) {
	query := `
		SELECT id, summary, people, topics, type, priority, action_items, source, namespace, created_at, updated_at
		FROM thoughts
		WHERE 1=1
	`
	args := []any{}
	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, formatDBTime(*filter.Since))
	}
	if filter.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, formatDBTime(*filter.Until))
	}
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Source != "" {
		query += " AND source = ?"
		args = append(args, filter.Source)
	}
	if filter.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, filter.Namespace)
	}
	for _, topic := range filter.Topics {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(topics) WHERE value = ?%d)", len(args)+1)
		args = append(args, topic)
	}
	for _, person := range filter.People {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM json_each(people) WHERE value = ?%d)", len(args)+1)
		args = append(args, person)
	}
	query += " ORDER BY created_at DESC, id ASC"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query thoughts: %w", err)
	}
	defer rows.Close()
	thoughts, err := scanThoughtRows(rows, false)
	if err != nil {
		return nil, err
	}
	if err := hydrateClaims(db, thoughts); err != nil {
		return nil, err
	}
	return thoughts, nil
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

func pruneOldThoughts(db *sql.DB, days int) (int, error) {
	cutoffStr := formatDBTime(time.Now().Add(-time.Duration(days) * 24 * time.Hour))
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
