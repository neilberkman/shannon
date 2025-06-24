package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/search"
	"golang.org/x/term"
)

// Key constants
const (
	keyEnter  = "enter"
	keyEsc    = "esc"
	keySlash  = "/"
	keyO      = "o"
	keyQ      = "q"
	keyG      = "g"
	keyShiftG = "G"
	keyN      = "n"
	keyShiftN = "N"
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
	engine        *search.Engine
	conversations []*models.Conversation
	list          list.Model
	textInput     textinput.Model
	mode          Mode
	searching     bool
	width         int
	height        int

	// Conversation view handles all conversation display and interaction
	convView conversationView
}

// newBrowseModel creates a new browse model
func newBrowseModel(engine *search.Engine) browseModel {
	// Get all conversations
	conversations, _ := engine.GetAllConversations(10000, 0)

	// Convert to list items
	items := make([]list.Item, len(conversations))
	for i, c := range conversations {
		items[i] = conversationItem{conv: c}
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

	l := list.New(items, delegate, width, height-5) // Leave room for search input
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
		mode:          ModeList,
		width:         width,
		height:        height,
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

		// Update conversation view if active
		if m.mode == ModeConversation {
			cv, _ := m.convView.Update(msg)
			m.convView = cv
		}

	case tea.KeyMsg:
		switch m.mode {
		case ModeList:
			// Check if the list is filtering before handling keys
			if m.list.FilterState() == list.Filtering {
				// Let the list handle filtering
				list, cmd := m.list.Update(msg)
				m.list = list
				cmds = append(cmds, cmd)
			} else if m.searching {
				switch msg.String() {
				case keyEnter:
					// Perform search
					query := m.textInput.Value()
					if query != "" {
						opts := search.SearchOptions{
							Query:     query,
							Limit:     1000,
							SortBy:    "relevance",
							SortOrder: "desc",
						}
						results, err := m.engine.Search(opts)
						if err != nil {
							// Log search error for debugging
							fmt.Printf("Search error for query '%s': %v\n", query, err)
							// Stay in search mode but clear input
							m.textInput.SetValue("")
						} else {
							// Switch to search results view
							return newSearchModel(m.engine, results, query), nil
						}
					}
					m.searching = false
					m.textInput.Blur()
				case keyEsc:
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
				case "q":
					return m, tea.Quit
				case "/":
					m.searching = true
					m.textInput.Focus()
					cmds = append(cmds, textinput.Blink)
				case keyEnter:
					if i, ok := m.list.SelectedItem().(conversationItem); ok {
						conv, messages, err := m.engine.GetConversation(i.conv.ID)
						if err != nil {
							// Log error for debugging - this will go to debug.log
							fmt.Printf("Error loading conversation %d: %v\n", i.conv.ID, err)
							// Could also show a temporary error message in the UI
						} else {
							// Create new conversation view
							m.convView = newConversationView(conv, messages, m.width, m.height)
							m.mode = ModeConversation
						}
					}
				case "o":
					// Open conversation in claude.ai
					if i, ok := m.list.SelectedItem().(conversationItem); ok {
						url := fmt.Sprintf("https://claude.ai/chat/%s", i.conv.UUID)
						openURL(url)
					}
				case "g":
					// Jump to beginning
					m.list.Select(0)
				case "G":
					// Jump to end
					m.list.Select(len(m.conversations) - 1)
				case "home":
					// Jump to beginning
					m.list.Select(0)
				case "end":
					// Jump to end
					m.list.Select(len(m.conversations) - 1)
				case "pgup":
					// Page up
					current := m.list.Index()
					pageSize := m.height - 5
					newIndex := current - pageSize
					if newIndex < 0 {
						newIndex = 0
					}
					m.list.Select(newIndex)
				case "pgdown":
					// Page down
					current := m.list.Index()
					pageSize := m.height - 5
					newIndex := current + pageSize
					if newIndex >= len(m.conversations) {
						newIndex = len(m.conversations) - 1
					}
					m.list.Select(newIndex)
				// Removed custom 'down'/'j' and 'up'/'k' handlers
				// Let the default list navigation handle these keys for single-item movement.
				default:
					list, cmd := m.list.Update(msg)
					m.list = list
					cmds = append(cmds, cmd)
				}
			}

		case ModeConversation:
			// Store the previous artifact focus state
			wasInArtifactMode := m.convView.focusedOnArtifact
			
			// Delegate all conversation handling to convView
			cv, cmd := m.convView.Update(msg)
			m.convView = cv
			cmds = append(cmds, cmd)

			// Check for keys that should exit conversation mode
			switch msg.String() {
			case "q":
				m.mode = ModeList
				return m, nil
			case "esc":
				// If we were in artifact mode and now we're not, the conversation view handled it
				if wasInArtifactMode && !m.convView.focusedOnArtifact {
					// Don't exit conversation mode - just return
					return m, tea.Batch(cmds...)
				}
				// Only exit if not in find mode and not in artifact focus mode
				if !m.convView.findActive && !m.convView.focusedOnArtifact {
					m.mode = ModeList
					return m, nil
				}
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
			searchBar = TitleStyle.Render("Search: ") + m.textInput.View() + "\n"
		} else {
			searchBar = HelpStyle.Render("Press / to search") + "\n"
		}

		// List
		content := m.list.View()

		// Help
		help := HelpStyle.Render("↑/↓/j/k: navigate • g/G: top/bottom • PgUp/PgDn: page • enter: view • o: open in claude.ai • /: search • q: quit")

		return searchBar + content + "\n" + help

	case ModeConversation:
		// Delegate to conversation view
		return m.convView.View()
	}

	return ""
}

// The following methods have been moved to conversationView:
// - findInConversation
// - extractArtifacts
// - getCurrentMessageWithArtifact
// - saveCurrentArtifact
// - sanitizeFilename
