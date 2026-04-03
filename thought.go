package picobrain

import "time"

type Thought struct {
	ID          string    `json:"id,omitempty"`
	Content     string    `json:"content"`
	Embedding   []float32 `json:"-"`
	People      []string  `json:"people,omitempty"`
	Topics      []string  `json:"topics,omitempty"`
	Type        string    `json:"type,omitempty"`
	ActionItems []string  `json:"action_items,omitempty"`
	Source      string    `json:"source,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	Distance    float64   `json:"distance,omitempty"`
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
	Type   string    // Filter by thought type
	Topics []string  // Filter by topics (must have ALL specified topics)
	People []string  // Filter by people (must have ALL specified people)
	Before time.Time // Filter thoughts created before this time
	After  time.Time // Filter thoughts created after this time
}
