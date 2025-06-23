package search

import (
	"testing"
)

func TestProcessFTSQuery(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single word",
			input:    "python",
			expected: "python",
		},
		{
			name:     "implicit AND for multi-word",
			input:    "machine learning",
			expected: "machine AND learning",
		},
		{
			name:     "explicit AND preserved",
			input:    "python AND django",
			expected: "python AND django",
		},
		{
			name:     "explicit OR preserved",
			input:    "python OR ruby",
			expected: "python OR ruby",
		},
		{
			name:     "explicit NOT preserved",
			input:    "python NOT django",
			expected: "python NOT django",
		},
		{
			name:     "quoted phrase preserved",
			input:    `"exact phrase match"`,
			expected: `"exact phrase match"`,
		},
		{
			name:     "wildcard preserved",
			input:    "pyth*",
			expected: "pyth*",
		},
		{
			name:     "lowercase and converted to uppercase",
			input:    "python and django",
			expected: "python AND django", // lowercase "and" is converted to uppercase AND
		},
		{
			name:     "lowercase or converted to uppercase",
			input:    "python or ruby",
			expected: "python OR ruby",
		},
		{
			name:     "lowercase not converted to uppercase",
			input:    "python not django",
			expected: "python NOT django",
		},
		{
			name:     "mixed case operators",
			input:    "python And django Or ruby",
			expected: "python And django Or ruby", // only lowercase full words are converted
		},
		{
			name:     "multiple words implicit AND",
			input:    "python django flask",
			expected: "python AND django AND flask",
		},
		{
			name:     "trimmed spaces",
			input:    "  python  django  ",
			expected: "python AND django",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.processFTSQuery(tt.input)
			if result != tt.expected {
				t.Errorf("processFTSQuery(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
