package picobrain

import (
	"testing"
	"time"
)

func TestParseTimeExpression(t *testing.T) {
	// Use a fixed reference time for consistent testing
	refTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		expression  string
		wantStart   time.Time
		wantEnd     time.Time
		wantErr     bool
		errContains string
	}{
		// Today
		{
			name:       "today",
			expression: "today",
			wantStart:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		// Yesterday
		{
			name:       "yesterday",
			expression: "yesterday",
			wantStart:  time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "this week",
			expression: "this week",
			wantStart:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "last week",
			expression: "last week",
			wantStart:  time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		// This month
		{
			name:       "this month",
			expression: "this month",
			wantStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		// Last month
		{
			name:       "last month",
			expression: "last month",
			wantStart:  time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		// N days ago
		{
			name:       "3 days ago",
			expression: "3 days ago",
			wantStart:  time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "7 days ago",
			expression: "7 days ago",
			wantStart:  time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC),
		},
		// N weeks ago
		{
			name:       "2 weeks ago",
			expression: "2 weeks ago",
			wantStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // Monday of week before last
			wantEnd:    time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // Monday of last week
		},
		// N months ago
		{
			name:       "2 months ago",
			expression: "2 months ago",
			wantStart:  time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		// Specific date
		{
			name:       "specific date YYYY-MM-DD",
			expression: "2024-01-10",
			wantStart:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
		},
		// Shortcuts
		{
			name:       "shortcut --today",
			expression: "--today",
			wantStart:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "shortcut --yesterday",
			expression: "--yesterday",
			wantStart:  time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "shortcut --this-week",
			expression: "--this-week",
			wantStart:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC),
		},
		// Variations with different spacing
		{
			name:       "today with spaces",
			expression: "  today  ",
			wantStart:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "last week lowercase",
			expression: "lastweek",
			wantErr:    true,
		},
		// Error cases
		{
			name:        "invalid expression",
			expression:  "invalid time expression",
			wantErr:     true,
			errContains: "invalid time expression",
		},
		{
			name:        "empty expression",
			expression:  "",
			wantErr:     true,
			errContains: "empty time expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeExpression(tt.expression, refTime)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTimeExpression() error = nil, wantErr = true")
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("ParseTimeExpression() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTimeExpression() unexpected error = %v", err)
				return
			}
			if !got.Start.Equal(tt.wantStart) {
				t.Errorf("ParseTimeExpression() Start = %v, want %v", got.Start, tt.wantStart)
			}
			if !got.End.Equal(tt.wantEnd) {
				t.Errorf("ParseTimeExpression() End = %v, want %v", got.End, tt.wantEnd)
			}
		})
	}
}

func TestExtractTimeFilterFromQuery(t *testing.T) {
	refTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name           string
		query          string
		wantCleanQuery string
		wantHasFilter  bool
		wantStart      time.Time
		wantEnd        time.Time
	}{
		{
			name:           "no time filter",
			query:          "what did I work on",
			wantCleanQuery: "what did I work on",
			wantHasFilter:  false,
		},
		{
			name:           "query with today",
			query:          "what did I work on today",
			wantCleanQuery: "what did I work on",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with yesterday",
			query:          "what did I do yesterday",
			wantCleanQuery: "what did I do",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with last week",
			query:          "show me decisions from last week",
			wantCleanQuery: "show me decisions from",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with --today shortcut",
			query:          "--today auth decisions",
			wantCleanQuery: "auth decisions",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with --yesterday shortcut",
			query:          "--yesterday frontend changes",
			wantCleanQuery: "frontend changes",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with --this-week shortcut",
			query:          "--this-week discussions about api",
			wantCleanQuery: "discussions about api",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with time at beginning",
			query:          "today I worked on",
			wantCleanQuery: "I worked on",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:           "query with time in middle",
			query:          "show me yesterday the decisions",
			wantCleanQuery: "show me the decisions",
			wantHasFilter:  true,
			wantStart:      time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			wantEnd:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTimeFilterFromQuery(tt.query, refTime)

			if got.CleanQuery != tt.wantCleanQuery {
				t.Errorf("ExtractTimeFilterFromQuery() CleanQuery = %q, want %q", got.CleanQuery, tt.wantCleanQuery)
			}
			if got.HasFilter != tt.wantHasFilter {
				t.Errorf("ExtractTimeFilterFromQuery() HasFilter = %v, want %v", got.HasFilter, tt.wantHasFilter)
			}
			if tt.wantHasFilter {
				if !got.Start.Equal(tt.wantStart) {
					t.Errorf("ExtractTimeFilterFromQuery() Start = %v, want %v", got.Start, tt.wantStart)
				}
				if !got.End.Equal(tt.wantEnd) {
					t.Errorf("ExtractTimeFilterFromQuery() End = %v, want %v", got.End, tt.wantEnd)
				}
			}
		})
	}
}

func TestTimeRangeValidation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		timeRange TimeRange
		wantValid bool
	}{
		{
			name: "valid range",
			timeRange: TimeRange{
				Start: now.Add(-24 * time.Hour),
				End:   now,
			},
			wantValid: true,
		},
		{
			name: "start equals end",
			timeRange: TimeRange{
				Start: now,
				End:   now,
			},
			wantValid: false,
		},
		{
			name: "start after end",
			timeRange: TimeRange{
				Start: now,
				End:   now.Add(-24 * time.Hour),
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.timeRange.IsValid()
			if got != tt.wantValid {
				t.Errorf("TimeRange.IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
