package tui

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/rendering"
)

// RenderConversation renders the full conversation view with markdown support
// This is shared between browse and search models
func RenderConversation(conversation *models.Conversation, messages []*models.Message, width int) string {
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

	// Create markdown renderer
	renderer, err := rendering.NewMarkdownRenderer(width - 4) // Account for padding
	if err != nil {
		// Fallback to plain rendering if markdown renderer fails
		return renderConversationPlain(conversation, messages, width)
	}

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
		sb.WriteString("\n\n")
		
		// Render message text with markdown
		renderedText, err := renderer.RenderMessage(msg.Text, msg.Sender, false)
		if err != nil {
			// Fallback to plain text if rendering fails
			renderedText = strings.TrimSpace(msg.Text)
		}
		
		sb.WriteString(renderedText)
		
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("─", width/2))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
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
		
		// Message text
		text := strings.TrimSpace(msg.Text)
		sb.WriteString(SnippetStyle.Render(text))
		
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("─", width/2))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}