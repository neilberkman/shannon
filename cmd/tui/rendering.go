package tui

import (
	"fmt"
	"strings"

	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/rendering"
)

// RenderConversation renders the full conversation view with plain text (debugging hang)
// This is shared between browse and search models
func RenderConversation(conversation *models.Conversation, messages []*models.Message, width int) string {
	// DEBUG: Use plain text until we find the real hang
	return renderConversationPlain(conversation, messages, width)
}

// renderConversationPlain provides fallback plain text rendering
func renderConversationPlain(conversation *models.Conversation, messages []*models.Message, width int) string {
	var sb strings.Builder

	// Header
	sb.WriteString(HeaderStyle.Render(fmt.Sprintf("Conversation: %s", conversation.Name)))
	sb.WriteString("\n")
	sb.WriteString(DateStyle.Render(fmt.Sprintf("Messages: %d | Updated: %s",
		len(messages),
		conversation.UpdatedAt.Format("2006-01-02 15:04"))))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n\n")

	// Messages
	for i, msg := range messages {
		// Message header
		displaySender := rendering.FormatSender(msg.Sender)
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")

		if msg.Sender == "human" {
			sb.WriteString(ConversationStyle.Bold(true).Render(fmt.Sprintf("%s (%s)", displaySender, timestamp)))
		} else {
			sb.WriteString(AssistantStyle.Render(fmt.Sprintf("%s (%s)", displaySender, timestamp)))
		}
		sb.WriteString("\n")

		// Message text with word wrap
		text := strings.TrimSpace(msg.Text)
		wrappedText := simpleWordWrap(text, width-4)
		sb.WriteString(wrappedText)

		if i < len(messages)-1 {
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("─", width/2))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// simpleWordWrap wraps text to the specified width, preserving line breaks
func simpleWordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		// Wrap long lines
		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, line)
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= width {
				currentLine += " " + word
			} else {
				result = append(result, currentLine)
				currentLine = word
			}
		}
		if currentLine != "" {
			result = append(result, currentLine)
		}
	}

	return strings.Join(result, "\n")
}

// RenderConversationWithArtifacts renders the conversation with inline artifacts
func RenderConversationWithArtifacts(conversation *models.Conversation, messages []*models.Message, messageArtifacts map[int64][]*artifacts.Artifact, width int, focusedOnArtifact bool, messageIndex int, artifactIndex int, expandedArtifacts map[string]bool) string {
	var sb strings.Builder
	renderer := artifacts.NewTerminalRenderer()

	// Header
	sb.WriteString(HeaderStyle.Render(fmt.Sprintf("Conversation: %s", conversation.Name)))
	sb.WriteString("\n")
	sb.WriteString(DateStyle.Render(fmt.Sprintf("Messages: %d | Updated: %s",
		len(messages),
		conversation.UpdatedAt.Format("2006-01-02 15:04"))))

	// Add artifact count if any
	totalArtifacts := 0
	for _, arts := range messageArtifacts {
		totalArtifacts += len(arts)
	}
	if totalArtifacts > 0 {
		sb.WriteString(" | ")
		sb.WriteString(DateStyle.Render(fmt.Sprintf("Artifacts: %d", totalArtifacts)))
	}

	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n\n")

	// Messages
	for i, msg := range messages {
		// Message header
		displaySender := rendering.FormatSender(msg.Sender)
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")

		if msg.Sender == "human" {
			sb.WriteString(ConversationStyle.Bold(true).Render(fmt.Sprintf("%s (%s)", displaySender, timestamp)))
		} else {
			sb.WriteString(AssistantStyle.Render(fmt.Sprintf("%s (%s)", displaySender, timestamp)))
		}
		sb.WriteString("\n")

		// Message text with artifacts removed
		text := strings.TrimSpace(msg.Text)
		if messageArtifacts[msg.ID] != nil && len(messageArtifacts[msg.ID]) > 0 {
			// Remove artifact tags from display
			extractor := artifacts.NewExtractor()
			text = extractor.ArtifactRegex.ReplaceAllString(text, "[Artifact: see below]")
		}

		// Word wrap the cleaned text
		wrappedText := simpleWordWrap(text, width-4)
		sb.WriteString(wrappedText)

		// Render artifacts inline if present
		if arts := messageArtifacts[msg.ID]; len(arts) > 0 {
			sb.WriteString("\n\n")

			for j, artifact := range arts {
				// Check if this artifact is currently focused
				isFocused := focusedOnArtifact && i == messageIndex && j == artifactIndex

				// Check if this artifact is expanded (default to false = show preview)
				// false = show maxHeight lines, true = show all lines
				isExpanded := false
				if expandedArtifacts != nil {
					if expanded, exists := expandedArtifacts[artifact.ID]; exists {
						isExpanded = expanded
					}
				}

				// Render artifact inline with limited height
				maxHeight := 10
				artifactRender := renderer.RenderInline(artifact, isFocused, isExpanded, maxHeight)

				// Indent the artifact
				lines := strings.Split(artifactRender, "\n")
				for _, line := range lines {
					sb.WriteString("  ")
					sb.WriteString(line)
					sb.WriteString("\n")
				}

				if j < len(arts)-1 {
					sb.WriteString("\n")
				}
			}
		}

		if i < len(messages)-1 {
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("─", width/2))
			sb.WriteString("\n\n")
		}
	}

	// Help text at bottom
	if focusedOnArtifact {
		sb.WriteString("\n\n")
		sb.WriteString(HelpStyle.Render("[Tab] unfocus | [s] save | [←/→] navigate artifacts | [q] back"))
	} else if totalArtifacts > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(HelpStyle.Render("[Tab] focus artifact | [/] find | [q] back"))
	}

	return sb.String()
}
