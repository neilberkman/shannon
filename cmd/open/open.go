package open

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// OpenCmd represents the open command
var OpenCmd = &cobra.Command{
	Use:   "open [conversation-id]",
	Short: "Open conversation in browser (reads ID from stdin if not provided)",
	Long: `Open a conversation in Claude's web interface.

Can read conversation ID from stdin, making it pipe-friendly:

Examples:
  # Open directly
  claudesearch open 123

  # Pipe from search
  claudesearch search "bug" --format json | jq -r '.results[0].conversation_id' | claudesearch open

  # Open from list
  claudesearch list --format json | jq -r '.conversations[0].id' | claudesearch open`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOpen,
}

func runOpen(cmd *cobra.Command, args []string) error {
	var convID string

	// Get conversation ID from args or stdin
	if len(args) > 0 {
		convID = args[0]
	} else {
		// Read from stdin
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			convID = strings.TrimSpace(scanner.Text())
		}
		if convID == "" {
			return fmt.Errorf("no conversation ID provided")
		}
	}

	// Validate it's a number
	if _, err := strconv.ParseInt(convID, 10, 64); err != nil {
		return fmt.Errorf("invalid conversation ID: %s", convID)
	}

	// For now, just open a search URL since we don't have the actual Claude URL structure
	// In reality, you'd need to map the conversation ID to Claude's URL format
	url := fmt.Sprintf("https://claude.ai/chat/%s", convID)

	// Open in browser
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	fmt.Printf("Opening conversation %s in browser...\n", convID)
	return nil
}
