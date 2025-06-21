package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/shannon/internal/models"
	"github.com/user/shannon/internal/search"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingLeft(2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			PaddingLeft(2)

	conversationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#04B575"))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	snippetStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(4)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#FAFAFA")).
			Padding(0, 1)
)

// searchItem implements list.Item for search results
type searchItem struct {
	result *models.SearchResult
}

func (i searchItem) Title() string {
	return fmt.Sprintf("%s (%s)", i.result.ConversationName, i.result.Sender)
}

func (i searchItem) Description() string {
	snippet := strings.ReplaceAll(i.result.Snippet, "\n", " ")
	if len(snippet) > 80 {
		snippet = snippet[:77] + "..."
	}
	return snippet
}

func (i searchItem) FilterValue() string {
	return i.result.ConversationName + " " + i.result.Snippet
}

// Mode represents the current view mode
type Mode int

const (
	ModeList Mode = iota
	ModeDetail
	ModeConversation
)

// searchModel is the main model for search TUI
type searchModel struct {
	engine      *search.Engine
	results     []*models.SearchResult
	list        list.Model
	viewport    viewport.Model
	mode        Mode
	selected    int
	width       int
	height      int
	query       string
	conversation *models.Conversation
	messages     []*models.Message
}

// newSearchModel creates a new search model
func newSearchModel(engine *search.Engine, results []*models.SearchResult, query string) searchModel {
	// Convert results to list items
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = searchItem{result: r}
	}

	// Create list
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4"))

	l := list.New(items, delegate, 0, 0)
	l.Title = fmt.Sprintf("Search Results for: %s", query)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	return searchModel{
		engine:   engine,
		results:  results,
		list:     l,
		viewport: viewport.New(0, 0),
		mode:     ModeList,
		query:    query,
	}
}

// Init initializes the model
func (m searchModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-3)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3

	case tea.KeyMsg:
		switch m.mode {
		case ModeList:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "enter":
				if i, ok := m.list.SelectedItem().(searchItem); ok {
					m.selected = m.list.Index()
					m.mode = ModeDetail
					m.viewport.SetContent(m.renderDetail(i.result))
					m.viewport.GotoTop()
				}
			case "v":
				// View full conversation
				if i, ok := m.list.SelectedItem().(searchItem); ok {
					conv, messages, err := m.engine.GetConversation(i.result.ConversationID)
					if err == nil {
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.viewport.SetContent(m.renderConversation())
						m.viewport.GotoTop()
					}
				}
			}

		case ModeDetail, ModeConversation:
			switch msg.String() {
			case "q", "esc":
				m.mode = ModeList
			case "v":
				if m.mode == ModeDetail && m.selected < len(m.results) {
					// Switch to conversation view
					result := m.results[m.selected]
					conv, messages, err := m.engine.GetConversation(result.ConversationID)
					if err == nil {
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.viewport.SetContent(m.renderConversation())
						m.viewport.GotoTop()
					}
				}
			}
		}
	}

	// Update components
	var cmd tea.Cmd
	switch m.mode {
	case ModeList:
		m.list, cmd = m.list.Update(msg)
	case ModeDetail, ModeConversation:
		m.viewport, cmd = m.viewport.Update(msg)
	}

	return m, cmd
}

// View renders the view
func (m searchModel) View() string {
	var content string

	switch m.mode {
	case ModeList:
		content = m.list.View()
	case ModeDetail, ModeConversation:
		content = m.viewport.View()
	}

	// Add help text
	var help string
	switch m.mode {
	case ModeList:
		help = helpStyle.Render("↑/↓: navigate • enter: view details • v: view conversation • q: quit")
	case ModeDetail:
		help = helpStyle.Render("↑/↓: scroll • v: view full conversation • esc: back • q: quit")
	case ModeConversation:
		help = helpStyle.Render("↑/↓: scroll • esc: back • q: quit")
	}

	return content + "\n" + help
}

// renderDetail renders the detail view for a search result
func (m searchModel) renderDetail(result *models.SearchResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString(headerStyle.Render("Search Result Details"))
	sb.WriteString("\n\n")

	// Conversation info
	sb.WriteString(conversationStyle.Bold(true).Render("Conversation: "))
	sb.WriteString(fmt.Sprintf("%s (ID: %d)\n", result.ConversationName, result.ConversationID))
	
	// Message info
	sb.WriteString(conversationStyle.Bold(true).Render("Sender: "))
	sb.WriteString(fmt.Sprintf("%s\n", strings.Title(result.Sender)))
	
	sb.WriteString(conversationStyle.Bold(true).Render("Date: "))
	sb.WriteString(dateStyle.Render(result.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n\n")

	// Full message with context
	sb.WriteString(conversationStyle.Bold(true).Render("Message Context:"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", m.width))
	sb.WriteString("\n")

	// Get context messages
	messages, err := m.getMessageContext(result, 3)
	if err == nil {
		for _, msg := range messages {
			if msg.UUID == result.MessageUUID {
				// Highlight the found message
				sb.WriteString(selectedStyle.Render(fmt.Sprintf("[%s] %s", 
					msg.CreatedAt.Format("15:04"),
					strings.Title(msg.Sender))))
				sb.WriteString("\n")
				text := strings.TrimSpace(msg.Text)
				if len(text) > 500 {
					text = text[:497] + "..."
				}
				sb.WriteString(selectedStyle.Render(text))
			} else {
				// Regular message
				sb.WriteString(fmt.Sprintf("[%s] %s\n", 
					msg.CreatedAt.Format("15:04"),
					strings.Title(msg.Sender)))
				text := strings.TrimSpace(msg.Text)
				if len(text) > 200 {
					text = text[:197] + "..."
				}
				sb.WriteString(snippetStyle.Render(text))
			}
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// renderConversation renders the full conversation view
func (m searchModel) renderConversation() string {
	var sb strings.Builder

	// Header
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Conversation: %s", m.conversation.Name)))
	sb.WriteString("\n")
	sb.WriteString(dateStyle.Render(fmt.Sprintf("Messages: %d | Updated: %s", 
		len(m.messages), 
		m.conversation.UpdatedAt.Format("2006-01-02 15:04"))))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", m.width))
	sb.WriteString("\n\n")

	// Messages
	for i, msg := range m.messages {
		// Message header
		sender := strings.Title(msg.Sender)
		timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
		
		if msg.Sender == "human" {
			sb.WriteString(conversationStyle.Bold(true).Render(fmt.Sprintf("%s (%s)", sender, timestamp)))
		} else {
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#04B5FF")).
				Render(fmt.Sprintf("%s (%s)", sender, timestamp)))
		}
		sb.WriteString("\n")
		
		// Message text
		text := strings.TrimSpace(msg.Text)
		sb.WriteString(snippetStyle.Render(text))
		
		if i < len(m.messages)-1 {
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("─", m.width/2))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// getMessageContext retrieves messages around the found message
func (m searchModel) getMessageContext(result *models.SearchResult, contextLines int) ([]*models.Message, error) {
	// Get all messages for the conversation
	_, messages, err := m.engine.GetConversation(result.ConversationID)
	if err != nil {
		return nil, err
	}

	// Find the target message index
	targetIdx := -1
	for i, msg := range messages {
		if msg.UUID == result.MessageUUID {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return nil, fmt.Errorf("message not found")
	}

	// Calculate range
	start := targetIdx - contextLines
	if start < 0 {
		start = 0
	}
	end := targetIdx + contextLines + 1
	if end > len(messages) {
		end = len(messages)
	}

	return messages[start:end], nil
}