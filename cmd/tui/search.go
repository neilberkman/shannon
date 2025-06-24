package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/search"
	"golang.org/x/term"
)

// Remove duplicated styles - now using shared styles from styles.go

// searchConversationItem implements list.Item for search result conversations
type searchConversationItem struct {
	conv     *models.Conversation
	snippets []string // Sample snippets from matching messages
}

func (i searchConversationItem) Title() string {
	return i.conv.Name
}

func (i searchConversationItem) Description() string {
	// Show snippet from matching messages + message count
	snippet := ""
	if len(i.snippets) > 0 {
		snippet = i.snippets[0]
		// Convert <mark> tags to proper highlighting
		snippet = strings.ReplaceAll(snippet, "<mark>", "")
		snippet = strings.ReplaceAll(snippet, "</mark>", "")
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		if len(snippet) > 60 {
			snippet = snippet[:57] + "..."
		}
	}
	return fmt.Sprintf("%d messages • %s", i.conv.MessageCount, snippet)
}

func (i searchConversationItem) FilterValue() string {
	return i.conv.Name + " " + strings.Join(i.snippets, " ")
}

// Mode represents the current view mode
type Mode int

const (
	ModeList Mode = iota
	ModeConversation
)

// searchModel is the main model for search TUI
type searchModel struct {
	engine        *search.Engine
	conversations []*models.Conversation // Conversations from grouped search results
	list          list.Model
	textInput     textinput.Model
	mode          Mode
	selected      int
	width         int
	height        int
	query         string

	// Conversation view handles all conversation display and interaction
	convView conversationView
}

// newSearchModel creates a new search model
func newSearchModel(engine *search.Engine, results []*models.SearchResult, query string) searchModel {
	// Group search results by conversation
	convMap := make(map[int64]*searchConversationItem)

	for _, result := range results {
		if item, exists := convMap[result.ConversationID]; exists {
			// Add snippet to existing conversation
			item.snippets = append(item.snippets, result.Snippet)
		} else {
			// Get conversation details
			conv, _, err := engine.GetConversation(result.ConversationID)
			if err != nil {
				continue // Skip if we can't get conversation details
			}

			// Create new conversation item
			convMap[result.ConversationID] = &searchConversationItem{
				conv:     conv,
				snippets: []string{result.Snippet},
			}
		}
	}

	// Convert to list items and store conversations
	items := make([]list.Item, 0, len(convMap))
	conversations := make([]*models.Conversation, 0, len(convMap))
	for _, item := range convMap {
		// Limit snippets to avoid overwhelming display
		if len(item.snippets) > 3 {
			item.snippets = item.snippets[:3]
		}
		items = append(items, *item)
		conversations = append(conversations, item.conv)
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

	// Create text input for find
	ti := textinput.New()
	ti.Placeholder = "Find in conversation..."
	ti.CharLimit = 100
	ti.Width = 50

	return searchModel{
		engine:        engine,
		conversations: conversations,
		list:          l,
		textInput:     ti,
		mode:          ModeList,
		width:         width,
		height:        height,
		query:         query,
	}
}

// Init initializes the model
func (m searchModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var skipComponentUpdate bool
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-3)

		// Update conversation view if active
		if m.mode == ModeConversation {
			cv, _ := m.convView.Update(msg)
			m.convView = cv
		}

	case tea.KeyMsg:
		switch m.mode {
		case ModeList:
			// *** FIX: Check if the list is filtering before handling keys ***
			// This prevents your custom navigation from overriding list filtering input
			if m.list.FilterState() == list.Filtering {
				break // Let the list handle the key press
			}

			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc":
				// ESC in search results goes back to browse mode
				return m, func() tea.Msg { return switchToBrowseMsg{} }
			case "enter", "v":
				// Both enter and v do the SAME thing - go to conversation view
				if i, ok := m.list.SelectedItem().(searchConversationItem); ok {
					conv, messages, err := m.engine.GetConversation(i.conv.ID)
					if err != nil {
						// Log error for debugging
						fmt.Printf("Error loading conversation %d: %v\n", i.conv.ID, err)
					} else {
						// Create new conversation view
						m.convView = newConversationView(conv, messages, m.width, m.height)
						m.mode = ModeConversation
						m.selected = m.list.Index()
					}
				}
			case "o":
				// Open conversation in claude.ai
				if i, ok := m.list.SelectedItem().(searchConversationItem); ok {
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
			// *** FIX: Removed custom 'down'/'j' and 'up'/'k' handlers ***
			// Let the default list navigation handle these keys for single-item movement.
			default:
				// Let other keys fall through to component
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
				return m, tea.Quit
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

	// Update components (skip if we consumed the event in find mode)
	var cmd tea.Cmd
	if !skipComponentUpdate {
		switch m.mode {
		case ModeList:
			m.list, cmd = m.list.Update(msg)
		}
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// View renders the view
func (m searchModel) View() string {
	switch m.mode {
	case ModeList:
		content := m.list.View()
		help := HelpStyle.Render("↑/↓/j/k: navigate • g/G: top/bottom • PgUp/PgDn: page • enter: view • o: open in claude.ai • q: quit")
		return content + "\n" + help

	case ModeConversation:
		// Delegate to conversation view
		return m.convView.View()
	}

	return ""
}

// The following methods have been moved to conversationView:
// - findInConversation
// - renderConversationWithHighlights
// - extractArtifacts
// - getCurrentMessageWithArtifact
// - saveCurrentArtifact

// openURL opens a URL in the default browser
func openURL(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}
