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
	conv       *models.Conversation
	matchCount int
	snippets   []string // Sample snippets from matching messages
}

func (i searchConversationItem) Title() string {
	return i.conv.Name
}

func (i searchConversationItem) Description() string {
	dateStr := formatConversationDates(i.conv.CreatedAt, i.conv.UpdatedAt)

	matchLabel := ""
	if i.matchCount == 1 {
		matchLabel = " • 1 match"
	} else if i.matchCount > 1 {
		matchLabel = fmt.Sprintf(" • %d matches", i.matchCount)
	}

	snippet := ""
	if len(i.snippets) > 0 {
		snippet = i.snippets[0]
		snippet = strings.ReplaceAll(snippet, "<mark>", "")
		snippet = strings.ReplaceAll(snippet, "</mark>", "")
		snippet = strings.Join(strings.Fields(snippet), " ")
		runes := []rune(snippet)
		if len(runes) > 65 {
			snippet = string(runes[:62]) + "..."
		}
		snippet = " • " + snippet
	}
	return fmt.Sprintf("%s • %d msgs%s%s", dateStr, i.conv.MessageCount, matchLabel, snippet)
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
	conversations []*models.Conversation // Ranked conversations from search results
	list          list.Model
	textInput     textinput.Model
	mode          Mode
	searching     bool
	selected      int
	width         int
	height        int
	query         string

	// Conversation view handles all conversation display and interaction
	convView conversationView
}

// newSearchModel creates a new search model preserving ranked conversation order.
func newSearchModel(engine *search.Engine, results []*search.ConversationSearchResult, query string) searchModel {
	items := make([]list.Item, 0, len(results))
	conversations := make([]*models.Conversation, 0, len(results))

	for _, res := range results {
		if res.Conversation == nil {
			continue
		}
		snippets := make([]string, 0, 3)
		for _, m := range res.Matches {
			if len(snippets) >= 3 {
				break
			}
			if strings.TrimSpace(m.Snippet) != "" {
				snippets = append(snippets, m.Snippet)
			}
		}
		items = append(items, searchConversationItem{
			conv:       res.Conversation,
			matchCount: len(res.Matches),
			snippets:   snippets,
		})
		conversations = append(conversations, res.Conversation)
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

	l := list.New(items, delegate, width, height-4)
	l.Title = fmt.Sprintf("Search (%d): %s", len(conversations), query)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	// Create text input for refining search directly from results
	ti := textinput.New()
	ti.Placeholder = "Search (e.g. python after:7d sender:human)..."
	ti.CharLimit = 200
	ti.Width = 60
	ti.SetValue(query)

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
			if m.searching {
				switch msg.String() {
				case "enter":
					query := strings.TrimSpace(m.textInput.Value())
					m.searching = false
					m.textInput.Blur()
					if query != "" {
						opts := search.ParseQuery(query)
						opts.Limit = 200
						if opts.SortBy == "" {
							opts.SortBy = "relevance"
						}
						opts.SortOrder = "desc"
						if results, err := m.engine.SearchConversationsGrouped(opts); err == nil {
							next := newSearchModel(m.engine, results, query)
							next.width = m.width
							next.height = m.height
							next.list.SetSize(m.width, m.height-4)
							return next, nil
						}
					}
					return m, nil
				case "esc":
					m.searching = false
					m.textInput.SetValue(m.query)
					m.textInput.Blur()
					return m, nil
				default:
					ti, cmd := m.textInput.Update(msg)
					m.textInput = ti
					return m, cmd
				}
			}

			// *** FIX: Check if the list is filtering before handling keys ***
			// This prevents your custom navigation from overriding list filtering input
			if m.list.FilterState() == list.Filtering {
				break // Let the list handle the key press
			}

			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "/":
				m.searching = true
				m.textInput.SetValue(m.query)
				m.textInput.CursorEnd()
				m.textInput.Focus()
				return m, textinput.Blink
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
			// Store the previous states
			wasInArtifactMode := m.convView.focusedOnArtifact
			wasInFindMode := m.convView.findActive

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
				// If we were in find mode and now we're not, the conversation view handled it
				if wasInFindMode && !m.convView.findActive {
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
		searchBar := ""
		if m.searching {
			searchBar = TitleStyle.Render("Search: ") + m.textInput.View() + "\n"
		}
		content := m.list.View()
		help := HelpStyle.Render("↑/↓/j/k: navigate • /: refine search (after:, before:, sender:) • enter: view • o: open • esc: browse • q: quit")
		return searchBar + content + "\n" + help

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
