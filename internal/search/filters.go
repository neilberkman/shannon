package search

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseQuery extracts inline filters from a user query string and returns a
// populated SearchOptions. Quoted phrases are preserved verbatim so filter-like
// tokens inside quotes are treated as literal search text.
//
// Supported inline filters:
//   - sender:human | sender:assistant (alias: from:)
//   - after:<date>  (alias: date:)
//   - before:<date>
//   - sort:date | sort:relevance
//
// Supported date formats:
//   - ISO 8601: 2026-01-15, 2026-01-15T10:30:00Z, 2026/01/15
//   - Relative durations: 30m, 3h, 24h, 7d, 2w, 1mo
//   - Relative words: today, yesterday, last-week, last-month,
//     3-days-ago, 2-weeks-ago, 5-hours-ago, 2-months-ago
func ParseQuery(raw string) SearchOptions {
	return parseQueryAt(raw, time.Now())
}

func parseQueryAt(raw string, now time.Time) SearchOptions {
	opts := SearchOptions{}
	tokens := splitPreservingQuotes(raw)
	var queryParts []string

	for _, tok := range tokens {
		// Never interpret quoted strings as filters.
		if strings.HasPrefix(tok, `"`) {
			queryParts = append(queryParts, tok)
			continue
		}

		lower := strings.ToLower(tok)
		switch {
		case strings.HasPrefix(lower, "sender:") || strings.HasPrefix(lower, "from:"):
			val := strings.ToLower(tok[strings.IndexByte(tok, ':')+1:])
			if val == "human" || val == "assistant" {
				opts.Sender = val
				continue
			}

		case strings.HasPrefix(lower, "after:") || strings.HasPrefix(lower, "date:"):
			val := tok[strings.IndexByte(tok, ':')+1:]
			if t, err := parseDateAt(val, false, now); err == nil {
				opts.StartDate = &t
				continue
			}

		case strings.HasPrefix(lower, "before:"):
			val := tok[strings.IndexByte(tok, ':')+1:]
			if t, err := parseDateAt(val, true, now); err == nil {
				opts.EndDate = &t
				continue
			}

		case strings.HasPrefix(lower, "sort:"):
			val := strings.ToLower(tok[strings.IndexByte(tok, ':')+1:])
			if val == "date" || val == "relevance" {
				opts.SortBy = val
				continue
			}
		}

		queryParts = append(queryParts, tok)
	}

	opts.Query = strings.Join(queryParts, " ")
	return opts
}

// ParseDate parses a flexible date specification (ISO 8601, duration like 7d/3h,
// or relative phrase like yesterday / 3-days-ago / last-week). When endOfDay is
// true, bare calendar dates (YYYY-MM-DD, today, yesterday) resolve to 23:59:59
// so upper-bound filters include the entire specified day.
func ParseDate(s string, endOfDay bool) (time.Time, error) {
	return parseDateAt(s, endOfDay, time.Now())
}

func parseDateAt(raw string, endOfDay bool, now time.Time) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	lower := strings.ToLower(s)

	// 1. Named relative days/periods.
	switch lower {
	case "today":
		return dayBound(now, endOfDay), nil
	case "yesterday":
		return dayBound(now.AddDate(0, 0, -1), endOfDay), nil
	case "last-week", "last_week":
		return now.AddDate(0, 0, -7), nil
	case "last-month", "last_month":
		return now.AddDate(0, -1, 0), nil
	case "last-year", "last_year":
		return now.AddDate(-1, 0, 0), nil
	}

	// 2. Hyphenated "<N>-<unit>-ago" (e.g. 3-days-ago, 2-weeks-ago, 5-hours-ago).
	if strings.HasSuffix(lower, "-ago") {
		parts := strings.Split(strings.TrimSuffix(lower, "-ago"), "-")
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[0]); err == nil && n >= 0 {
				if t, ok := subtractUnit(now, n, parts[1]); ok {
					return t, nil
				}
			}
		}
	}

	// 3. Compact durations: Go durations (30m, 3h) plus d/w/mo suffixes (7d, 2w, 3mo).
	if d, err := time.ParseDuration(lower); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	if t, ok := parseCompactRelative(lower, now); ok {
		return t, nil
	}

	// 4. Full RFC3339 / datetime formats (exact timestamp, no endOfDay shift).
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	// 5. Bare calendar dates (shift to end-of-day when used as an upper bound).
	for _, layout := range []string{
		"2006-01-02",
		"2006/01/02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return dayBound(t, endOfDay), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized date %q (expected YYYY-MM-DD, RFC3339, 7d, yesterday, or 3-days-ago)", raw)
}

func dayBound(t time.Time, endOfDay bool) time.Time {
	y, m, d := t.Date()
	if endOfDay {
		return time.Date(y, m, d, 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
	}
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func subtractUnit(now time.Time, n int, unit string) (time.Time, bool) {
	switch unit {
	case "minute", "minutes", "min", "mins", "m":
		return now.Add(-time.Duration(n) * time.Minute), true
	case "hour", "hours", "hr", "hrs", "h":
		return now.Add(-time.Duration(n) * time.Hour), true
	case "day", "days", "d":
		return now.AddDate(0, 0, -n), true
	case "week", "weeks", "w":
		return now.AddDate(0, 0, -7*n), true
	case "month", "months", "mo":
		return now.AddDate(0, -n, 0), true
	case "year", "years", "y":
		return now.AddDate(-n, 0, 0), true
	default:
		return time.Time{}, false
	}
}

func parseCompactRelative(s string, now time.Time) (time.Time, bool) {
	for _, suffix := range []string{"mo", "d", "w", "y"} {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSuffix(s, suffix)
			if n, err := strconv.Atoi(numStr); err == nil && n >= 0 {
				return subtractUnit(now, n, suffix)
			}
		}
	}
	return time.Time{}, false
}

// splitPreservingQuotes splits s on whitespace while keeping double-quoted
// substrings together as single tokens.
func splitPreservingQuotes(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			cur.WriteByte(ch)
		case !inQuotes && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'):
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// parseDBTimestamp parses timestamps stored in SQLite across the formats Go's
// sqlite driver and older imports may have written.
func parseDBTimestamp(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
