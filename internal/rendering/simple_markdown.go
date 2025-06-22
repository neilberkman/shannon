package rendering

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SimpleMarkdownRenderer is a lightweight, fast markdown renderer
type SimpleMarkdownRenderer struct {
	width int
}

// NewSimpleMarkdownRenderer creates a new simple markdown renderer
func NewSimpleMarkdownRenderer(width int) *SimpleMarkdownRenderer {
	return &SimpleMarkdownRenderer{width: width}
}

// RenderMessage renders markdown text with basic formatting
func (r *SimpleMarkdownRenderer) RenderMessage(text string, sender string, isSnippet bool) (string, error) {
	if isSnippet {
		return r.renderSnippet(text), nil
	}
	return r.renderFull(text), nil
}

// renderSnippet renders text for list snippets (minimal formatting)
func (r *SimpleMarkdownRenderer) renderSnippet(text string) string {
	// For snippets, just clean up and return plain text
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)

	// Remove markdown syntax for clean snippet display
	text = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(text, "$1") // Bold
	text = regexp.MustCompile(`\*(.*?)\*`).ReplaceAllString(text, "$1")     // Italic
	text = regexp.MustCompile("`(.*?)`").ReplaceAllString(text, "$1")       // Code
	text = regexp.MustCompile(`#{1,6}\s+`).ReplaceAllString(text, "")       // Headers

	if len(text) > 80 {
		text = text[:77] + "..."
	}

	return text
}

// renderFull renders text with full markdown formatting
func (r *SimpleMarkdownRenderer) renderFull(text string) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder

	// Styles
	codeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2D2D")).
		Foreground(lipgloss.Color("#E6E6E6")).
		Padding(0, 1)

	codeBlockStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1E1E1E")).
		Foreground(lipgloss.Color("#E6E6E6")).
		Padding(1).
		Margin(1, 0)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))

	boldStyle := lipgloss.NewStyle().Bold(true)

	inCodeBlock := false
	var codeBlockContent strings.Builder

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")

		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				result.WriteString(codeBlockStyle.Render(codeBlockContent.String()))
				result.WriteString("\n")
				codeBlockContent.Reset()
			} else {
				// Start code block
				if codeBlockContent.Len() > 0 {
					codeBlockContent.WriteString("\n")
				}
			}
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			codeBlockContent.WriteString(line + "\n")
			continue
		}

		// Process regular lines
		processed := r.processInlineFormatting(line, codeStyle, boldStyle)

		// Handle headers
		if strings.HasPrefix(processed, "#") {
			headerLevel := 0
			for i, char := range processed {
				if char == '#' {
					headerLevel++
				} else if char == ' ' {
					processed = headerStyle.Render(processed[i+1:])
					break
				} else {
					break
				}
			}
		}

		result.WriteString(processed)
		result.WriteString("\n")
	}

	// Handle unclosed code block
	if inCodeBlock && codeBlockContent.Len() > 0 {
		result.WriteString(codeBlockStyle.Render(codeBlockContent.String()))
	}

	return strings.TrimSpace(result.String())
}

// processInlineFormatting handles inline markdown formatting
func (r *SimpleMarkdownRenderer) processInlineFormatting(text string, codeStyle, boldStyle lipgloss.Style) string {
	// Handle inline code first (to avoid conflicts)
	text = r.replaceWithStyle(text, "`([^`]+)`", codeStyle)

	// Handle bold
	text = r.replaceWithStyle(text, `\*\*([^*]+)\*\*`, boldStyle)
	text = r.replaceWithStyle(text, `__([^_]+)__`, boldStyle)

	return text
}

// replaceWithStyle replaces regex matches with styled text
func (r *SimpleMarkdownRenderer) replaceWithStyle(text string, pattern string, style lipgloss.Style) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract content between markers
		submatches := re.FindStringSubmatch(match)
		if len(submatches) > 1 {
			return style.Render(submatches[1])
		}
		return match
	})
}
