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
	RenderInline(artifact *Artifact, focused bool, maxHeight int) string
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
func (r *TerminalRenderer) RenderInline(artifact *Artifact, focused bool, maxHeight int) string {
	icon := getArtifactIcon(artifact.Type)

	// Header line
	header := fmt.Sprintf("┌─ %s %s ", icon, artifact.Title)
	if artifact.Language != "" {
		header += fmt.Sprintf("(%s) ", artifact.Language)
	}
	header += strings.Repeat("─", max(0, 50-len(header)))
	if focused {
		header += " [Tab] unfocus ─┐"
	} else {
		header += "─┐"
	}

	// Content preview
	lines := strings.Split(artifact.Content, "\n")
	var contentLines []string

	for i := 0; i < min(len(lines), maxHeight); i++ {
		line := lines[i]
		if len(line) > 48 {
			line = line[:45] + "..."
		}
		contentLines = append(contentLines, fmt.Sprintf("│ %s │", padRight(line, 48)))
	}

	// Footer
	footer := "└"
	if len(lines) > maxHeight {
		footer += fmt.Sprintf("─ ... (%d more lines) ", len(lines)-maxHeight)
	}
	footer += strings.Repeat("─", max(0, 49-len(footer)))
	if focused {
		footer += " [s] save ─┘"
	} else {
		footer += "─┘"
	}

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
func (r *MarkdownRenderer) RenderInline(artifact *Artifact, focused bool, maxHeight int) string {
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
