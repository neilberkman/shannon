package rendering

import (
	"os"
	"strings"
)

// TerminalCapabilities represents what features the current terminal supports
type TerminalCapabilities struct {
	SupportsHyperlinks    bool
	SupportsGraphics      bool
	SupportsAdvancedInput bool
	TerminalType          string
}

// DetectTerminalCapabilities detects what features the current terminal supports
func DetectTerminalCapabilities() *TerminalCapabilities {
	caps := &TerminalCapabilities{}

	// Check environment variables for terminal identification
	termProgram := os.Getenv("TERM_PROGRAM")
	termName := os.Getenv("TERM")

	caps.TerminalType = termProgram
	if caps.TerminalType == "" {
		caps.TerminalType = termName
	}

	// Detect based on known terminal programs
	switch termProgram {
	case "ghostty":
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = true
		caps.SupportsAdvancedInput = true
	case "kitty":
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = true
		caps.SupportsAdvancedInput = true
	case "wezterm":
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = true
		caps.SupportsAdvancedInput = true
	case "iTerm.app":
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = true // iTerm2 protocol
		caps.SupportsAdvancedInput = false
	case "vscode":
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = false
		caps.SupportsAdvancedInput = false
	}

	// Check for specific environment variables that indicate capability
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		caps.SupportsHyperlinks = true
		caps.SupportsGraphics = true
		caps.SupportsAdvancedInput = true
	}

	// Check TERM variable for additional hints
	if strings.Contains(termName, "xterm") {
		// Most modern xterm variants support hyperlinks
		caps.SupportsHyperlinks = true
	}

	// Conservative fallback - if we can't detect, assume basic terminal
	// Better to have working text than broken escape codes

	return caps
}

// IsHyperlinksSupported returns true if the terminal supports OSC 8 hyperlinks
func IsHyperlinksSupported() bool {
	return DetectTerminalCapabilities().SupportsHyperlinks
}

// IsGraphicsSupported returns true if the terminal supports graphics protocols
func IsGraphicsSupported() bool {
	return DetectTerminalCapabilities().SupportsGraphics
}

// GetTerminalInfo returns human-readable terminal information
func GetTerminalInfo() string {
	caps := DetectTerminalCapabilities()

	info := "Terminal: " + caps.TerminalType

	features := []string{}
	if caps.SupportsHyperlinks {
		features = append(features, "hyperlinks")
	}
	if caps.SupportsGraphics {
		features = append(features, "graphics")
	}
	if caps.SupportsAdvancedInput {
		features = append(features, "advanced-input")
	}

	if len(features) > 0 {
		info += " (supports: " + strings.Join(features, ", ") + ")"
	}

	return info
}
