package picobrain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeRange represents a time range with start and end times.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// IsValid checks if the time range is valid (start before end).
func (tr TimeRange) IsValid() bool {
	return tr.Start.Before(tr.End)
}

// TimeFilterResult contains the result of extracting a time filter from a query.
type TimeFilterResult struct {
	CleanQuery string
	HasFilter  bool
	Start      time.Time
	End        time.Time
}

// ParseTimeExpression parses a natural language time expression and returns a TimeRange.
// Supported expressions:
//   - today, yesterday
//   - this week, last week
//   - this month, last month
//   - N days ago, N weeks ago, N months ago
//   - YYYY-MM-DD (specific date)
//   - Shortcuts: --today, --yesterday, --this-week
func ParseTimeExpression(expression string, refTime time.Time) (TimeRange, error) {
	expression = strings.TrimSpace(strings.ToLower(expression))

	if expression == "" {
		return TimeRange{}, fmt.Errorf("empty time expression")
	}

	if strings.HasPrefix(expression, "--") {
		shortcut := strings.TrimPrefix(expression, "--")
		switch shortcut {
		case "today":
			return getTodayRange(refTime), nil
		case "yesterday":
			return getYesterdayRange(refTime), nil
		case "this-week":
			return getThisWeekRange(refTime), nil
		default:
			return TimeRange{}, fmt.Errorf("unknown shortcut: %s", shortcut)
		}
	}

	switch expression {
	case "today":
		return getTodayRange(refTime), nil
	case "yesterday":
		return getYesterdayRange(refTime), nil
	case "this week":
		return getThisWeekRange(refTime), nil
	case "last week":
		return getLastWeekRange(refTime), nil
	case "this month":
		return getThisMonthRange(refTime), nil
	case "last month":
		return getLastMonthRange(refTime), nil
	}

	if tr, ok := parseRelativeTime(expression, refTime); ok {
		return tr, nil
	}

	if tr, ok := parseSpecificDate(expression); ok {
		return tr, nil
	}

	return TimeRange{}, fmt.Errorf("invalid time expression: %s", expression)
}

// ExtractTimeFilterFromQuery extracts a time filter from a natural language query.
// It returns the cleaned query (with time expression removed) and the time range if found.
func ExtractTimeFilterFromQuery(query string, refTime time.Time) TimeFilterResult {
	query = strings.TrimSpace(query)
	lowerQuery := strings.ToLower(query)

	patterns := []struct {
		pattern string
		handler func(time.Time) TimeRange
	}{
		{"--today ", func(t time.Time) TimeRange { return getTodayRange(t) }},
		{"--yesterday ", func(t time.Time) TimeRange { return getYesterdayRange(t) }},
		{"--this-week ", func(t time.Time) TimeRange { return getThisWeekRange(t) }},
		{"today ", func(t time.Time) TimeRange { return getTodayRange(t) }},
		{"today", func(t time.Time) TimeRange { return getTodayRange(t) }},
		{" yesterday", func(t time.Time) TimeRange { return getYesterdayRange(t) }},
		{" last week", func(t time.Time) TimeRange { return getLastWeekRange(t) }},
		{" this week", func(t time.Time) TimeRange { return getThisWeekRange(t) }},
		{" last month", func(t time.Time) TimeRange { return getLastMonthRange(t) }},
		{" this month", func(t time.Time) TimeRange { return getThisMonthRange(t) }},
	}

	relativePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\d+)\s+days?\s+ago`),
		regexp.MustCompile(`(?i)(\d+)\s+weeks?\s+ago`),
		regexp.MustCompile(`(?i)(\d+)\s+months?\s+ago`),
	}

	for _, p := range patterns[:3] {
		if strings.HasPrefix(lowerQuery, p.pattern) {
			cleanQuery := strings.TrimSpace(query[len(p.pattern):])
			tr := p.handler(refTime)
			return TimeFilterResult{
				CleanQuery: cleanQuery,
				HasFilter:  true,
				Start:      tr.Start,
				End:        tr.End,
			}
		}
	}

	for _, p := range patterns[3:] {
		if idx := strings.Index(lowerQuery, p.pattern); idx != -1 {
			before := strings.TrimSpace(query[:idx])
			after := strings.TrimSpace(query[idx+len(p.pattern):])
			cleanQuery := strings.TrimSpace(before + " " + after)
			tr := p.handler(refTime)
			return TimeFilterResult{
				CleanQuery: cleanQuery,
				HasFilter:  true,
				Start:      tr.Start,
				End:        tr.End,
			}
		}
	}

	for _, re := range relativePatterns {
		if matches := re.FindStringSubmatchIndex(lowerQuery); matches != nil {
			matchedText := query[matches[0]:matches[1]]
			tr, _ := parseRelativeTime(strings.ToLower(matchedText), refTime)
			before := strings.TrimSpace(query[:matches[0]])
			after := strings.TrimSpace(query[matches[1]:])
			cleanQuery := strings.TrimSpace(before + " " + after)
			return TimeFilterResult{
				CleanQuery: cleanQuery,
				HasFilter:  true,
				Start:      tr.Start,
				End:        tr.End,
			}
		}
	}

	return TimeFilterResult{
		CleanQuery: query,
		HasFilter:  false,
	}
}

func getTodayRange(refTime time.Time) TimeRange {
	start := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 0, 0, 0, 0, refTime.Location())
	end := start.Add(24 * time.Hour)
	return TimeRange{Start: start, End: end}
}

func getYesterdayRange(refTime time.Time) TimeRange {
	today := getTodayRange(refTime)
	return TimeRange{
		Start: today.Start.Add(-24 * time.Hour),
		End:   today.Start,
	}
}

func getThisWeekRange(refTime time.Time) TimeRange {
	weekday := int(refTime.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysFromMonday := weekday - 1
	start := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 0, 0, 0, 0, refTime.Location())
	start = start.Add(-time.Duration(daysFromMonday) * 24 * time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	return TimeRange{Start: start, End: end}
}

func getLastWeekRange(refTime time.Time) TimeRange {
	thisWeek := getThisWeekRange(refTime)
	return TimeRange{
		Start: thisWeek.Start.Add(-7 * 24 * time.Hour),
		End:   thisWeek.Start,
	}
}

func getThisMonthRange(refTime time.Time) TimeRange {
	start := time.Date(refTime.Year(), refTime.Month(), 1, 0, 0, 0, 0, refTime.Location())
	end := start.AddDate(0, 1, 0)
	return TimeRange{Start: start, End: end}
}

func getLastMonthRange(refTime time.Time) TimeRange {
	thisMonth := getThisMonthRange(refTime)
	return TimeRange{
		Start: thisMonth.Start.AddDate(0, -1, 0),
		End:   thisMonth.Start,
	}
}

func parseRelativeTime(expression string, refTime time.Time) (TimeRange, bool) {
	reDays := regexp.MustCompile(`^(\d+)\s+days?\s+ago$`)
	if matches := reDays.FindStringSubmatch(expression); matches != nil {
		n, _ := strconv.Atoi(matches[1])
		end := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 0, 0, 0, 0, refTime.Location())
		start := end.Add(-time.Duration(n) * 24 * time.Hour)
		return TimeRange{Start: start, End: start.Add(24 * time.Hour)}, true
	}

	reWeeks := regexp.MustCompile(`^(\d+)\s+weeks?\s+ago$`)
	if matches := reWeeks.FindStringSubmatch(expression); matches != nil {
		n, _ := strconv.Atoi(matches[1])
		thisWeek := getThisWeekRange(refTime)
		start := thisWeek.Start.AddDate(0, 0, -n*7)
		end := start.Add(7 * 24 * time.Hour)
		return TimeRange{Start: start, End: end}, true
	}

	reMonths := regexp.MustCompile(`^(\d+)\s+months?\s+ago$`)
	if matches := reMonths.FindStringSubmatch(expression); matches != nil {
		n, _ := strconv.Atoi(matches[1])
		thisMonth := getThisMonthRange(refTime)
		start := thisMonth.Start.AddDate(0, -n, 0)
		end := thisMonth.Start.AddDate(0, -n+1, 0)
		return TimeRange{Start: start, End: end}, true
	}

	return TimeRange{}, false
}

func parseSpecificDate(expression string) (TimeRange, bool) {
	if t, err := time.Parse("2006-01-02", expression); err == nil {
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		end := start.Add(24 * time.Hour)
		return TimeRange{Start: start, End: end}, true
	}

	return TimeRange{}, false
}
