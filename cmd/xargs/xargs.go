package xargs

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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

	// Get the root command to access all subcommands
	rootCmd := cmd.Root()

	// Find the target subcommand using Cobra's Find
	targetCmd, _, err := rootCmd.Find([]string{subcommand})
	if err != nil {
		return fmt.Errorf("command '%s' not found: %w", subcommand, err)
	}

	// Verify this isn't the xargs command itself (prevent infinite recursion)
	if targetCmd == cmd {
		return fmt.Errorf("cannot use xargs with itself")
	}

	// Execute the command for each ID
	for _, id := range ids {
		// Create a copy of the target command to avoid state issues
		cmdCopy := &cobra.Command{}
		*cmdCopy = *targetCmd

		// Build the complete argument list: subcommand flags + ID
		cmdArgs := append(subargs, id)
		cmdCopy.SetArgs(cmdArgs)

		// Reset flags to avoid state pollution between executions
		if err := cmdCopy.Flags().Parse([]string{}); err != nil {
			return fmt.Errorf("failed to reset flags for '%s': %w", subcommand, err)
		}

		// Execute the command
		if err := cmdCopy.Execute(); err != nil {
			return fmt.Errorf("failed to execute '%s' for conversation %s: %w", subcommand, id, err)
		}
	}

	return nil
}
