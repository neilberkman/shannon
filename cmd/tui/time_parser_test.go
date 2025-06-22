package tui

import (
	"testing"
	"time"
)

func TestParseTimeExpression(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		input     string
		checkFunc func(time.Time) bool
		wantZero  bool
	}{
		// Absolute date formats
		{
			name:  "ISO date",
			input: "2024-01-01",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				return result.UTC().Equal(expected)
			},
		},
		{
			name:  "ISO datetime",
			input: "2024-01-01T15:04:05",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 1, 15, 4, 5, 0, time.UTC)
				return result.UTC().Equal(expected)
			},
		},
		{
			name:  "US date format",
			input: "01/15/2024",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
				return result.UTC().Equal(expected)
			},
		},
		{
			name:  "Year-month",
			input: "2024-01",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				return result.UTC().Equal(expected)
			},
		},
		{
			name:  "Year only",
			input: "2024",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				return result.UTC().Equal(expected)
			},
		},

		// Keywords (relative to current time)
		{
			name:  "today",
			input: "today",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				return timeEqual(result, expected)
			},
		},
		{
			name:  "yesterday",
			input: "yesterday",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -1)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour // Allow some tolerance
			},
		},
		{
			name:  "1d",
			input: "1d",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -1)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "week",
			input: "week",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -7)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "1w",
			input: "1w",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -7)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "month",
			input: "month",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, -1, 0)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "1y",
			input: "1y",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(-1, 0, 0)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},

		// Relative expressions
		{
			name:  "30d",
			input: "30d",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -30)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "2w",
			input: "2w",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, 0, -14)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "6m",
			input: "6m",
			checkFunc: func(result time.Time) bool {
				expected := now.AddDate(0, -6, 0)
				diff := result.Sub(expected).Abs()
				return diff < time.Hour
			},
		},
		{
			name:  "24h",
			input: "24h",
			checkFunc: func(result time.Time) bool {
				expected := now.Add(-24 * time.Hour)
				diff := result.Sub(expected).Abs()
				return diff < time.Minute
			},
		},

		// Git-style @YYYY format
		{
			name:  "@2024",
			input: "@2024",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(2024, 1, 1, 0, 0, 0, 0, now.Location())
				return timeEqual(result, expected)
			},
		},

		// Case insensitive
		{
			name:  "TODAY (uppercase)",
			input: "TODAY",
			checkFunc: func(result time.Time) bool {
				expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				return timeEqual(result, expected)
			},
		},

		// Invalid inputs should return zero time
		{
			name:     "empty string",
			input:    "",
			wantZero: true,
		},
		{
			name:     "invalid format",
			input:    "not-a-date",
			wantZero: true,
		},
		{
			name:     "invalid number",
			input:    "abc30d",
			wantZero: true,
		},
		{
			name:     "invalid unit",
			input:    "30x",
			wantZero: true,
		},
		{
			name:     "invalid git format",
			input:    "@abc",
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimeExpression(tt.input)

			if tt.wantZero {
				if !result.IsZero() {
					t.Errorf("parseTimeExpression(%q) = %v, want zero time", tt.input, result)
				}
				return
			}

			if result.IsZero() {
				t.Errorf("parseTimeExpression(%q) returned zero time, want valid time", tt.input)
				return
			}

			if !tt.checkFunc(result) {
				t.Errorf("parseTimeExpression(%q) = %v, failed validation check", tt.input, result)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"zero", "0", 0},
		{"positive number", "42", 42},
		{"large number", "123456", 123456},
		{"invalid - letters", "abc", 0},
		{"invalid - mixed", "12abc", 0},
		{"invalid - special chars", "12@", 0},
		{"invalid - negative", "-5", 0}, // Our parseInt doesn't handle negative
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInt(tt.input)
			if result != tt.expected {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// timeEqual compares two times with some tolerance for location differences
func timeEqual(t1, t2 time.Time) bool {
	// Convert both to UTC for comparison
	return t1.UTC().Equal(t2.UTC())
}

// Benchmark the time parsing function
func BenchmarkParseTimeExpression(b *testing.B) {
	expressions := []string{
		"30d", "2w", "6m", "1y",
		"today", "yesterday", "week",
		"2024-01-01", "2024-01", "2024",
		"@2024", "24h", "invalid",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expr := expressions[i%len(expressions)]
		parseTimeExpression(expr)
	}
}
