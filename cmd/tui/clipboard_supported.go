//go:build darwin || windows

package tui

import clipboard "golang.design/x/clipboard"

// initClipboard initializes the clipboard
func initClipboard() error {
	return clipboard.Init()
}

// writeToClipboard writes text to the clipboard
func writeToClipboard(text string) error {
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}