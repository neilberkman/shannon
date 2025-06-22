package rendering

import (
	"os"
	"testing"
)

func TestDetectTerminalCapabilities(t *testing.T) {
	tests := []struct {
		name                  string
		setupEnv              func()
		expectedTerminalType  string
		expectedHyperlinks    bool
		expectedGraphics      bool
		expectedAdvancedInput bool
	}{
		{
			name: "Ghostty terminal",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			expectedTerminalType:  "ghostty",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name: "Kitty terminal",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "kitty")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			expectedTerminalType:  "kitty",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name: "Kitty via KITTY_WINDOW_ID",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("TERM")
				os.Setenv("KITTY_WINDOW_ID", "1")
			},
			expectedTerminalType:  "",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name: "WezTerm terminal",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "wezterm")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			expectedTerminalType:  "wezterm",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: true,
		},
		{
			name: "iTerm2 terminal",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "iTerm.app")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			expectedTerminalType:  "iTerm.app",
			expectedHyperlinks:    true,
			expectedGraphics:      true,
			expectedAdvancedInput: false,
		},
		{
			name: "VS Code terminal",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "vscode")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			expectedTerminalType:  "vscode",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name: "xterm-based terminal",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "xterm-256color")
			},
			expectedTerminalType:  "xterm-256color",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name: "basic terminal",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "xterm")
			},
			expectedTerminalType:  "xterm",
			expectedHyperlinks:    true,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name: "unknown terminal",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "unknown")
			},
			expectedTerminalType:  "unknown",
			expectedHyperlinks:    false,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
		{
			name: "no terminal info",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("TERM")
			},
			expectedTerminalType:  "",
			expectedHyperlinks:    false,
			expectedGraphics:      false,
			expectedAdvancedInput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origTermProgram := os.Getenv("TERM_PROGRAM")
			origTerm := os.Getenv("TERM")
			origKittyWindowID := os.Getenv("KITTY_WINDOW_ID")

			// Setup test environment
			tt.setupEnv()

			// Run test
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

			// Restore original environment
			if origTermProgram != "" {
				os.Setenv("TERM_PROGRAM", origTermProgram)
			} else {
				os.Unsetenv("TERM_PROGRAM")
			}
			if origTerm != "" {
				os.Setenv("TERM", origTerm)
			} else {
				os.Unsetenv("TERM")
			}
			if origKittyWindowID != "" {
				os.Setenv("KITTY_WINDOW_ID", origKittyWindowID)
			} else {
				os.Unsetenv("KITTY_WINDOW_ID")
			}
		})
	}
}

func TestIsHyperlinksSupported(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func()
		expected bool
	}{
		{
			name: "Ghostty supports hyperlinks",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			expected: true,
		},
		{
			name: "Unknown terminal doesn't support hyperlinks",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "dumb")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origTermProgram := os.Getenv("TERM_PROGRAM")
			origTerm := os.Getenv("TERM")
			origKittyWindowID := os.Getenv("KITTY_WINDOW_ID")

			tt.setupEnv()

			result := IsHyperlinksSupported()
			if result != tt.expected {
				t.Errorf("IsHyperlinksSupported() = %t, want %t", result, tt.expected)
			}

			// Restore original environment
			if origTermProgram != "" {
				os.Setenv("TERM_PROGRAM", origTermProgram)
			} else {
				os.Unsetenv("TERM_PROGRAM")
			}
			if origTerm != "" {
				os.Setenv("TERM", origTerm)
			} else {
				os.Unsetenv("TERM")
			}
			if origKittyWindowID != "" {
				os.Setenv("KITTY_WINDOW_ID", origKittyWindowID)
			} else {
				os.Unsetenv("KITTY_WINDOW_ID")
			}
		})
	}
}

func TestIsGraphicsSupported(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func()
		expected bool
	}{
		{
			name: "Ghostty supports graphics",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			expected: true,
		},
		{
			name: "VS Code doesn't support graphics",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "vscode")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origTermProgram := os.Getenv("TERM_PROGRAM")
			origTerm := os.Getenv("TERM")
			origKittyWindowID := os.Getenv("KITTY_WINDOW_ID")

			tt.setupEnv()

			result := IsGraphicsSupported()
			if result != tt.expected {
				t.Errorf("IsGraphicsSupported() = %t, want %t", result, tt.expected)
			}

			// Restore original environment
			if origTermProgram != "" {
				os.Setenv("TERM_PROGRAM", origTermProgram)
			} else {
				os.Unsetenv("TERM_PROGRAM")
			}
			if origTerm != "" {
				os.Setenv("TERM", origTerm)
			} else {
				os.Unsetenv("TERM")
			}
			if origKittyWindowID != "" {
				os.Setenv("KITTY_WINDOW_ID", origKittyWindowID)
			} else {
				os.Unsetenv("KITTY_WINDOW_ID")
			}
		})
	}
}

func TestGetTerminalInfo(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func()
		contains []string
	}{
		{
			name: "Ghostty terminal info",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			contains: []string{
				"Terminal: ghostty",
				"supports:",
				"hyperlinks",
				"graphics",
				"advanced-input",
			},
		},
		{
			name: "VS Code terminal info",
			setupEnv: func() {
				os.Setenv("TERM_PROGRAM", "vscode")
			},
			contains: []string{
				"Terminal: vscode",
				"supports:",
				"hyperlinks",
			},
		},
		{
			name: "Basic terminal info",
			setupEnv: func() {
				os.Unsetenv("TERM_PROGRAM")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Setenv("TERM", "dumb")
			},
			contains: []string{
				"Terminal: dumb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origTermProgram := os.Getenv("TERM_PROGRAM")
			origTerm := os.Getenv("TERM")
			origKittyWindowID := os.Getenv("KITTY_WINDOW_ID")

			tt.setupEnv()

			result := GetTerminalInfo()

			for _, expectedSubstring := range tt.contains {
				if !containsStringSimple(result, expectedSubstring) {
					t.Errorf("GetTerminalInfo() result %q does not contain expected substring %q", result, expectedSubstring)
				}
			}

			// Restore original environment
			if origTermProgram != "" {
				os.Setenv("TERM_PROGRAM", origTermProgram)
			} else {
				os.Unsetenv("TERM_PROGRAM")
			}
			if origTerm != "" {
				os.Setenv("TERM", origTerm)
			} else {
				os.Unsetenv("TERM")
			}
			if origKittyWindowID != "" {
				os.Setenv("KITTY_WINDOW_ID", origKittyWindowID)
			} else {
				os.Unsetenv("KITTY_WINDOW_ID")
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
	// Save original environment
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origTerm := os.Getenv("TERM")
	origKittyWindowID := os.Getenv("KITTY_WINDOW_ID")

	defer func() {
		// Restore original environment
		if origTermProgram != "" {
			os.Setenv("TERM_PROGRAM", origTermProgram)
		} else {
			os.Unsetenv("TERM_PROGRAM")
		}
		if origTerm != "" {
			os.Setenv("TERM", origTerm)
		} else {
			os.Unsetenv("TERM")
		}
		if origKittyWindowID != "" {
			os.Setenv("KITTY_WINDOW_ID", origKittyWindowID)
		} else {
			os.Unsetenv("KITTY_WINDOW_ID")
		}
	}()

	// TERM_PROGRAM should take precedence over TERM
	os.Setenv("TERM_PROGRAM", "ghostty")
	os.Setenv("TERM", "xterm")

	caps := DetectTerminalCapabilities()

	if caps.TerminalType != "ghostty" {
		t.Errorf("Expected TERM_PROGRAM to take precedence, got TerminalType = %q", caps.TerminalType)
	}

	// When TERM_PROGRAM is not set, should fall back to TERM
	os.Unsetenv("TERM_PROGRAM")
	os.Setenv("TERM", "xterm-256color")

	caps = DetectTerminalCapabilities()

	if caps.TerminalType != "xterm-256color" {
		t.Errorf("Expected fallback to TERM, got TerminalType = %q", caps.TerminalType)
	}
}
