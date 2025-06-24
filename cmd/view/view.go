package view

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/search"
	"github.com/spf13/cobra"
)

var (
	showBranches  bool
	showArtifacts bool
	fullArtifacts bool
)

// ViewCmd represents the view command
var ViewCmd = &cobra.Command{
	Use:   "view [conversation-id]",
	Short: "View a conversation with all messages",
	Long: `View a full conversation with all messages, including branch information if available.

Example:
  shannon view 123
  shannon view 123 --branches
  shannon view 123 --show-artifacts
  shannon view 123 --full-artifacts`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func init() {
	ViewCmd.Flags().BoolVar(&showBranches, "branches", false, "show branch information")
	ViewCmd.Flags().BoolVar(&showArtifacts, "show-artifacts", true, "show artifacts inline")
	ViewCmd.Flags().BoolVar(&fullArtifacts, "full-artifacts", false, "show complete artifact content")
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
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

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

	// Extract artifacts if requested
	var artifactExtractor *artifacts.Extractor
	var messageArtifacts map[int64][]*artifacts.Artifact

	if showArtifacts {
		artifactExtractor = artifacts.NewExtractor()
		messageArtifacts = make(map[int64][]*artifacts.Artifact)

		// Extract artifacts from all messages
		for _, msg := range messages {
			if msg.Sender == "assistant" {
				msgArtifacts, _ := artifactExtractor.ExtractFromMessage(msg)
				if len(msgArtifacts) > 0 {
					messageArtifacts[msg.ID] = msgArtifacts
				}
			}
		}
	}

	// Display messages
	currentBranch := int64(-1)
	renderer := artifacts.NewTerminalRenderer()

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

		// Process message content
		content := msg.Text

		// If showing artifacts, remove artifact tags from display
		if showArtifacts && messageArtifacts[msg.ID] != nil {
			content = removeArtifactTags(content)
		}

		// Display message text (truncated if needed)
		lines := strings.Split(content, "\n")
		maxLines := 20
		if !fullArtifacts {
			if len(lines) > maxLines {
				fmt.Printf("    %s\n", strings.Join(lines[:maxLines], "\n    "))
				fmt.Printf("    ... (%d more lines)\n", len(lines)-maxLines)
			} else {
				fmt.Printf("    %s\n", strings.Join(lines, "\n    "))
			}
		} else {
			fmt.Printf("    %s\n", strings.Join(lines, "\n    "))
		}

		// Display artifacts inline if present
		if showArtifacts && messageArtifacts[msg.ID] != nil {
			fmt.Println()
			for j, artifact := range messageArtifacts[msg.ID] {
				if fullArtifacts {
					fmt.Printf("    %s\n", renderer.RenderDetail(artifact))
				} else {
					maxHeight := 10
					inline := renderer.RenderInline(artifact, false, true, maxHeight)
					// Indent the artifact display
					lines := strings.Split(inline, "\n")
					for _, line := range lines {
						fmt.Printf("    %s\n", line)
					}
				}

				if j < len(messageArtifacts[msg.ID])-1 {
					fmt.Println()
				}
			}
		}

		fmt.Println()
	}

	return nil
}

// removeArtifactTags removes artifact XML tags from content
func removeArtifactTags(content string) string {
	// Simple regex to remove artifact tags
	artifactRegex := artifacts.NewExtractor().ArtifactRegex
	return artifactRegex.ReplaceAllString(content, "[Artifact: see below]")
}
