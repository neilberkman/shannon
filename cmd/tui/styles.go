package tui

import "github.com/charmbracelet/lipgloss"

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
)
