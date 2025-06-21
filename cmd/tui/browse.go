package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/shannon/internal/models"
	"github.com/user/shannon/internal/search"
)

// conversationItem implements list.Item for conversations
type conversationItem struct {
	conv *models.Conversation
}

func (i conversationItem) Title() string {
	return i.conv.Name
}

func (i conversationItem) Description() string {
	return fmt.Sprintf("%d messages • Updated %s", 
		i.conv.MessageCount, 
		i.conv.UpdatedAt.Format("2006-01-02"))
}

func (i conversationItem) FilterValue() string {
	return i.conv.Name
}

// browseModel is the model for browsing conversations
type browseModel struct {
	engine       *search.Engine
	conversations []*models.Conversation
	list         list.Model
	textInput    textinput.Model
	viewport     viewport.Model
	mode         Mode
	searching    bool
	width        int
	height       int
	conversation *models.Conversation
	messages     []*models.Message
}

// newBrowseModel creates a new browse model
func newBrowseModel(engine *search.Engine) browseModel {
	// Get all conversations
	conversations, _ := engine.GetAllConversations(100, 0)
	
	// Convert to list items
	items := make([]list.Item, len(conversations))
	for i, c := range conversations {
		items[i] = conversationItem{conv: c}
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
	l.Title = "Browse Conversations"
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	// Create text input for search
	ti := textinput.New()
	ti.Placeholder = "Search conversations..."
	ti.CharLimit = 100
	ti.Width = 50

	return browseModel{
		engine:        engine,
		conversations: conversations,
		list:          l,
		textInput:     ti,
		viewport:      viewport.New(0, 0),
		mode:          ModeList,
	}
}

// Init initializes the model
func (m browseModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m browseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-5) // Leave room for search
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3

	case tea.KeyMsg:
		switch m.mode {
		case ModeList:
			if m.searching {
				switch msg.String() {
				case "enter":
					// Perform search
					query := m.textInput.Value()
					if query != "" {
						opts := search.SearchOptions{
							Query:     query,
							Limit:     100,
							SortBy:    "relevance",
							SortOrder: "desc",
						}
						results, err := m.engine.Search(opts)
						if err == nil {
							// Switch to search results view
							return newSearchModel(m.engine, results, query), nil
						}
					}
					m.searching = false
					m.textInput.Blur()
				case "esc":
					m.searching = false
					m.textInput.SetValue("")
					m.textInput.Blur()
				default:
					ti, cmd := m.textInput.Update(msg)
					m.textInput = ti
					cmds = append(cmds, cmd)
				}
			} else {
				switch msg.String() {
				case "q", "ctrl+c":
					return m, tea.Quit
				case "/":
					m.searching = true
					m.textInput.Focus()
					cmds = append(cmds, textinput.Blink)
				case "enter":
					if i, ok := m.list.SelectedItem().(conversationItem); ok {
						conv, messages, err := m.engine.GetConversation(i.conv.ID)
						if err == nil {
							m.conversation = conv
							m.messages = messages
							m.mode = ModeConversation
							m.viewport.SetContent(m.renderConversation())
							m.viewport.GotoTop()
						}
					}
				default:
					list, cmd := m.list.Update(msg)
					m.list = list
					cmds = append(cmds, cmd)
				}
			}

		case ModeConversation:
			switch msg.String() {
			case "q", "esc":
				m.mode = ModeList
			default:
				vp, cmd := m.viewport.Update(msg)
				m.viewport = vp
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the view
func (m browseModel) View() string {
	switch m.mode {
	case ModeList:
		// Search bar
		searchBar := ""
		if m.searching {
			searchBar = titleStyle.Render("Search: ") + m.textInput.View() + "\n"
		} else {
			searchBar = helpStyle.Render("Press / to search") + "\n"
		}

		// List
		content := m.list.View()

		// Help
		help := helpStyle.Render("↑/↓: navigate • enter: view • /: search • q: quit")

		return searchBar + content + "\n" + help

	case ModeConversation:
		content := m.viewport.View()
		help := helpStyle.Render("↑/↓: scroll • esc: back • q: quit")
		return content + "\n" + help
	}

	return ""
}

// renderConversation renders the full conversation view
func (m browseModel) renderConversation() string {
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