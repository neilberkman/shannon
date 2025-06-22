package rendering

import (
	"testing"
)

func TestDetectTerminalCapabilities(t *testing.T) {
	tests := []struct {
		name                  string
		termProgram           string
		term                  string
		kittyWindowID         string
		expectedTerminalType  string
		expectedHyperlinks    bool
		expectedGraphics      bool
		expectedAdvancedInput bool
	}{
		{
			name:                  "Ghostty terminal",
			termProgram:           "ghostty",
			expectedTerminalType:  "ghostty",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name:                  "Kitty terminal",
			termProgram:           "kitty",
			expectedTerminalType:  "kitty",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name:                  "Kitty via KITTY_WINDOW_ID",
			kittyWindowID:         "1",
			expectedTerminalType:  "",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name:                  "WezTerm terminal",
			termProgram:           "wezterm",
			expectedTerminalType:  "wezterm",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name:                  "iTerm2 terminal",
			termProgram:           "iTerm.app",
			expectedTerminalType:  "iTerm.app",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: false,
		},
		{
			name:                  "VS Code terminal",
			termProgram:           "vscode",
			expectedTerminalType:  "vscode",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name:                  "xterm-based terminal",
			term:                  "xterm-256color",
			expectedTerminalType:  "xterm-256color",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name:                  "basic terminal",
			term:                  "xterm",
			expectedTerminalType:  "xterm",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name:                  "unknown terminal",
			term:                  "unknown",
			expectedTerminalType:  "unknown",
			expectedHyperlinks:    false,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name:                  "no terminal info",
			expectedTerminalType:  "",
			expectedHyperlinks:    false,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv automatically restores environment after test
			// Always set all environment variables to ensure clean state
			if tt.termProgram != "" {
				t.Setenv("TERM_PROGRAM", tt.termProgram)
			} else {
				t.Setenv("TERM_PROGRAM", "")
			}
			if tt.term != "" {
				t.Setenv("TERM", tt.term)
			} else {
				t.Setenv("TERM", "")
			}
			if tt.kittyWindowID != "" {
				t.Setenv("KITTY_WINDOW_ID", tt.kittyWindowID)
			} else {
				t.Setenv("KITTY_WINDOW_ID", "")
			}

			caps := DetectTerminalCapabilities()

			if caps.TerminalType != tt.expectedTerminalType {
				t.Errorf("DetectTerminalCapabilities().TerminalType = %q, want %q", caps.TerminalType, tt.expectedTerminalType)
			}

			if caps.SupportsHyperlinks != tt.expectedHyperlinks {
				t.Errorf("DetectTerminalCapabilities().SupportsHyperlinks = %t, want %t", caps.SupportsHyperlinks, tt.expectedHyperlinks)
			}

			if caps.SupportsGraphics != tt.expectedGraphics {
				t.Errorf("DetectTerminalCapabilities().SupportsGraphics = %t, want %t", caps.SupportsGraphics, tt.expectedGraphics)
			}

			if caps.SupportsAdvancedInput != tt.expectedAdvancedInput {
				t.Errorf("DetectTerminalCapabilities().SupportsAdvancedInput = %t, want %t", caps.SupportsAdvancedInput, tt.expectedAdvancedInput)
			}
		})
	}
}

func TestIsHyperlinksSupported(t *testing.T) {
	tests := []struct {
		name        string
		termProgram string
		term        string
		expected    bool
	}{
		{
			name:        "Ghostty supports hyperlinks",
			termProgram: "ghostty",
			expected:    true,
		},
		{
			name:     "Unknown terminal doesn't support hyperlinks",
			term:     "dumb",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.termProgram != "" {
				t.Setenv("TERM_PROGRAM", tt.termProgram)
			} else {
				t.Setenv("TERM_PROGRAM", "")
			}
			if tt.term != "" {
				t.Setenv("TERM", tt.term)
			} else {
				t.Setenv("TERM", "")
			}
			t.Setenv("KITTY_WINDOW_ID", "")

			result := IsHyperlinksSupported()
			if result != tt.expected {
				t.Errorf("IsHyperlinksSupported() = %t, want %t", result, tt.expected)
			}
		})
	}
}

func TestIsGraphicsSupported(t *testing.T) {
	tests := []struct {
		name        string
		termProgram string
		expected    bool
	}{
		{
			name:        "Ghostty supports graphics",
			termProgram: "ghostty",
			expected:    true,
		},
		{
			name:        "VS Code doesn't support graphics",
			termProgram: "vscode",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.termProgram != "" {
				t.Setenv("TERM_PROGRAM", tt.termProgram)
			} else {
				t.Setenv("TERM_PROGRAM", "")
			}
			t.Setenv("TERM", "")
			t.Setenv("KITTY_WINDOW_ID", "")

			result := IsGraphicsSupported()
			if result != tt.expected {
				t.Errorf("IsGraphicsSupported() = %t, want %t", result, tt.expected)
			}
		})
	}
}

func TestGetTerminalInfo(t *testing.T) {
	tests := []struct {
		name        string
		termProgram string
		term        string
		contains    []string
	}{
		{
			name:        "Ghostty terminal info",
			termProgram: "ghostty",
			contains: []string{
				"Terminal: ghostty",
				"supports:",
				"hyperlinks",
				"graphics",
				"advanced-input",
			},
		},
		{
			name:        "VS Code terminal info",
			termProgram: "vscode",
			contains: []string{
				"Terminal: vscode",
				"supports:",
				"hyperlinks",
			},
		},
		{
			name: "Basic terminal info",
			term: "dumb",
			contains: []string{
				"Terminal: dumb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.termProgram != "" {
				t.Setenv("TERM_PROGRAM", tt.termProgram)
			} else {
				t.Setenv("TERM_PROGRAM", "")
			}
			if tt.term != "" {
				t.Setenv("TERM", tt.term)
			} else {
				t.Setenv("TERM", "")
			}
			t.Setenv("KITTY_WINDOW_ID", "")

			result := GetTerminalInfo()

			for _, expectedSubstring := range tt.contains {
				if !containsStringSimple(result, expectedSubstring) {
					t.Errorf("GetTerminalInfo() result %q does not contain expected substring %q", result, expectedSubstring)
				}
			}
		})
	}
}

// Helper function for simple substring checking
func containsStringSimple(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test that the terminal type precedence works correctly
func TestTerminalTypePrecedence(t *testing.T) {
	t.Run("TERM_PROGRAM takes precedence over TERM", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "ghostty")
		t.Setenv("TERM", "xterm")
		t.Setenv("KITTY_WINDOW_ID", "")

		caps := DetectTerminalCapabilities()

		if caps.TerminalType != "ghostty" {
			t.Errorf("Expected TERM_PROGRAM to take precedence, got TerminalType = %q", caps.TerminalType)
		}
	})

	t.Run("fallback to TERM when TERM_PROGRAM not set", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "")
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("KITTY_WINDOW_ID", "")

		caps := DetectTerminalCapabilities()

		if caps.TerminalType != "xterm-256color" {
			t.Errorf("Expected fallback to TERM, got TerminalType = %q", caps.TerminalType)
		}
	})
}
