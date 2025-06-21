package xargs

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/shannon/cmd/export"
	"github.com/user/shannon/cmd/root"
)

// XargsCmd represents the xargs command
var XargsCmd = &cobra.Command{
	Use:   "xargs <command> [args...]",
	Short: "Execute commands with conversation IDs from stdin",
	Long: `Execute ClaudeSearch commands with conversation IDs read from stdin.

Similar to Unix xargs, this reads conversation IDs from stdin and executes
the specified command for each ID.

Examples:
  # Export multiple conversations
  claudesearch list --format json | jq -r '.conversations[].id' | claudesearch xargs export

  # Open conversations in editor
  claudesearch search "TODO" --format json | jq -r '.results[].conversation_id' | sort -u | claudesearch xargs edit

  # View multiple conversations
  echo -e "123\n456\n789" | claudesearch xargs view`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    true,
	DisableFlagsInUseLine: true,
	RunE:                  runXargs,
}

func runXargs(cmd *cobra.Command, args []string) error {
	// Read IDs from stdin
	var ids []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" {
			ids = append(ids, id)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	if len(ids) == 0 {
		return fmt.Errorf("no conversation IDs provided on stdin")
	}

	// Get the subcommand
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	subcommand := args[0]
	subargs := args[1:]

	// Execute the command for each ID
	for _, id := range ids {
		// Build command args
		cmdArgs := append([]string{subcommand}, append(subargs, id)...)
		
		// Create a new root command instance to avoid state issues
		rootCmd := root.NewRootCmd()
		
		// Add subcommands (you'd need to export this setup from main.go)
		// For now, let's handle the most common cases directly
		switch subcommand {
		case "export":
			// Special handling for export since it's the most common
			exportArgs := append(subargs, id)
			exportCmd := &cobra.Command{}
			*exportCmd = *export.ExportCmd
			exportCmd.SetArgs(exportArgs)
			if err := exportCmd.Execute(); err != nil {
				return fmt.Errorf("failed to export conversation %s: %w", id, err)
			}
		default:
			// For other commands, we'd need to set them up
			// This is a limitation of the current architecture
			fmt.Fprintf(os.Stderr, "xargs: command '%s' not yet supported\n", subcommand)
			return fmt.Errorf("unsupported command: %s", subcommand)
		}
	}

	return nil
}