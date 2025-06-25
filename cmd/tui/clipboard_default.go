//go:build !darwin && !windows

package tui

import (
	"bytes"
	"fmt"
	"os/exec"
)

// initClipboard is a no-op on systems without clipboard support
func initClipboard() error {
	// Check if xclip or xsel is available
	if _, err := exec.LookPath("xclip"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("wl-copy"); err == nil {
		// Wayland clipboard
		return nil
	}
	return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-clipboard)")
}

// writeToClipboard attempts to use xclip, xsel, or wl-copy if available
func writeToClipboard(text string) error {
	// Try xclip first (most common)
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = bytes.NewReader([]byte(text))
		return cmd.Run()
	}

	// Try xsel
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = bytes.NewReader([]byte(text))
		return cmd.Run()
	}

	// Try wl-copy for Wayland
	if _, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = bytes.NewReader([]byte(text))
		return cmd.Run()
	}

	return fmt.Errorf("no clipboard tool available")
}
