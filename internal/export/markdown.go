package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/models"
)

// ConversationToMarkdown exports a conversation and its messages to a markdown file
func ConversationToMarkdown(conv *models.Conversation, messages []*models.Message, outputPath string) error {
	var sb strings.Builder

	// Write conversation header
	sb.WriteString(fmt.Sprintf("# %s\n\n", conv.Name))
	sb.WriteString(fmt.Sprintf("**Conversation ID:** %d\n\n", conv.ID))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n\n", conv.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Updated:** %s\n\n", conv.UpdatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Messages:** %d\n\n", len(messages)))
	sb.WriteString("---\n\n")

	// Extract artifacts from messages
	artifactExtractor := artifacts.NewExtractor()
	messageArtifacts := make(map[int64][]*artifacts.Artifact)
	for _, msg := range messages {
		if msg.Sender == "assistant" {
			msgArtifacts, _ := artifactExtractor.ExtractFromMessage(msg)
			if len(msgArtifacts) > 0 {
				messageArtifacts[msg.ID] = msgArtifacts
			}
		}
	}

	// Write messages
	for i, msg := range messages {
		// Message header with sender and timestamp
		sender := msg.Sender
		if len(sender) > 0 {
			sender = strings.ToUpper(sender[:1]) + sender[1:]
		}
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("## %s (%s)\n\n", sender, timestamp))

		// Message content
		content := msg.Text

		// Remove artifact tags if artifacts are present
		if messageArtifacts[msg.ID] != nil {
			content = removeArtifactTags(content, artifactExtractor)
		}

		sb.WriteString(content)
		sb.WriteString("\n\n")

		// Add artifacts if present
		if artifacts := messageArtifacts[msg.ID]; artifacts != nil {
			for _, artifact := range artifacts {
				sb.WriteString(formatArtifactMarkdown(artifact))
				sb.WriteString("\n\n")
			}
		}

		// Add separator between messages (except after last)
		if i < len(messages)-1 {
			sb.WriteString("---\n\n")
		}
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write markdown file: %w", err)
	}

	return nil
}

// GenerateDefaultFilename creates a default filename for a conversation export
func GenerateDefaultFilename(conv *models.Conversation) string {
	// Sanitize conversation name for filename
	name := conv.Name
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")

	// Trim and limit length
	name = strings.TrimSpace(name)
	if len(name) > 100 {
		name = name[:100]
	}

	// Add timestamp to make unique
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.md", name, timestamp)
}

// removeArtifactTags removes artifact XML tags from content
func removeArtifactTags(content string, extractor *artifacts.Extractor) string {
	return extractor.ArtifactRegex.ReplaceAllString(content, "")
}

// formatArtifactMarkdown formats an artifact as markdown
func formatArtifactMarkdown(artifact *artifacts.Artifact) string {
	var sb strings.Builder

	// Artifact header
	sb.WriteString(fmt.Sprintf("### Artifact: %s\n\n", artifact.Title))
	sb.WriteString(fmt.Sprintf("**Type:** %s", artifact.Type))
	if artifact.Language != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", artifact.Language))
	}
	sb.WriteString("\n\n")

	// Artifact content in code block
	language := artifact.Language
	if language == "" {
		// Determine language from type
		switch artifact.Type {
		case "application/vnd.ant.code":
			language = "text"
		case "text/markdown":
			language = "markdown"
		case "text/html":
			language = "html"
		case "image/svg+xml":
			language = "xml"
		case "application/vnd.ant.react":
			language = "jsx"
		case "application/vnd.ant.mermaid":
			language = "mermaid"
		default:
			language = "text"
		}
	}

	sb.WriteString(fmt.Sprintf("```%s\n", language))
	sb.WriteString(artifact.Content)
	if !strings.HasSuffix(artifact.Content, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("```")

	return sb.String()
}
