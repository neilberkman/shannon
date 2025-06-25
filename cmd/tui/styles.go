package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Shared TUI styles
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingLeft(2)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			PaddingLeft(2)

	ConversationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#04B575"))

	DateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	SnippetStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(4)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#FAFAFA")).
			Padding(0, 1)

	AssistantStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B5FF"))

	NotificationStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#04B575")).
				Foreground(lipgloss.Color("#FAFAFA")).
				Padding(0, 1).
				Bold(true)

	// Find highlight style that respects terminal themes
	FindHighlightStyle = lipgloss.NewStyle().
				Reverse(true).
				Bold(true)
)

// sanitizeFilename makes a filename safe for the filesystem
func sanitizeFilename(name string) string {
	// Replace problematic characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "_",
	)
	return replacer.Replace(name)
}

// highlightMatches highlights all occurrences of query in the content
func highlightMatches(content, query string) string {
	if query == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(query)

	for i, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, queryLower) {
			// Find all occurrences in the line
			result := ""
			lastEnd := 0

			for {
				idx := strings.Index(strings.ToLower(line[lastEnd:]), queryLower)
				if idx == -1 {
					result += line[lastEnd:]
					break
				}

				// Add text before match
				result += line[lastEnd : lastEnd+idx]

				// Add highlighted match (preserve original case)
				matchEnd := lastEnd + idx + len(query)
				if matchEnd > len(line) {
					matchEnd = len(line)
				}
				matchText := line[lastEnd+idx : matchEnd]
				result += FindHighlightStyle.Render(matchText)

				lastEnd += idx + len(query)
			}

			lines[i] = result
		}
	}

	return strings.Join(lines, "\n")
}

// formatConversationDates formats the date range for a conversation
// Shows single date if created and updated on same day, otherwise shows range
func formatConversationDates(createdAt, updatedAt time.Time) string {
	startDate := createdAt.Format("Jan 2, 2006")
	endDate := updatedAt.Format("Jan 2, 2006")

	if startDate == endDate {
		return startDate
	}
	return fmt.Sprintf("%s - %s", startDate, endDate)
}
