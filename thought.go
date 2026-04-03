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
	Namespace   string    `json:"namespace,omitempty"`
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
