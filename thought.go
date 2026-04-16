package picobrain

import (
	"encoding/json"
	"strings"
	"time"
)

type Thought struct {
	ID          string    `json:"id,omitempty"`
	Summary     string    `json:"summary"`
	Content     string    `json:"-"`
	Embedding   []float32 `json:"-"`
	People      []string  `json:"people,omitempty"`
	Topics      []string  `json:"topics,omitempty"`
	Type        string    `json:"type,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	ActionItems []string  `json:"action_items,omitempty"`
	Source      string    `json:"source,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Distance    float64   `json:"distance,omitempty"`
	Claims      []Claim   `json:"claims"`
}

type Claim struct {
	ID                string    `json:"id,omitempty"`
	Subject           string    `json:"subject"`
	Predicate         string    `json:"predicate"`
	Object            string    `json:"object"`
	Polarity          string    `json:"polarity"`
	Cardinality       string    `json:"cardinality"`
	Status            string    `json:"status"`
	SupersedesClaimID string    `json:"supersedes_claim_id,omitempty"`
	Confidence        string    `json:"confidence,omitempty"`
	CreatedAt         time.Time `json:"created_at,omitempty"`
}

type BrainStats struct {
	TotalThoughts    int       `json:"total_thoughts"`
	ThoughtsThisWeek int       `json:"thoughts_this_week"`
	TopTopics        []string  `json:"top_topics"`
	TopSources       []string  `json:"top_sources"`
	FirstThought     time.Time `json:"first_thought"`
	LastThought      time.Time `json:"last_thought"`
	AvgPerDay        float64   `json:"avg_per_day"`
}

// SearchFilters contains optional filters for semantic search.
type SearchFilters struct {
	Type      string
	Topics    []string
	People    []string
	Before    time.Time
	After     time.Time
	Namespace string
}

type LintIssue struct {
	Type      string   `json:"type"`
	Namespace string   `json:"namespace"`
	ThoughtID string   `json:"thought_id"`
	ClaimIDs  []string `json:"claim_ids,omitempty"`
	Reason    string   `json:"reason"`
}

type ThoughtSummary struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	Type      string    `json:"type,omitempty"`
	Topics    []string  `json:"topics,omitempty"`
	Namespace string    `json:"namespace"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type ThoughtIndex struct {
	ByTopic         map[string][]ThoughtSummary `json:"by_topic"`
	BySubject       map[string][]ThoughtSummary `json:"by_subject"`
	ByPredicate     map[string][]ThoughtSummary `json:"by_predicate"`
	ByStatus        map[string][]ThoughtSummary `json:"by_status"`
	Recent          []ThoughtSummary            `json:"recent"`
	ConflictBuckets map[string][]string         `json:"conflict_buckets"`
	Stats           *BrainStats                 `json:"stats"`
}

type StoredRecord struct {
	ID       string   `json:"id"`
	ClaimIDs []string `json:"claim_ids"`
}

type ReflectResult struct {
	Stored        []string       `json:"stored"`
	StoredRecords []StoredRecord `json:"stored_records,omitempty"`
	Deleted       []string       `json:"deleted"`
}

type ImportResult struct {
	Line     int      `json:"line"`
	ID       string   `json:"id"`
	ClaimIDs []string `json:"claim_ids"`
}

func (t *Thought) UnmarshalJSON(data []byte) error {
	type alias Thought
	aux := &struct {
		Content string `json:"content,omitempty"`
		*alias
	}{alias: (*alias)(t)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if t.Summary == "" {
		t.Summary = aux.Content
	}
	if t.Content == "" {
		t.Content = t.Summary
	}
	return nil
}

func (t *Thought) syncSummaryContent() {
	if t.Summary == "" {
		t.Summary = t.Content
	}
	if t.Content == "" {
		t.Content = t.Summary
	}
}

func (t *Thought) claimIDs() []string {
	ids := make([]string, 0, len(t.Claims))
	for _, claim := range t.Claims {
		if claim.ID != "" {
			ids = append(ids, claim.ID)
		}
	}
	return ids
}

func (t *Thought) canonicalText() string {
	t.syncSummaryContent()
	parts := []string{strings.TrimSpace(t.Summary)}
	for _, claim := range t.Claims {
		claimText := strings.TrimSpace(strings.Join([]string{
			claim.Subject,
			claim.Predicate,
			claim.Object,
			claim.Polarity,
			claim.Cardinality,
			claim.Status,
		}, " "))
		if claimText != "" {
			parts = append(parts, claimText)
		}
	}
	return strings.Join(parts, "\n")
}
