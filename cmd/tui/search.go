package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/rendering"
	"github.com/neilberkman/shannon/internal/search"
	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Remove duplicated styles - now using shared styles from styles.go

// searchItem implements list.Item for search results
type searchItem struct {
	result *models.SearchResult
}

func (i searchItem) Title() string {
	return fmt.Sprintf("%s (%s)", i.result.ConversationName, i.result.Sender)
}

func (i searchItem) Description() string {
	// Try to render snippet with markdown formatting
	renderer, err := rendering.NewMarkdownRenderer(80)
	if err != nil {
		// Fallback to plain text
		return i.getPlainSnippet()
	}

	rendered, err := renderer.RenderMessage(i.result.Snippet, i.result.Sender, true)
	if err != nil {
		// Fallback to plain text
		return i.getPlainSnippet()
	}

	// Clean up the rendered text for list display
	snippet := strings.ReplaceAll(rendered, "\n", " ")
	snippet = strings.TrimSpace(snippet)

	// Truncate if too long
	if len(snippet) > 80 {
		snippet = snippet[:77] + "..."
	}

	return snippet
}

func (i searchItem) getPlainSnippet() string {
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
	engine       *search.Engine
	results      []*models.SearchResult
	list         list.Model
	viewport     viewport.Model
	mode         Mode
	selected     int
	width        int
	height       int
	query        string
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
	delegate.Styles.SelectedTitle = SelectedStyle
	delegate.Styles.SelectedDesc = SelectedStyle

	// Get actual terminal size
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 || height == 0 {
		// Fallback to reasonable defaults if terminal size detection fails
		width, height = 80, 24
	}

	l := list.New(items, delegate, width, height-3)
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
			case "q":
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
					if err != nil {
						// Log error for debugging
						fmt.Printf("Error loading conversation %d: %v\n", i.result.ConversationID, err)
					} else {
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.viewport.SetContent(RenderConversation(conv, messages, m.width))
						m.viewport.GotoTop()
					}
				}
			default:
				// Forward navigation keys to the list
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				return m, cmd
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
					if err != nil {
						// Log error for debugging
						fmt.Printf("Error loading conversation %d: %v\n", result.ConversationID, err)
					} else {
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.viewport.SetContent(RenderConversation(conv, messages, m.width))
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
		help = HelpStyle.Render("↑/↓: navigate • enter: view details • v: view conversation • q: quit")
	case ModeDetail:
		help = HelpStyle.Render("↑/↓: scroll • v: view full conversation • esc: back • q: quit")
	case ModeConversation:
		help = HelpStyle.Render("↑/↓: scroll • esc: back • q: quit")
	}

	return content + "\n" + help
}

// renderDetail renders the detail view for a search result
func (m searchModel) renderDetail(result *models.SearchResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString(HeaderStyle.Render("Search Result Details"))
	sb.WriteString("\n\n")

	// Conversation info
	sb.WriteString(ConversationStyle.Bold(true).Render("Conversation: "))
	sb.WriteString(fmt.Sprintf("%s (ID: %d)\n", result.ConversationName, result.ConversationID))

	// Message info
	sb.WriteString(ConversationStyle.Bold(true).Render("Sender: "))
	caser := cases.Title(language.English)
	sb.WriteString(fmt.Sprintf("%s\n", caser.String(result.Sender)))

	sb.WriteString(ConversationStyle.Bold(true).Render("Date: "))
	sb.WriteString(DateStyle.Render(result.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n\n")

	// Full message with context
	sb.WriteString(ConversationStyle.Bold(true).Render("Message Context:"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", m.width))
	sb.WriteString("\n")

	// Get context messages
	messages, err := m.getMessageContext(result, 3)
	if err == nil {
		for _, msg := range messages {
			if msg.UUID == result.MessageUUID {
				// Highlight the found message
				caser := cases.Title(language.English)
				sb.WriteString(SelectedStyle.Render(fmt.Sprintf("[%s] %s",
					msg.CreatedAt.Format("15:04"),
					caser.String(msg.Sender))))
				sb.WriteString("\n")
				text := strings.TrimSpace(msg.Text)
				if len(text) > 500 {
					text = text[:497] + "..."
				}
				sb.WriteString(SelectedStyle.Render(text))
			} else {
				// Regular message
				caser := cases.Title(language.English)
				sb.WriteString(fmt.Sprintf("[%s] %s\n",
					msg.CreatedAt.Format("15:04"),
					caser.String(msg.Sender)))
				text := strings.TrimSpace(msg.Text)
				if len(text) > 200 {
					text = text[:197] + "..."
				}
				sb.WriteString(SnippetStyle.Render(text))
			}
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
