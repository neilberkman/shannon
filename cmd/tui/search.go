package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/models"
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
	// DEBUG: Just plain text until we find the real hang
	return i.getPlainSnippet()
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
	ModeConversation
)

// searchModel is the main model for search TUI
type searchModel struct {
	engine       *search.Engine
	results      []*models.SearchResult
	list         list.Model
	textInput    textinput.Model
	viewport     viewport.Model
	mode         Mode
	selected     int
	width        int
	height       int
	query        string
	conversation *models.Conversation
	messages     []*models.Message
	findQuery    string
	findActive   bool
	findMatches  []int // line numbers that match the find query
	currentMatch int   // current match index
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

	// Create text input for find
	ti := textinput.New()
	ti.Placeholder = "Find in conversation..."
	ti.CharLimit = 100
	ti.Width = 50

	return searchModel{
		engine:    engine,
		results:   results,
		list:      l,
		textInput: ti,
		viewport:  viewport.New(width, height-3),
		mode:      ModeList,
		width:     width,
		height:    height,
		query:     query,
	}
}

// Init initializes the model
func (m searchModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
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
			// *** FIX: Check if the list is filtering before handling keys ***
			// This prevents your custom navigation from overriding list filtering input
			if m.list.FilterState() == list.Filtering {
				break // Let the list handle the key press
			}

			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "enter", "v":
				// Both enter and v do the SAME thing - go to conversation view
				if i, ok := m.list.SelectedItem().(searchItem); ok {
					conv, messages, err := m.engine.GetConversation(i.result.ConversationID)
					if err != nil {
						// Log error for debugging
						fmt.Printf("Error loading conversation %d: %v\n", i.result.ConversationID, err)
					} else {
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.selected = m.list.Index()

						// Set content and go to top
						m.viewport.SetContent(RenderConversation(conv, messages, m.width))
						m.viewport.GotoTop()
					}
				}
			case "o":
				// Open conversation in claude.ai
				if i, ok := m.list.SelectedItem().(searchItem); ok {
					url := fmt.Sprintf("https://claude.ai/chat/%s", i.result.ConversationUUID)
					openURL(url)
				}
			case "g":
				// Jump to beginning
				m.list.Select(0)
			case "G":
				// Jump to end
				m.list.Select(len(m.results) - 1)
			case "home":
				// Jump to beginning
				m.list.Select(0)
			case "end":
				// Jump to end
				m.list.Select(len(m.results) - 1)
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
				if newIndex >= len(m.results) {
					newIndex = len(m.results) - 1
				}
				m.list.Select(newIndex)
			// *** FIX: Removed custom 'down'/'j' and 'up'/'k' handlers ***
			// Let the default list navigation handle these keys for single-item movement.
			default:
				// Let other keys fall through to component
			}

		case ModeConversation:
			if m.findActive {
				switch msg.String() {
				case "enter":
					if m.textInput.Value() != "" {
						m.findQuery = m.textInput.Value()
						m.findMatches = m.findInConversation(m.findQuery)
						m.currentMatch = 0
						if len(m.findMatches) > 0 {
							m.viewport.SetYOffset(m.findMatches[0])
						}
					}
					m.findActive = false
					m.textInput.Blur()
				case "esc":
					m.findActive = false
					m.findQuery = ""
					m.findMatches = nil
					m.textInput.SetValue("")
					m.textInput.Blur()
				default:
					ti, cmd := m.textInput.Update(msg)
					m.textInput = ti
					cmds = append(cmds, cmd)
				}
			} else {
				switch msg.String() {
				case "q", "esc":
					m.mode = ModeList
				case "/", "f":
					m.findActive = true
					m.textInput.SetValue("")
					m.textInput.Placeholder = "Find in conversation..."
					m.textInput.Focus()
					cmds = append(cmds, textinput.Blink)
				case "n":
					if len(m.findMatches) > 0 {
						m.currentMatch = (m.currentMatch + 1) % len(m.findMatches)
						m.viewport.SetYOffset(m.findMatches[m.currentMatch])
					}
				case "N":
					if len(m.findMatches) > 0 {
						m.currentMatch = (m.currentMatch - 1 + len(m.findMatches)) % len(m.findMatches)
						m.viewport.SetYOffset(m.findMatches[m.currentMatch])
					}
				default:
					vp, cmd := m.viewport.Update(msg)
					m.viewport = vp
					cmds = append(cmds, cmd)
				}
			}
		}
	}

	// Update components
	var cmd tea.Cmd
	switch m.mode {
	case ModeList:
		m.list, cmd = m.list.Update(msg)
	case ModeConversation:
		m.viewport, cmd = m.viewport.Update(msg)
	}
	
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// View renders the view
func (m searchModel) View() string {
	var content string

	switch m.mode {
	case ModeList:
		content = m.list.View()
	case ModeConversation:
		content = m.viewport.View()
	}

	// Add help text
	var help string
	switch m.mode {
	case ModeList:
		help = HelpStyle.Render("↑/↓/j/k: navigate • g/G: top/bottom • PgUp/PgDn: page • enter: view • o: open in claude.ai • q: quit")
	case ModeConversation:
		if m.findActive {
			help = HelpStyle.Render("enter: search • esc: cancel")
		} else {
			help = HelpStyle.Render("↑/↓: scroll • /f: find • n/N: next/prev match • esc: back • q: quit")
		}
	}

	// For conversation mode, add find interface
	if m.mode == ModeConversation {
		var findBar string
		if m.findActive {
			findBar = TitleStyle.Render("Find: ") + m.textInput.View() + "\n"
		} else if m.findQuery != "" {
			if len(m.findMatches) > 0 {
				findBar = HelpStyle.Render(fmt.Sprintf("Found %d matches for '%s' • Match %d/%d • n: next • N: prev", 
					len(m.findMatches), m.findQuery, m.currentMatch+1, len(m.findMatches))) + "\n"
			} else {
				findBar = HelpStyle.Render(fmt.Sprintf("No matches found for '%s' • Press / to search again", m.findQuery)) + "\n"
			}
		}
		return findBar + content + "\n" + help
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

// findInConversation searches for a query in the conversation and returns line numbers of matches
func (m searchModel) findInConversation(query string) []int {
	if m.conversation == nil || m.messages == nil || query == "" {
		return nil
	}
	
	// Generate the conversation text to search through
	content := RenderConversation(m.conversation, m.messages, m.width)
	lines := strings.Split(content, "\n")
	
	var matches []int
	queryLower := strings.ToLower(query)
	
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			matches = append(matches, i)
		}
	}
	
	return matches
}

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
	exec.Command(cmd, args...).Start()
}
