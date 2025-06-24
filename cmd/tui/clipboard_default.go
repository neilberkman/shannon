//go:build !darwin && !windows

package tui

import "fmt"

// initClipboard is a no-op on systems without clipboard support
func initClipboard() error {
	// No initialization needed for fallback
	return nil
}

// writeToClipboard attempts to use xclip or xsel if available
func writeToClipboard(text string) error {
	// On Linux, we can't use the golang.design/x/clipboard library without X11 headers
	// This is a fallback that returns an error indicating clipboard is not available
	return fmt.Errorf("clipboard support not available (requires X11 development headers)")
}