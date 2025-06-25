//go:build darwin || windows

package tui

import (
	"fmt"
	"os"

	clipboard "golang.design/x/clipboard"
)

var clipboardInitialized bool
var clipboardErr error

// initClipboard initializes the clipboard
func initClipboard() error {
	// Skip initialization in test environment
	if os.Getenv("GO_TEST") == "1" || os.Getenv("CI") != "" {
		return nil
	}

	// Catch any panics from clipboard.Init()
	defer func() {
		if r := recover(); r != nil {
			clipboardErr = fmt.Errorf("clipboard initialization panicked: %v", r)
			clipboardInitialized = false
		}
	}()

	clipboardErr = clipboard.Init()
	clipboardInitialized = (clipboardErr == nil)
	return clipboardErr
}

// writeToClipboard writes text to the clipboard
func writeToClipboard(text string) error {
	// Skip in test environment
	if os.Getenv("GO_TEST") == "1" || os.Getenv("CI") != "" {
		return nil
	}

	if !clipboardInitialized {
		if clipboardErr != nil {
			return clipboardErr
		}
		return fmt.Errorf("clipboard not initialized")
	}

	// Catch any panics from clipboard.Write()
	defer func() {
		if r := recover(); r != nil {
			clipboardErr = fmt.Errorf("clipboard write panicked: %v", r)
		}
	}()

	// Try to write to clipboard
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}
