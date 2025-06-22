package rendering

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// MarkdownRenderer handles markdown formatting for different contexts
type MarkdownRenderer struct {
	termRenderer *glamour.TermRenderer
	width        int
}

var (
	sharedRenderer     *MarkdownRenderer
	sharedRendererOnce sync.Once
)

// GetSharedRenderer returns a singleton markdown renderer
func GetSharedRenderer() *MarkdownRenderer {
	sharedRendererOnce.Do(func() {
		// Create renderer with a fixed dark theme - no auto detection
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(76), // Fixed width for consistency
		)
		if err != nil {
			// If glamour fails, create a minimal renderer
			sharedRenderer = &MarkdownRenderer{
				termRenderer: nil,
				width:        80,
			}
			return
		}

		sharedRenderer = &MarkdownRenderer{
			termRenderer: r,
			width:        80,
		}
	})
	return sharedRenderer
}

// NewMarkdownRenderer creates a new markdown renderer (legacy function)
func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	// Use shared renderer for performance
	return GetSharedRenderer(), nil
}

// RenderMessage renders a message with markdown formatting
func (mr *MarkdownRenderer) RenderMessage(text string, sender string, isSnippet bool) (string, error) {
	// If glamour creation failed, use plain text
	if mr.termRenderer == nil {
		return mr.formatPlainText(text), nil
	}

	// For snippets, we want to preserve the search highlighting
	if isSnippet {
		return mr.renderSnippet(text, sender)
	}

	// For full messages, render with full markdown support
	return mr.renderFullMessage(text, sender)
}

// renderSnippet handles search result snippets with highlighting
func (mr *MarkdownRenderer) renderSnippet(text string, sender string) (string, error) {
	// For snippets, we want to be more conservative with markdown rendering
	// to preserve search highlighting markup (<mark>...</mark>)

	// First, protect the search highlighting
	text = strings.ReplaceAll(text, "<mark>", "___MARK_START___")
	text = strings.ReplaceAll(text, "</mark>", "___MARK_END___")

	// Render markdown but with limited features for snippets
	rendered, err := mr.termRenderer.Render(text)
	if err != nil {
		// If markdown rendering fails, return the original text
		rendered = text
	}

	// Restore search highlighting with proper styling
	markStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#FFD700")).
		Foreground(lipgloss.Color("#000000")).
		Bold(true)

	rendered = strings.ReplaceAll(rendered, "___MARK_START___", markStyle.Render(""))
	rendered = strings.ReplaceAll(rendered, "___MARK_END___", lipgloss.NewStyle().Render(""))

	// Apply proper search highlight styling
	parts := strings.Split(rendered, markStyle.Render(""))
	if len(parts) > 1 {
		var result strings.Builder
		for i, part := range parts {
			if i > 0 && i < len(parts) {
				// Find the text until the next end marker
				endIdx := strings.Index(part, lipgloss.NewStyle().Render(""))
				if endIdx != -1 {
					highlightedText := part[:endIdx]
					remainingText := part[endIdx+len(lipgloss.NewStyle().Render("")):]
					result.WriteString(markStyle.Render(highlightedText))
					result.WriteString(remainingText)
				} else {
					result.WriteString(part)
				}
			} else {
				result.WriteString(part)
			}
		}
		rendered = result.String()
	}

	return strings.TrimSpace(rendered), nil
}

// renderFullMessage handles full message rendering with complete markdown support
func (mr *MarkdownRenderer) renderFullMessage(text string, sender string) (string, error) {
	// First enhance text with hyperlinks if supported
	if IsHyperlinksSupported() {
		text = EnhanceTextWithLinks(text)
	}

	rendered, err := mr.termRenderer.Render(text)
	if err != nil {
		// If rendering fails, return formatted plain text
		return mr.formatPlainText(text), nil
	}

	return strings.TrimSpace(rendered), nil
}

// formatPlainText provides basic formatting for when markdown rendering fails
func (mr *MarkdownRenderer) formatPlainText(text string) string {
	// Basic formatting for plain text
	lines := strings.Split(text, "\n")
	var formatted strings.Builder

	codeBlockStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2D2D")).
		Foreground(lipgloss.Color("#E6E6E6")).
		Padding(0, 1).
		Margin(1, 0)

	inCodeBlock := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")

		// Detect code blocks
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				formatted.WriteString(codeBlockStyle.Render(""))
			} else {
				formatted.WriteString(lipgloss.NewStyle().Render(""))
			}
			continue
		}

		if inCodeBlock {
			formatted.WriteString(codeBlockStyle.Render(line))
		} else {
			// Basic inline code formatting
			if strings.Contains(line, "`") {
				line = mr.formatInlineCode(line)
			}
			formatted.WriteString(line)
		}
		formatted.WriteString("\n")
	}

	return strings.TrimSpace(formatted.String())
}

// formatInlineCode applies basic styling to inline code
func (mr *MarkdownRenderer) formatInlineCode(text string) string {
	inlineCodeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2D2D")).
		Foreground(lipgloss.Color("#E6E6E6")).
		Padding(0, 1)

	// Simple inline code detection and formatting
	parts := strings.Split(text, "`")
	if len(parts) < 3 {
		return text
	}

	var result strings.Builder
	inCode := false

	for i, part := range parts {
		if i > 0 {
			inCode = !inCode
		}

		if inCode && part != "" {
			result.WriteString(inlineCodeStyle.Render(part))
		} else {
			result.WriteString(part)
		}

		// Note: backtick separators are handled by the formatting logic above
	}

	return result.String()
}

// DetectContentType analyzes text to determine if it's likely to contain markdown
func DetectContentType(text string) ContentType {
	// Check for common markdown patterns
	markdownPatterns := []string{
		"```",  // Code blocks
		"# ",   // Headers
		"## ",  // Headers
		"### ", // Headers
		"- ",   // Lists
		"* ",   // Lists
		"1. ",  // Numbered lists
		"[",    // Links or references
		"**",   // Bold
		"__",   // Bold
		"*",    // Italic (but be careful with wildcards)
		"`",    // Inline code
		"|",    // Tables
		">",    // Blockquotes
		"---",  // Horizontal rules
	}

	markdownScore := 0
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for markdown patterns
		for _, pattern := range markdownPatterns {
			if strings.Contains(line, pattern) {
				markdownScore++
				break
			}
		}

		// Bonus points for code blocks
		if strings.HasPrefix(line, "```") {
			markdownScore += 3
		}

		// Bonus points for headers
		if strings.HasPrefix(line, "#") {
			markdownScore += 2
		}
	}

	// Determine content type based on score
	totalLines := len(lines)
	if totalLines == 0 {
		return ContentTypePlain
	}

	markdownRatio := float64(markdownScore) / float64(totalLines)

	if markdownRatio > 0.3 || markdownScore > 5 {
		return ContentTypeMarkdown
	} else if markdownScore > 0 {
		return ContentTypeMixed
	}

	return ContentTypePlain
}

// ContentType represents the type of content detected
type ContentType int

const (
	ContentTypePlain ContentType = iota
	ContentTypeMarkdown
	ContentTypeMixed
)

// String returns string representation of content type
func (ct ContentType) String() string {
	switch ct {
	case ContentTypeMarkdown:
		return "markdown"
	case ContentTypeMixed:
		return "mixed"
	default:
		return "plain"
	}
}

// RenderConversationWithMarkdown renders a full conversation with markdown support
func RenderConversationWithMarkdown(messages []MessageForRendering, width int) (string, error) {
	renderer, err := NewMarkdownRenderer(width)
	if err != nil {
		return "", err
	}

	var result strings.Builder

	for i, msg := range messages {
		// Add separator between messages
		if i > 0 {
			result.WriteString("\n" + strings.Repeat("─", width-4) + "\n\n")
		}

		// Render sender header
		senderStyle := lipgloss.NewStyle().Bold(true)
		if msg.Sender == "human" {
			senderStyle = senderStyle.Foreground(lipgloss.Color("#00D4AA"))
		} else {
			senderStyle = senderStyle.Foreground(lipgloss.Color("#7D56F4"))
		}

		result.WriteString(senderStyle.Render(strings.ToUpper(msg.Sender)))
		result.WriteString("\n\n")

		// Render message content
		rendered, err := renderer.RenderMessage(msg.Text, msg.Sender, false)
		if err != nil {
			// Fallback to plain text if rendering fails
			rendered = msg.Text
		}

		result.WriteString(rendered)
		result.WriteString("\n")
	}

	return result.String(), nil
}

// MessageForRendering represents a message for rendering purposes
type MessageForRendering struct {
	Sender string
	Text   string
}
