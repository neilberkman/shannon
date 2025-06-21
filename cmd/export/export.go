package export

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/search"
)

var (
	outputFormat string
	outputFile   string
	outputDir    string
	stdout       bool
	quiet        bool
)

// ExportCmd represents the export command
var ExportCmd = &cobra.Command{
	Use:   "export [conversation-id...]",
	Short: "Export conversations to files",
	Long: `Export one or more conversations to files in various formats.

Examples:
  # Export single conversation (stdout by default)
  claudesearch export 123
  
  # Export as JSON to stdout
  claudesearch export 123 --format json

  # Export to file
  claudesearch export 123 -o conversation.md

  # Export multiple conversations to directory
  claudesearch export 123 456 789 -d exports/

  # Pipe to other tools
  claudesearch export 123 | grep "TODO"
  claudesearch export 123 --format json | jq '.messages[].text'
  
  # Read IDs from stdin with -
  claudesearch search "bug" --format json | jq -r '.results[].conversation_id' | claudesearch export -`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExport,
}

func init() {
	ExportCmd.Flags().StringVarP(&outputFormat, "format", "f", "markdown", "output format: markdown, text, or json")
	ExportCmd.Flags().StringVarP(&outputFile, "output", "o", "", "output to file instead of stdout")
	ExportCmd.Flags().StringVarP(&outputDir, "dir", "d", "", "output directory (required for multiple conversations)")
	ExportCmd.Flags().BoolVar(&stdout, "stdout", false, "force output to stdout (deprecated, now default)")
	ExportCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress status messages")
}

func runExport(cmd *cobra.Command, args []string) error {
	// Handle stdin input with "-"
	if len(args) == 1 && args[0] == "-" {
		// Read IDs from stdin
		scanner := bufio.NewScanner(os.Stdin)
		args = []string{}
		for scanner.Scan() {
			id := strings.TrimSpace(scanner.Text())
			if id != "" {
				args = append(args, id)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
		if len(args) == 0 {
			return fmt.Errorf("no conversation IDs provided on stdin")
		}
	}
	// Validate arguments
	if len(args) > 1 && outputFile != "" {
		return fmt.Errorf("cannot use -o with multiple conversations, use -d instead")
	}

	if len(args) > 1 && outputDir == "" {
		return fmt.Errorf("multiple conversations require -d flag to specify output directory")
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

	// Export each conversation
	for _, idStr := range args {
		convID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid conversation ID %s: %w", idStr, err)
		}

		if err := exportConversation(engine, convID, len(args) > 1, quiet); err != nil {
			return fmt.Errorf("failed to export conversation %d: %w", convID, err)
		}
	}

	return nil
}

func exportConversation(engine *search.Engine, convID int64, multiple bool, quiet bool) error {
	// Get conversation and messages
	conv, messages, err := engine.GetConversation(convID)
	if err != nil {
		return err
	}

	// Generate content based on format
	var content string
	switch outputFormat {
	case "json":
		content, err = formatJSON(conv, messages)
	case "text":
		content = formatText(conv, messages)
	default: // markdown
		content = formatMarkdown(conv, messages)
	}

	if err != nil {
		return err
	}

	// Determine output destination
	// Default to stdout for single exports unless file/dir specified
	if !multiple && outputFile == "" && outputDir == "" {
		fmt.Print(content)
		return nil
	}

	// Generate filename
	var filename string
	if outputFile != "" && !multiple {
		filename = outputFile
	} else {
		// Sanitize conversation name for filename
		safeName := strings.ReplaceAll(conv.Name, "/", "-")
		safeName = strings.ReplaceAll(safeName, ":", "-")
		safeName = strings.TrimSpace(safeName)
		if len(safeName) > 100 {
			safeName = safeName[:100]
		}

		ext := ".md"
		switch outputFormat {
		case "json":
			ext = ".json"
		case "text":
			ext = ".txt"
		}

		filename = fmt.Sprintf("%d-%s%s", conv.ID, safeName, ext)

		if outputDir != "" {
			filename = filepath.Join(outputDir, filename)
		}
	}

	// Create directory if needed
	dir := filepath.Dir(filename)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if !quiet {
		fmt.Printf("Exported conversation %d to %s\n", conv.ID, filename)
	}
	return nil
}

func formatMarkdown(conv *models.Conversation, messages []*models.Message) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# %s\n\n", conv.Name))
	sb.WriteString(fmt.Sprintf("**ID:** %d  \n", conv.ID))
	sb.WriteString(fmt.Sprintf("**Created:** %s  \n", conv.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Updated:** %s  \n", conv.UpdatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Messages:** %d  \n\n", len(messages)))
	sb.WriteString("---\n\n")

	// Messages
	for i, msg := range messages {
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")

		if msg.Sender == "human" {
			sb.WriteString(fmt.Sprintf("## Human (%s)\n\n", timestamp))
		} else {
			sb.WriteString(fmt.Sprintf("## Assistant (%s)\n\n", timestamp))
		}

		// Handle code blocks in message text
		text := strings.ReplaceAll(msg.Text, "```", "````")
		sb.WriteString(text)
		sb.WriteString("\n\n")

		// Add separator between messages (except last)
		if i < len(messages)-1 {
			sb.WriteString("---\n\n")
		}
	}

	return sb.String()
}

func formatText(conv *models.Conversation, messages []*models.Message) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("CONVERSATION: %s\n", conv.Name))
	sb.WriteString(fmt.Sprintf("ID: %d\n", conv.ID))
	sb.WriteString(fmt.Sprintf("Created: %s\n", conv.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Updated: %s\n", conv.UpdatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Messages: %d\n", len(messages)))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Messages
	for _, msg := range messages {
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
		sender := strings.ToUpper(msg.Sender)

		sb.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, sender))
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString(msg.Text)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func formatJSON(conv *models.Conversation, messages []*models.Message) (string, error) {
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
