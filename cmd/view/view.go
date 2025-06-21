package view

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/search"
)

var showBranches bool

// ViewCmd represents the view command
var ViewCmd = &cobra.Command{
	Use:   "view [conversation-id]",
	Short: "View a conversation with all messages",
	Long: `View a full conversation with all messages, including branch information if available.

Example:
  claudesearch view 123
  claudesearch view 123 --branches`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func init() {
	ViewCmd.Flags().BoolVar(&showBranches, "branches", false, "show branch information")
}

func runView(cmd *cobra.Command, args []string) error {
	// Parse conversation ID
	convID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid conversation ID: %w", err)
	}

	// Get configuration
	cfg := config.Get()

	// Open database
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create search engine
	engine := search.NewEngine(database)

	// Get conversation and messages
	conv, messages, err := engine.GetConversation(convID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Display conversation info
	fmt.Printf("=== Conversation: %s ===\n", conv.Name)
	fmt.Printf("ID: %d\n", conv.ID)
	fmt.Printf("UUID: %s\n", conv.UUID)
	fmt.Printf("Created: %s\n", conv.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", conv.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Messages: %d\n\n", len(messages))

	// Display messages
	currentBranch := int64(-1)
	for i, msg := range messages {
		// Show branch info if requested and branch changed
		if showBranches && msg.BranchID != currentBranch {
			currentBranch = msg.BranchID
			fmt.Printf("\n--- Branch %d ---\n", currentBranch)
		}

		// Message header
		fmt.Printf("[%d] %s (%s)\n", i+1, msg.Sender, msg.CreatedAt.Format("2006-01-02 15:04:05"))

		// Show parent info if exists
		if msg.ParentID != nil {
			fmt.Printf("    Parent: Message #%d\n", *msg.ParentID)
		}

		// Message content (truncate if too long)
		content := msg.Text
		if len(content) > 500 {
			content = content[:497] + "..."
		}
		fmt.Printf("    %s\n\n", content)
	}

	return nil
}
