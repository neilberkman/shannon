package rendering

import (
	"os"
	"testing"
)

func TestMakeHyperlink(t *testing.T) {
	tests := []struct {
		name        string
		displayText string
		targetURL   string
		setupEnv    func()
		expected    string
	}{
		{
			name:        "hyperlinks supported",
			displayText: "Click here",
			targetURL:   "https://example.com",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			expected: "\x1b]8;;https://example.com\x1b\\Click here\x1b]8;;\x1b\\",
		},
		{
			name:        "hyperlinks not supported",
			displayText: "Click here",
			targetURL:   "https://example.com",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "dumb")
			},
			expected: "Click here",
		},
		{
			name:        "empty URL returns display text",
			displayText: "No link",
			targetURL:   "",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			expected: "No link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			tt.setupEnv()

			// Clear cache for terminal detection
			// (This would be needed if we cached terminal capabilities)

			result := MakeHyperlink(tt.displayText, tt.targetURL)
			if result != tt.expected {
				t.Errorf("MakeHyperlink() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMakeHyperlinkWithID(t *testing.T) {
	// Setup supported terminal
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name        string
		displayText string
		targetURL   string
		id          string
		expected    string
	}{
		{
			name:        "with ID parameter",
			displayText: "Test Link",
			targetURL:   "https://example.com",
			id:          "test-123",
			expected:    "\x1b]8;id=test-123;https://example.com\x1b\\Test Link\x1b]8;;\x1b\\",
		},
		{
			name:        "without ID parameter",
			displayText: "Test Link",
			targetURL:   "https://example.com",
			id:          "",
			expected:    "\x1b]8;;https://example.com\x1b\\Test Link\x1b]8;;\x1b\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeHyperlinkWithID(tt.displayText, tt.targetURL, tt.id)
			if result != tt.expected {
				t.Errorf("MakeHyperlinkWithID() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAutoLinkText(t *testing.T) {
	// Setup supported terminal
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name     string
		input    string
		contains []string // Substrings that should be in the output
	}{
		{
			name:  "basic HTTP URL",
			input: "Visit https://example.com for more info",
			contains: []string{
				"\x1b]8;;https://example.com\x1b\\https://example.com\x1b]8;;\x1b\\",
				"Visit",
				"for more info",
			},
		},
		{
			name:  "HTTP URL without scheme",
			input: "Check out example.com",
			contains: []string{
				"\x1b]8;;https://example.com\x1b\\example.com\x1b]8;;\x1b\\",
				"Check out",
			},
		},
		{
			name:  "multiple URLs",
			input: "See https://github.com and stackoverflow.com",
			contains: []string{
				"\x1b]8;;https://github.com\x1b\\https://github.com\x1b]8;;\x1b\\",
				"\x1b]8;;https://stackoverflow.com\x1b\\stackoverflow.com\x1b]8;;\x1b\\",
			},
		},
		{
			name:     "no URLs",
			input:    "This is just plain text with no links",
			contains: []string{"This is just plain text with no links"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AutoLinkText(tt.input)

			for _, expectedSubstring := range tt.contains {
				if !containsString(result, expectedSubstring) {
					t.Errorf("AutoLinkText() result %q does not contain expected substring %q", result, expectedSubstring)
				}
			}
		})
	}
}

func TestAutoLinkTextUnsupportedTerminal(t *testing.T) {
	// Setup unsupported terminal
	os.Unsetenv("TERM_PROGRAM")
	os.Unsetenv("KITTY_WINDOW_ID")

	input := "Visit https://example.com for more info"
	result := AutoLinkText(input)

	// Should return original text unchanged
	if result != input {
		t.Errorf("AutoLinkText() in unsupported terminal = %q, want %q", result, input)
	}
}

func TestMakeLinkedInProfileLink(t *testing.T) {
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name       string
		profileURL string
		expected   string
	}{
		{
			name:       "LinkedIn profile URL",
			profileURL: "https://linkedin.com/in/johndoe",
			expected:   "\x1b]8;;https://linkedin.com/in/johndoe\x1b\\@johndoe\x1b]8;;\x1b\\",
		},
		{
			name:       "empty URL",
			profileURL: "",
			expected:   "",
		},
		{
			name:       "malformed URL falls back to default text",
			profileURL: "not-a-url",
			expected:   "\x1b]8;;not-a-url\x1b\\LinkedIn Profile\x1b]8;;\x1b\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeLinkedInProfileLink(tt.profileURL)
			if result != tt.expected {
				t.Errorf("MakeLinkedInProfileLink() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMakeCompanyWebsiteLink(t *testing.T) {
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name        string
		websiteURL  string
		companyName string
		expected    string
	}{
		{
			name:        "with company name",
			websiteURL:  "https://example.com",
			companyName: "Example Corp",
			expected:    "\x1b]8;;https://example.com\x1b\\Example Corp\x1b]8;;\x1b\\",
		},
		{
			name:        "without company name",
			websiteURL:  "https://example.com",
			companyName: "",
			expected:    "\x1b]8;;https://example.com\x1b\\example.com\x1b]8;;\x1b\\",
		},
		{
			name:        "empty URL returns company name",
			websiteURL:  "",
			companyName: "Example Corp",
			expected:    "Example Corp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeCompanyWebsiteLink(tt.websiteURL, tt.companyName)
			if result != tt.expected {
				t.Errorf("MakeCompanyWebsiteLink() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMakeEmailLink(t *testing.T) {
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "valid email",
			email:    "test@example.com",
			expected: "\x1b]8;;mailto:test@example.com\x1b\\test@example.com\x1b]8;;\x1b\\",
		},
		{
			name:     "empty email",
			email:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeEmailLink(tt.email)
			if result != tt.expected {
				t.Errorf("MakeEmailLink() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractURLsFromText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single HTTP URL",
			input:    "Visit https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "multiple URLs",
			input:    "Check https://github.com and example.com",
			expected: []string{"https://github.com", "example.com"},
		},
		{
			name:     "no URLs",
			input:    "Just plain text",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractURLsFromText(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("ExtractURLsFromText() returned %d URLs, want %d", len(result), len(tt.expected))
				return
			}

			for i, expectedURL := range tt.expected {
				if result[i] != expectedURL {
					t.Errorf("ExtractURLsFromText()[%d] = %q, want %q", i, result[i], expectedURL)
				}
			}
		})
	}
}

func TestEnhanceTextWithLinks(t *testing.T) {
	os.Setenv("TERM_PROGRAM", "ghostty")

	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "URLs and text",
			input: "Visit https://example.com for info",
			contains: []string{
				"\x1b]8;;https://example.com\x1b\\https://example.com\x1b]8;;\x1b\\",
				"Visit",
				"for info",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnhanceTextWithLinks(tt.input)

			for _, expectedSubstring := range tt.contains {
				if !containsString(result, expectedSubstring) {
					t.Errorf("EnhanceTextWithLinks() result does not contain expected substring %q", expectedSubstring)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
