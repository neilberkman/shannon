package tui

import (
	"testing"
	"time"

	"github.com/neilberkman/shannon/internal/search"
)

func TestBuildSearchOptions(t *testing.T) {
	// Create a mock browse model for testing
	createTestModel := func() browseModel {
		return browseModel{}
	}

	tests := []struct {
		name           string
		query          string
		expectedQuery  string
		expectedSender string
		checkStartDate bool
		checkEndDate   bool
		startDateCheck func(time.Time) bool
		endDateCheck   func(time.Time) bool
	}{
		{
			name:           "simple query without filters",
			query:          "machine learning",
			expectedQuery:  "machine learning",
			expectedSender: "",
		},
		{
			name:           "sender filter - human",
			query:          "error from:human",
			expectedQuery:  "error",
			expectedSender: "human",
		},
		{
			name:           "sender filter - human shorthand",
			query:          "error from:h",
			expectedQuery:  "error",
			expectedSender: "human",
		},
		{
			name:           "sender filter - assistant",
			query:          "debugging sender:assistant",
			expectedQuery:  "debugging",
			expectedSender: "assistant",
		},
		{
			name:           "sender filter - assistant shorthand",
			query:          "debugging from:a",
			expectedQuery:  "debugging",
			expectedSender: "assistant",
		},
		{
			name:           "after filter with relative time",
			query:          "python a:30d",
			expectedQuery:  "python",
			checkStartDate: true,
			startDateCheck: func(t time.Time) bool {
				// Should be approximately 30 days ago
				expected := time.Now().AddDate(0, 0, -30)
				diff := t.Sub(expected).Abs()
				return diff < time.Hour // Allow 1 hour tolerance
			},
		},
		{
			name:           "since filter with keyword",
			query:          "code since:week",
			expectedQuery:  "code",
			checkStartDate: true,
			startDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, 0, -7)
				diff := t.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:          "before filter with relative time",
			query:         "error b:1m",
			expectedQuery: "error",
			checkEndDate:  true,
			endDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, -1, 0)
				diff := t.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:          "until filter with keyword",
			query:         "bug until:yesterday",
			expectedQuery: "bug",
			checkEndDate:  true,
			endDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, 0, -1)
				diff := t.Sub(expected).Abs()
				return diff < 24*time.Hour // More tolerance for yesterday
			},
		},
		{
			name:           "absolute date filters",
			query:          "performance after:2024-01-01 before:2024-12-31",
			expectedQuery:  "performance",
			checkStartDate: true,
			checkEndDate:   true,
			startDateCheck: func(t time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				return t.UTC().Equal(expected)
			},
			endDateCheck: func(t time.Time) bool {
				expected := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
				return t.UTC().Equal(expected)
			},
		},
		{
			name:           "multiple filters",
			query:          "debugging from:a a:2w b:yesterday",
			expectedQuery:  "debugging",
			expectedSender: "assistant",
			checkStartDate: true,
			checkEndDate:   true,
			startDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, 0, -14)
				diff := t.Sub(expected).Abs()
				return diff < time.Hour
			},
			endDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, 0, -1)
				diff := t.Sub(expected).Abs()
				return diff < 24*time.Hour
			},
		},
		{
			name:           "git-style year filter",
			query:          "code a:@2024",
			expectedQuery:  "code",
			checkStartDate: true,
			startDateCheck: func(t time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Now().Location())
				return timeEqual(t, expected)
			},
		},
		{
			name:           "complex query with multiple search terms",
			query:          "python machine learning from:h a:1m",
			expectedQuery:  "python machine learning",
			expectedSender: "human",
			checkStartDate: true,
			startDateCheck: func(t time.Time) bool {
				expected := time.Now().AddDate(0, -1, 0)
				diff := t.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:           "invalid filters are ignored",
			query:          "test from:invalid a:baddate",
			expectedQuery:  "test", // Both invalid filters are silently ignored
			expectedSender: "",     // invalid sender should be ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := createTestModel()
			opts := model.buildSearchOptions(tt.query)

			// Check basic properties
			if opts.Query != tt.expectedQuery {
				t.Errorf("buildSearchOptions().Query = %q, want %q", opts.Query, tt.expectedQuery)
			}

			if opts.Sender != tt.expectedSender {
				t.Errorf("buildSearchOptions().Sender = %q, want %q", opts.Sender, tt.expectedSender)
			}

			// Check default values
			if opts.Limit != 1000 {
				t.Errorf("buildSearchOptions().Limit = %d, want 1000", opts.Limit)
			}

			if opts.SortBy != "relevance" {
				t.Errorf("buildSearchOptions().SortBy = %q, want \"relevance\"", opts.SortBy)
			}

			if opts.SortOrder != "desc" {
				t.Errorf("buildSearchOptions().SortOrder = %q, want \"desc\"", opts.SortOrder)
			}

			// Check start date
			if tt.checkStartDate {
				if opts.StartDate == nil {
					t.Error("buildSearchOptions().StartDate is nil, expected non-nil")
				} else if !tt.startDateCheck(*opts.StartDate) {
					t.Errorf("buildSearchOptions().StartDate = %v, failed validation", *opts.StartDate)
				}
			} else {
				if opts.StartDate != nil {
					t.Errorf("buildSearchOptions().StartDate = %v, want nil", *opts.StartDate)
				}
			}

			// Check end date
			if tt.checkEndDate {
				if opts.EndDate == nil {
					t.Error("buildSearchOptions().EndDate is nil, expected non-nil")
				} else if !tt.endDateCheck(*opts.EndDate) {
					t.Errorf("buildSearchOptions().EndDate = %v, failed validation", *opts.EndDate)
				}
			} else {
				if opts.EndDate != nil {
					t.Errorf("buildSearchOptions().EndDate = %v, want nil", *opts.EndDate)
				}
			}
		})
	}
}

func TestSearchOptionsEdgeCases(t *testing.T) {
	model := browseModel{}

	tests := []struct {
		name  string
		query string
		check func(search.SearchOptions) bool
	}{
		{
			name:  "empty query",
			query: "",
			check: func(opts search.SearchOptions) bool {
				return opts.Query == "" && opts.Sender == "" && opts.StartDate == nil && opts.EndDate == nil
			},
		},
		{
			name:  "only filters, no search terms",
			query: "from:human a:30d",
			check: func(opts search.SearchOptions) bool {
				return opts.Query == "" && opts.Sender == "human" && opts.StartDate != nil
			},
		},
		{
			name:  "multiple spaces",
			query: "test   query   from:human   a:30d",
			check: func(opts search.SearchOptions) bool {
				return opts.Query == "test query" && opts.Sender == "human"
			},
		},
		{
			name:  "case insensitive sender",
			query: "test FROM:HUMAN",
			check: func(opts search.SearchOptions) bool {
				return opts.Query == "test FROM:HUMAN" // Should not parse uppercase
			},
		},
		{
			name:  "malformed filters",
			query: "test from: a: b:",
			check: func(opts search.SearchOptions) bool {
				// Malformed filters (empty values) are silently ignored
				return opts.Query == "test" && opts.Sender == ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := model.buildSearchOptions(tt.query)
			if !tt.check(opts) {
				t.Errorf("buildSearchOptions(%q) failed validation check", tt.query)
			}
		})
	}
}

// Benchmark the search options building
func BenchmarkBuildSearchOptions(b *testing.B) {
	model := browseModel{}
	queries := []string{
		"simple query",
		"python from:human a:30d",
		"debugging from:a after:2024-01-01 before:2024-12-31",
		"machine learning since:1w until:yesterday",
		"error handling code from:h b:1m",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		model.buildSearchOptions(query)
	}
}
