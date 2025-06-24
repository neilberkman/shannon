//go:build darwin || windows

package tui

import (
	"os"
	
	clipboard "golang.design/x/clipboard"
)

// initClipboard initializes the clipboard
func initClipboard() error {
	// Skip initialization in test environment
	if os.Getenv("GO_TEST") == "1" || os.Getenv("CI") != "" {
		return nil
	}
	return clipboard.Init()
}

// writeToClipboard writes text to the clipboard
func writeToClipboard(text string) error {
	// Skip in test environment
	if os.Getenv("GO_TEST") == "1" || os.Getenv("CI") != "" {
		return nil
	}
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}