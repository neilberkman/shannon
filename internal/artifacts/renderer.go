package artifacts

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Renderer interface for different output formats
type Renderer interface {
	RenderList(artifacts []*Artifact) string
	RenderDetail(artifact *Artifact) string
	RenderInline(artifact *Artifact, focused bool, expanded bool, maxHeight int) string
}

// TerminalRenderer renders artifacts for terminal display
type TerminalRenderer struct {
	artifactStyle lipgloss.Style
	focusedStyle  lipgloss.Style
	titleStyle    lipgloss.Style
	languageStyle lipgloss.Style
	previewStyle  lipgloss.Style
}

// NewTerminalRenderer creates a new terminal renderer with styles
func NewTerminalRenderer() *TerminalRenderer {
	return &TerminalRenderer{
		artifactStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
		focusedStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1),
		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")),
		languageStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")),
		previewStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")),
	}
}

// RenderList renders a list of artifacts
func (r *TerminalRenderer) RenderList(artifacts []*Artifact) string {
	if len(artifacts) == 0 {
		return "No artifacts found"
	}

	var lines []string
	for i, artifact := range artifacts {
		icon := getArtifactIcon(artifact.Type)
		typeName := artifact.GetTypeName()

		line := fmt.Sprintf("[%d] %s %s - %s",
			i+1,
			icon,
			r.titleStyle.Render(artifact.Title),
			r.languageStyle.Render(typeName))

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// RenderDetail renders full artifact content
func (r *TerminalRenderer) RenderDetail(artifact *Artifact) string {
	icon := getArtifactIcon(artifact.Type)
	header := fmt.Sprintf("%s %s", icon, r.titleStyle.Render(artifact.Title))

	if artifact.Language != "" {
		header += " " + r.languageStyle.Render(fmt.Sprintf("(%s)", artifact.Language))
	}

	content := r.artifactStyle.Render(artifact.Content)

	return fmt.Sprintf("%s\n%s", header, content)
}

// RenderInline renders an artifact inline within a conversation view
func (r *TerminalRenderer) RenderInline(artifact *Artifact, focused bool, expanded bool, maxHeight int) string {
	icon := getArtifactIcon(artifact.Type)

	// Base header content
	headerContent := fmt.Sprintf(" %s %s ", icon, artifact.Title)
	if artifact.Language != "" {
		headerContent += fmt.Sprintf("(%s) ", artifact.Language)
	}

	// Get content lines
	lines := strings.Split(artifact.Content, "\n")

	// Find the maximum line width for proper box formatting
	maxWidth := 50
	for _, line := range lines {
		if len(line)+4 > maxWidth { // +4 for "│ " and " │"
			maxWidth = min(len(line)+4, 100) // Cap at 100 chars total
		}
	}

	// Adjust width to accommodate header controls if focused
	if focused {
		minHeaderWidth := len(headerContent) + len(" [Tab] collapse • [Esc] exit ") + 4
		if minHeaderWidth > maxWidth {
			maxWidth = minHeaderWidth
		}
	}

	// Build header with proper width
	header := "┌─" + headerContent
	if focused {
		actions := " [Tab] collapse • [Esc] exit "
		padding := max(0, maxWidth-len(headerContent)-len(actions)-4)
		header += strings.Repeat("─", padding) + actions + "─┐"
	} else {
		padding := max(0, maxWidth-len(headerContent)-4)
		header += strings.Repeat("─", padding) + "─┐"
	}

	// Build content lines
	var contentLines []string
	innerWidth := maxWidth - 4 // Account for "│ " and " │"

	// Determine how many lines to show
	linesToShow := len(lines)
	if !expanded && len(lines) > maxHeight {
		// Show preview (maxHeight lines) when collapsed
		linesToShow = maxHeight
	}

	for i := 0; i < linesToShow; i++ {
		displayLine := lines[i]
		if len(displayLine) > innerWidth {
			displayLine = displayLine[:innerWidth-3] + "..."
		}
		contentLines = append(contentLines, fmt.Sprintf("│ %s │", padRight(displayLine, innerWidth)))
	}

	// Build footer
	footer := "└"

	// Show "more lines" info if collapsed and there are more lines
	if !expanded && len(lines) > maxHeight {
		moreInfo := fmt.Sprintf("─ ... (%d more lines) ", len(lines)-maxHeight)
		if focused {
			saveText := " [s] save "
			padding := max(0, maxWidth-len(moreInfo)-len(saveText)-2)
			footer += moreInfo + strings.Repeat("─", padding) + saveText
		} else {
			padding := max(0, maxWidth-len(moreInfo)-2)
			footer += moreInfo + strings.Repeat("─", padding)
		}
	} else {
		// Expanded or short artifact
		if focused {
			saveText := " [s] save "
			if expanded && len(lines) > 20 {
				lineInfo := fmt.Sprintf("─ (%d lines total) ", len(lines))
				padding := max(0, maxWidth-len(lineInfo)-len(saveText)-2)
				footer += lineInfo + strings.Repeat("─", padding) + saveText
			} else {
				padding := max(0, maxWidth-len(saveText)-2)
				footer += strings.Repeat("─", padding) + saveText
			}
		} else {
			if expanded && len(lines) > 20 {
				lineInfo := fmt.Sprintf("─ (%d lines total) ", len(lines))
				padding := max(0, maxWidth-len(lineInfo)-2)
				footer += lineInfo + strings.Repeat("─", padding)
			} else {
				footer += strings.Repeat("─", maxWidth-2)
			}
		}
	}
	footer += "─┘"

	// Apply style
	style := r.artifactStyle
	if focused {
		style = r.focusedStyle
	}

	result := header + "\n" + strings.Join(contentLines, "\n") + "\n" + footer
	return style.Render(result)
}

// MarkdownRenderer renders artifacts as markdown
type MarkdownRenderer struct{}

// NewMarkdownRenderer creates a new markdown renderer
func NewMarkdownRenderer() *MarkdownRenderer {
	return &MarkdownRenderer{}
}

// RenderList renders a list of artifacts as markdown
func (r *MarkdownRenderer) RenderList(artifacts []*Artifact) string {
	if len(artifacts) == 0 {
		return "*No artifacts found*"
	}

	var lines []string
	lines = append(lines, "## Artifacts\n")

	for i, artifact := range artifacts {
		icon := getArtifactIcon(artifact.Type)
		typeName := artifact.GetTypeName()

		line := fmt.Sprintf("%d. %s **%s** - %s",
			i+1,
			icon,
			artifact.Title,
			typeName)

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// RenderDetail renders full artifact content as markdown
func (r *MarkdownRenderer) RenderDetail(artifact *Artifact) string {
	icon := getArtifactIcon(artifact.Type)
	header := fmt.Sprintf("## %s %s\n", icon, artifact.Title)

	if artifact.Language != "" {
		header += fmt.Sprintf("**Language:** %s\n", artifact.Language)
	}

	// Wrap content in code block for code artifacts
	content := artifact.Content
	if artifact.Type == TypeCode && artifact.Language != "" {
		content = fmt.Sprintf("```%s\n%s\n```", artifact.Language, content)
	} else if artifact.Type == TypeCode {
		content = fmt.Sprintf("```\n%s\n```", content)
	}

	return header + "\n" + content
}

// RenderInline renders an artifact inline (same as detail for markdown)
func (r *MarkdownRenderer) RenderInline(artifact *Artifact, focused bool, expanded bool, maxHeight int) string {
	return r.RenderDetail(artifact)
}

// Helper functions

func getArtifactIcon(artifactType string) string {
	switch artifactType {
	case TypeCode:
		return "📄"
	case TypeMarkdown:
		return "📝"
	case TypeHTML:
		return "🌐"
	case TypeSVG:
		return "🎨"
	case TypeReact:
		return "⚛️"
	case TypeMermaid:
		return "📊"
	default:
		return "📋"
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
