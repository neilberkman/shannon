package tui

import (
	"fmt"
	"strings"

	"github.com/neilberkman/shannon/internal/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
		caser := cases.Title(language.English)
		sender := caser.String(msg.Sender)
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")

		if msg.Sender == "human" {
			sb.WriteString(ConversationStyle.Bold(true).Render(fmt.Sprintf("%s (%s)", sender, timestamp)))
		} else {
			sb.WriteString(AssistantStyle.Render(fmt.Sprintf("%s (%s)", sender, timestamp)))
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
