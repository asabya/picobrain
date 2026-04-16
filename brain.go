package picobrain

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

type Brain struct {
	db        *sql.DB
	embedder  Embedder
	depParser *DepParser
	config    Config
	cache     *ThoughtCache
}

var localEmbedderFactory = func(modelName, cacheDir string, autoDownload bool) (Embedder, error) {
	return NewLocalEmbedder(modelName, cacheDir, autoDownload)
}

var depParserFactory = NewDepParser

func New(cfg Config) (*Brain, error) {
	sqlite_vec.Auto()
	db, err := openBrainDB(cfg)
	if err != nil {
		return nil, err
	}

	embedder, err := localEmbedderFactory(cfg.EmbedModel, cfg.ModelCacheDir, cfg.AutoDownload)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	return newBrainWithResources(cfg, db, embedder, true)
}

func NewWithEmbedder(cfg Config, emb Embedder) (*Brain, error) {
	sqlite_vec.Auto()
	db, err := openBrainDB(cfg)
	if err != nil {
		return nil, err
	}

	return newBrainWithResources(cfg, db, emb, true)
}

func openBrainDB(cfg Config) (*sql.DB, error) {
	if cfg.DBPath != ":memory:" {
		dir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if cfg.DBPath != ":memory:" {
		db.Exec("PRAGMA journal_mode=WAL")
	}
	db.Exec("PRAGMA foreign_keys = ON")
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return db, nil
}

func newBrainWithResources(cfg Config, db *sql.DB, emb Embedder, closeEmbedderOnError bool) (*Brain, error) {
	brain := &Brain{
		db:       db,
		embedder: emb,
		config:   cfg,
		cache:    NewThoughtCache(cfg.CacheSize),
	}

	parser, err := depParserFactory(cfg.SpacyCacheDir)
	if err != nil {
		if closeEmbedderOnError && emb != nil {
			_ = emb.Close()
		}
		db.Close()
		return nil, fmt.Errorf("initialize spacy dependency parser: %w", err)
	}
	brain.depParser = parser

	return brain, nil
}

func (b *Brain) Close() error {
	if b.depParser != nil {
		b.depParser.Close()
	}
	if b.embedder != nil {
		b.embedder.Close()
	}
	return b.db.Close()
}

func (b *Brain) Store(ctx context.Context, t *Thought) error {
	return b.store(ctx, t, false, false)
}

func (b *Brain) store(ctx context.Context, t *Thought, strict bool, requireIDs bool) error {
	if err := prepareThoughtForStorage(t, b.defaultNamespace(), strict, requireIDs); err != nil {
		return err
	}
	if len(t.Embedding) == 0 {
		emb, err := b.embedder.Embed(ctx, t.canonicalText())
		if err != nil {
			return fmt.Errorf("generate embedding: %w", err)
		}
		t.Embedding = emb
	}
	if err := insertThoughtTx(b.db, t, strict); err != nil {
		return err
	}
	b.cache.Put(*t)
	if b.depParser != nil {
		go func(summary string, thoughtID string) {
			triples, err := b.depParser.Parse(context.Background(), summary)
			if err == nil && len(triples) > 0 {
				b.autoLinkTriples(context.Background(), thoughtID, triples)
			}
		}(t.Summary, t.ID)
	}
	return nil
}

func (b *Brain) Search(ctx context.Context, query string, limit int, thoughtType string, timeRange *TimeRange) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}
	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	return searchByVector(b.db, queryEmb, limit, thoughtType, timeRange)
}

func (b *Brain) SearchWithFilters(ctx context.Context, query string, limit int, filters SearchFilters) ([]Thought, error) {
	if limit <= 0 {
		limit = 10
	}
	queryEmb, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	return searchByVectorWithFilters(b.db, queryEmb, limit, filters)
}

func (b *Brain) ListRecent(ctx context.Context, since time.Time, limit int, thoughtType string) ([]Thought, error) {
	return b.ListRecentWithNamespace(ctx, since, limit, thoughtType, "")
}

func (b *Brain) ListRecentWithNamespace(ctx context.Context, since time.Time, limit int, thoughtType, namespace string) ([]Thought, error) {
	if limit <= 0 {
		limit = 20
	}
	thoughts, err := listRecentWithNamespace(b.db, since, limit, thoughtType, namespace)
	if err != nil {
		return nil, err
	}
	for i := range thoughts {
		b.cache.Put(thoughts[i])
	}
	return thoughts, nil
}

func (b *Brain) Stats(ctx context.Context) (*BrainStats, error) {
	return getStats(b.db)
}

func (b *Brain) StatsByNamespace(ctx context.Context, namespace string) (*BrainStats, error) {
	if namespace == "" {
		namespace = b.defaultNamespace()
	}
	return getStatsByNamespace(b.db, namespace)
}

func (b *Brain) Get(ctx context.Context, id string) (*Thought, error) {
	if thought, found := b.cache.Get(id); found {
		return &thought, nil
	}
	thought, err := getThought(b.db, id)
	if err != nil {
		return nil, err
	}
	b.cache.Put(*thought)
	return thought, nil
}

func (b *Brain) GetRecent(limit int) []Thought {
	if limit <= 0 {
		limit = 20
	}
	return b.cache.GetRecent(limit)
}

func (b *Brain) Delete(ctx context.Context, id string) error {
	if err := deleteThought(b.db, id); err != nil {
		return err
	}
	b.cache.Remove(id)
	return nil
}

func (b *Brain) Reflect(ctx context.Context, deleteIDs []string, newThoughts []*Thought) (*ReflectResult, error) {
	for _, t := range newThoughts {
		if err := prepareThoughtForStorage(t, b.defaultNamespace(), false, false); err != nil {
			return nil, err
		}
		if len(t.Embedding) == 0 {
			emb, err := b.embedder.Embed(ctx, t.canonicalText())
			if err != nil {
				return nil, fmt.Errorf("generate embedding: %w", err)
			}
			t.Embedding = emb
		}
	}
	if err := reflectTx(b.db, deleteIDs, newThoughts); err != nil {
		return nil, err
	}
	for _, id := range deleteIDs {
		b.cache.Remove(id)
	}
	storedIDs := make([]string, len(newThoughts))
	storedRecords := make([]StoredRecord, len(newThoughts))
	for i, t := range newThoughts {
		b.cache.Put(*t)
		storedIDs[i] = t.ID
		storedRecords[i] = StoredRecord{ID: t.ID, ClaimIDs: t.claimIDs()}
	}
	return &ReflectResult{Stored: storedIDs, StoredRecords: storedRecords, Deleted: deleteIDs}, nil
}

func (b *Brain) BulkImport(ctx context.Context, r io.Reader) (int, error) {
	scanner := bufio.NewScanner(r)
	thoughts := make([]*Thought, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var t Thought
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return len(thoughts), fmt.Errorf("parse line %d: %w", len(thoughts)+1, err)
		}
		if err := prepareThoughtForStorage(&t, b.defaultNamespace(), false, false); err != nil {
			return len(thoughts), err
		}
		emb, err := b.embedder.Embed(ctx, t.canonicalText())
		if err != nil {
			return len(thoughts), fmt.Errorf("embed thought %d: %w", len(thoughts)+1, err)
		}
		t.Embedding = emb
		thoughts = append(thoughts, &t)
	}
	if err := scanner.Err(); err != nil {
		return len(thoughts), fmt.Errorf("read input: %w", err)
	}
	if len(thoughts) == 0 {
		return 0, nil
	}
	tx, err := b.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	batchClaims := map[string]Claim{}
	for _, thought := range thoughts {
		for _, claim := range thought.Claims {
			batchClaims[claim.ID] = claim
		}
	}
	for _, thought := range thoughts {
		if err := validateSupersessionReferences(tx, thought.Namespace, thought.Claims, batchClaims); err != nil {
			return 0, err
		}
		if err := insertPreparedThoughtTx(tx, thought); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	for _, thought := range thoughts {
		b.cache.Put(*thought)
	}
	return len(thoughts), nil
}

func (b *Brain) BulkImportDetailed(ctx context.Context, r io.Reader, namespace string) ([]ImportResult, error) {
	scanner := bufio.NewScanner(r)
	thoughts := make([]*Thought, 0)
	results := make([]ImportResult, 0)
	batchClaims := map[string]Claim{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var t Thought
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if t.Namespace == "" {
			t.Namespace = namespace
		}
		if err := prepareThoughtForStorage(&t, b.defaultNamespace(), true, true); err != nil {
			return nil, annotateLineError(lineNo, err)
		}
		for _, claim := range t.Claims {
			if _, exists := batchClaims[claim.ID]; exists {
				return nil, annotateLineError(lineNo, validationError("claims.id", "must be globally unique"))
			}
			batchClaims[claim.ID] = claim
		}
		thoughts = append(thoughts, &t)
		results = append(results, ImportResult{Line: lineNo, ID: t.ID, ClaimIDs: t.claimIDs()})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if len(thoughts) == 0 {
		return results, nil
	}
	for _, thought := range thoughts {
		if len(thought.Embedding) == 0 {
			emb, err := b.embedder.Embed(ctx, thought.canonicalText())
			if err != nil {
				return nil, fmt.Errorf("embed import line for thought %s: %w", thought.ID, err)
			}
			thought.Embedding = emb
		}
	}
	tx, err := b.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	for i, thought := range thoughts {
		if err := ensureThoughtAndClaimIDsAvailable(tx, thought); err != nil {
			return nil, annotateLineError(results[i].Line, err)
		}
		if err := validateSupersessionReferences(tx, thought.Namespace, thought.Claims, batchClaims); err != nil {
			return nil, annotateLineError(results[i].Line, err)
		}
	}
	for _, thought := range thoughts {
		if err := insertPreparedThoughtTx(tx, thought); err != nil {
			return nil, fmt.Errorf("insert imported thought %s: %w", thought.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	for _, thought := range thoughts {
		b.cache.Put(*thought)
	}
	return results, nil
}

func ensureThoughtAndClaimIDsAvailable(exec dbQueryExecer, thought *Thought) error {
	var count int
	if err := exec.QueryRow(`SELECT COUNT(*) FROM thoughts WHERE id = ?`, thought.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return validationError("id", "must be unique within the target namespace")
	}
	for _, claim := range thought.Claims {
		if err := exec.QueryRow(`SELECT COUNT(*) FROM thought_claims WHERE id = ?`, claim.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return validationError("claims.id", "must be globally unique")
		}
	}
	return nil
}

func annotateLineError(line int, err error) error {
	field, msg, ok := parseValidationError(err)
	if !ok {
		return err
	}
	return fmt.Errorf("validation_failed:%s:%s:line=%d", field, msg, line)
}

func annotateRecordError(recordIndex int, err error) error {
	field, msg, ok := parseValidationError(err)
	if !ok {
		return err
	}
	return fmt.Errorf("validation_failed:%s:%s:record_index=%d", field, msg, recordIndex)
}

func (b *Brain) Prune(ctx context.Context, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	deleted, err := pruneOldThoughts(b.db, days)
	if err != nil {
		return 0, fmt.Errorf("prune thoughts: %w", err)
	}
	if deleted > 0 {
		b.cache.Clear()
	}
	return deleted, nil
}

func (b *Brain) Lint(ctx context.Context, namespace string) ([]LintIssue, error) {
	if namespace == "" {
		namespace = b.defaultNamespace()
	}
	thoughts, err := b.ListRecentWithNamespace(ctx, time.Time{}, 10000, "", namespace)
	if err != nil {
		return nil, err
	}
	issues := make([]LintIssue, 0)
	tupleSeen := map[string]Claim{}
	topicSubjectOwners := map[string][]string{}
	activeClaimBySuperseded := map[string]Claim{}
	for _, thought := range thoughts {
		for _, topic := range thought.Topics {
			topicSubjectOwners["topic:"+topic] = append(topicSubjectOwners["topic:"+topic], thought.ID)
		}
		for _, claim := range thought.Claims {
			topicSubjectOwners["subject:"+claim.Subject] = append(topicSubjectOwners["subject:"+claim.Subject], thought.ID)
			if claim.Status == "active" && claim.SupersedesClaimID != "" {
				activeClaimBySuperseded[claim.SupersedesClaimID] = claim
			}
			tupleKey := strings.Join([]string{claim.Subject, claim.Predicate, claim.Object, claim.Polarity, claim.Cardinality, claim.Status}, "|")
			if seen, ok := tupleSeen[tupleKey]; ok && claim.Status == "active" && seen.Status == "active" {
				issues = append(issues, LintIssue{Type: "duplicate", Namespace: namespace, ThoughtID: thought.ID, ClaimIDs: []string{seen.ID, claim.ID}, Reason: "duplicate active claim tuple found"})
			} else {
				tupleSeen[tupleKey] = claim
			}
		}
	}
	for _, thought := range thoughts {
		for _, claim := range thought.Claims {
			if claim.Status == "superseded" {
				if replacement, ok := activeClaimBySuperseded[claim.ID]; ok {
					issues = append(issues, LintIssue{Type: "superseded", Namespace: namespace, ThoughtID: thought.ID, ClaimIDs: []string{claim.ID, replacement.ID}, Reason: "claim is superseded by an active replacement claim"})
				}
			}
		}
		noEdges, err := b.isOrphan(thought.ID)
		if err != nil {
			return nil, err
		}
		shared := false
		for _, topic := range thought.Topics {
			if len(uniqueStrings(topicSubjectOwners["topic:"+topic])) > 1 {
				shared = true
				break
			}
		}
		if !shared {
			for _, claim := range thought.Claims {
				if len(uniqueStrings(topicSubjectOwners["subject:"+claim.Subject])) > 1 {
					shared = true
					break
				}
			}
		}
		if noEdges && !shared {
			issues = append(issues, LintIssue{Type: "orphan", Namespace: namespace, ThoughtID: thought.ID, Reason: "record has no graph edges and shares no topic or claim subject with another record"})
		}
	}
	activeBySP := map[string][]Claim{}
	for _, thought := range thoughts {
		for _, claim := range thought.Claims {
			if claim.Status == "active" {
				activeBySP[claim.Subject+"|"+claim.Predicate] = append(activeBySP[claim.Subject+"|"+claim.Predicate], claim)
			}
		}
	}
	keys := make([]string, 0, len(activeBySP))
	for key := range activeBySP {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		claims := activeBySP[key]
		for i := 0; i < len(claims); i++ {
			for j := i + 1; j < len(claims); j++ {
				a, c := claims[i], claims[j]
				if a.Object == c.Object && a.Polarity != c.Polarity {
					issues = append(issues, LintIssue{Type: "contradiction", Namespace: namespace, ClaimIDs: []string{a.ID, c.ID}, Reason: "same subject/predicate/object but opposite polarity"})
					continue
				}
				if a.Object != c.Object && a.Cardinality == "one" && c.Cardinality == "one" {
					issues = append(issues, LintIssue{Type: "contradiction", Namespace: namespace, ClaimIDs: []string{a.ID, c.ID}, Reason: "same subject/predicate but conflicting cardinality-one objects"})
				}
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		left := issues[i]
		right := issues[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.ThoughtID != right.ThoughtID {
			return left.ThoughtID < right.ThoughtID
		}
		leftClaims := strings.Join(left.ClaimIDs, ",")
		rightClaims := strings.Join(right.ClaimIDs, ",")
		if leftClaims != rightClaims {
			return leftClaims < rightClaims
		}
		return left.Reason < right.Reason
	})
	return issues, nil
}

func (b *Brain) isOrphan(thoughtID string) (bool, error) {
	var count int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM thought_edges WHERE source_id = ? OR target_id = ?`, thoughtID, thoughtID).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (b *Brain) Index(ctx context.Context, namespace string) (*ThoughtIndex, error) {
	if namespace == "" {
		namespace = b.defaultNamespace()
	}
	thoughts, err := b.ListRecentWithNamespace(ctx, time.Time{}, 10000, "", namespace)
	if err != nil {
		return nil, err
	}
	stats, err := b.StatsByNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	issues, err := b.Lint(ctx, namespace)
	if err != nil {
		return nil, err
	}
	index := &ThoughtIndex{
		ByTopic:         map[string][]ThoughtSummary{},
		BySubject:       map[string][]ThoughtSummary{},
		ByPredicate:     map[string][]ThoughtSummary{},
		ByStatus:        map[string][]ThoughtSummary{},
		Recent:          make([]ThoughtSummary, 0, len(thoughts)),
		ConflictBuckets: map[string][]string{},
		Stats:           stats,
	}
	for _, thought := range thoughts {
		summary := ThoughtSummary{ID: thought.ID, Summary: thought.Summary, Type: thought.Type, Topics: thought.Topics, Namespace: thought.Namespace, CreatedAt: thought.CreatedAt}
		index.Recent = append(index.Recent, summary)
		for _, topic := range thought.Topics {
			index.ByTopic[topic] = append(index.ByTopic[topic], summary)
		}
		for _, claim := range thought.Claims {
			index.BySubject[claim.Subject] = append(index.BySubject[claim.Subject], summary)
			index.ByPredicate[claim.Predicate] = append(index.ByPredicate[claim.Predicate], summary)
			index.ByStatus[claim.Status] = append(index.ByStatus[claim.Status], summary)
		}
	}
	for _, issue := range issues {
		if issue.Type == "contradiction" || issue.Type == "duplicate" || issue.Type == "superseded" {
			index.ConflictBuckets[issue.Type] = append(index.ConflictBuckets[issue.Type], issue.Reason)
		}
	}
	for key := range index.ConflictBuckets {
		sort.Strings(index.ConflictBuckets[key])
	}
	return index, nil
}

func (b *Brain) defaultNamespace() string {
	if b.config.DefaultNamespace != "" {
		return b.config.DefaultNamespace
	}
	return "default"
}

// --- Graph methods ---

func (b *Brain) CreateEdge(ctx context.Context, e *Edge) error {
	if e.SourceID == "" || e.TargetID == "" || e.RelationType == "" {
		return fmt.Errorf("source_id, target_id, and relation_type are required")
	}
	if _, err := b.Get(ctx, e.SourceID); err != nil {
		return fmt.Errorf("source thought %s not found: %w", e.SourceID, err)
	}
	if _, err := b.Get(ctx, e.TargetID); err != nil {
		return fmt.Errorf("target thought %s not found: %w", e.TargetID, err)
	}
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := insertEdgeTx(tx, e); err != nil {
		return err
	}
	return tx.Commit()
}

func (b *Brain) DeleteEdge(ctx context.Context, id string) error {
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := deleteEdgeTx(tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (b *Brain) GetEdge(ctx context.Context, id string) (*Edge, error) { return getEdge(b.db, id) }
func (b *Brain) GetNeighbors(ctx context.Context, thoughtID, direction, relationType string, limit int) ([]Edge, error) {
	return getNeighbors(b.db, thoughtID, direction, relationType, limit)
}
func (b *Brain) FindPath(ctx context.Context, sourceID, targetID string, maxDepth int) ([]PathStep, error) {
	return findPath(b.db, sourceID, targetID, maxDepth)
}
func (b *Brain) GraphStats(ctx context.Context) (*GraphStats, error) { return getGraphStats(b.db) }

func (b *Brain) ExtractTriples(ctx context.Context, text string) ([]Triple, error) {
	if b.depParser == nil {
		return nil, fmt.Errorf("dependency parser not available")
	}
	return b.depParser.Parse(ctx, text)
}

func (b *Brain) autoLinkTriples(ctx context.Context, sourceThoughtID string, triples []Triple) {
	threshold := b.config.AutoGraphThreshold
	if threshold <= 0 {
		threshold = 0.7
	}
	for _, triple := range triples {
		targetID := b.resolveEntity(ctx, triple.Tail, threshold)
		if targetID == "" {
			continue
		}
		edge := &Edge{SourceID: sourceThoughtID, TargetID: targetID, RelationType: triple.Relation, AutoExtracted: true}
		if err := b.CreateEdge(ctx, edge); err != nil {
			continue
		}
	}
}

func (b *Brain) resolveEntity(ctx context.Context, entityText string, threshold float64) string {
	if entityText == "" {
		return ""
	}
	results, err := b.Search(ctx, entityText, 3, "", nil)
	if err != nil || len(results) == 0 {
		return ""
	}
	best := results[0]
	if best.Distance < (1.0 - threshold) {
		return best.ID
	}
	return ""
}
