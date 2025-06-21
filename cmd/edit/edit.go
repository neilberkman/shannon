package edit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/models"
	"github.com/user/shannon/internal/search"
)

var (
	editor       string
	outputFormat string
)

// EditCmd represents the edit command
var EditCmd = &cobra.Command{
	Use:   "edit [conversation-id]",
	Short: "Open a conversation in your editor",
	Long: `Open a conversation in your editor for viewing or editing.

The conversation will be exported to a temporary file and opened in your
editor. The editor is determined by the --editor flag, $EDITOR environment
variable, or common defaults.

Examples:
  # Open conversation in default editor
  claudesearch edit 123

  # Open with specific editor
  claudesearch edit 123 --editor vim

  # Open as JSON
  claudesearch edit 123 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

func init() {
	EditCmd.Flags().StringVarP(&editor, "editor", "e", "", "editor to use (defaults to $EDITOR)")
	EditCmd.Flags().StringVarP(&outputFormat, "format", "f", "markdown", "format: markdown, json, or text")
}

func runEdit(cmd *cobra.Command, args []string) error {
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

	// Generate content based on format
	var content string
	var ext string
	switch outputFormat {
	case "json":
		content, err = formatJSON(conv, messages)
		ext = ".json"
	case "text":
		content = formatText(conv, messages)
		ext = ".txt"
	default: // markdown
		content = formatMarkdown(conv, messages)
		ext = ".md"
	}
	if err != nil {
		return err
	}

	// Create temporary file
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("claudesearch-%d%s", conv.ID, ext))
	
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Determine editor
	editorCmd := determineEditor(editor)
	if editorCmd == "" {
		return fmt.Errorf("no editor found; set $EDITOR or use --editor flag")
	}

	// Open in editor
	fmt.Printf("Opening conversation %d in %s...\n", conv.ID, editorCmd)
	editCmd := exec.Command(editorCmd, tmpFile)
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr
	
	if err := editCmd.Run(); err != nil {
		return fmt.Errorf("failed to run editor: %w", err)
	}

	fmt.Printf("\nConversation was opened in: %s\n", tmpFile)
	return nil
}

func determineEditor(specified string) string {
	// Use specified editor if provided
	if specified != "" {
		return specified
	}

	// Try $EDITOR environment variable
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	// Try common editors
	editors := []string{"vim", "nvim", "nano", "emacs", "vi", "code", "subl"}
	for _, editor := range editors {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	return ""
}

// Format functions (reused from export command)
func formatMarkdown(conv *models.Conversation, messages []*models.Message) string {
	// Same implementation as in export command
	var content string
	content += fmt.Sprintf("# %s\n\n", conv.Name)
	content += fmt.Sprintf("**ID:** %d  \n", conv.ID)
	content += fmt.Sprintf("**Created:** %s  \n", conv.CreatedAt.Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("**Updated:** %s  \n", conv.UpdatedAt.Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("**Messages:** %d  \n\n", len(messages))
	content += "---\n\n"

	for i, msg := range messages {
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
		
		if msg.Sender == "human" {
			content += fmt.Sprintf("## Human (%s)\n\n", timestamp)
		} else {
			content += fmt.Sprintf("## Assistant (%s)\n\n", timestamp)
		}
		
		content += msg.Text + "\n\n"
		
		if i < len(messages)-1 {
			content += "---\n\n"
		}
	}

	return content
}

func formatText(conv *models.Conversation, messages []*models.Message) string {
	var content string
	content += fmt.Sprintf("CONVERSATION: %s\n", conv.Name)
	content += fmt.Sprintf("ID: %d\n", conv.ID)
	content += fmt.Sprintf("Created: %s\n", conv.CreatedAt.Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("Updated: %s\n", conv.UpdatedAt.Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("Messages: %d\n", len(messages))
	content += "================================================================================\n\n"

	for _, msg := range messages {
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
		sender := msg.Sender
		if sender == "human" {
			sender = "HUMAN"
		} else {
			sender = "ASSISTANT"
		}
		
		content += fmt.Sprintf("[%s] %s\n", timestamp, sender)
		content += "----------------------------------------\n"
		content += msg.Text + "\n\n"
	}

	return content
}

func formatJSON(conv *models.Conversation, messages []*models.Message) (string, error) {
	// Same implementation as in export command
	data := map[string]interface{}{
		"conversation": map[string]interface{}{
			"id":         conv.ID,
			"uuid":       conv.UUID,
			"name":       conv.Name,
			"created_at": conv.CreatedAt,
			"updated_at": conv.UpdatedAt,
		},
		"messages": messages,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonBytes), nil
}