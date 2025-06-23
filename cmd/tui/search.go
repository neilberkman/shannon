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
	"github.com/neilberkman/shannon/internal/artifacts"
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
	viewport      viewport.Model
	mode          Mode
	selected      int
	width         int
	height        int
	query         string
	conversation  *models.Conversation
	messages      []*models.Message
	findQuery     string
	findActive    bool
	findMatches   []int // line numbers that match the find query
	currentMatch  int   // current match index

	// Artifact support
	artifacts         map[int64][]*artifacts.Artifact // message ID -> artifacts
	focusedOnArtifact bool
	artifactIndex     int // which artifact in current message
	messageIndex      int // which message we're viewing artifacts for
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
		viewport:      viewport.New(width, height-3),
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
						m.conversation = conv
						m.messages = messages
						m.mode = ModeConversation
						m.selected = m.list.Index()

						// Extract artifacts
						m.extractArtifacts()

						// Clear any previous find state and go to top
						m.findQuery = ""
						m.findMatches = nil
						m.currentMatch = 0
						m.findActive = false

						// Set content and go to top
						m.viewport.SetContent(RenderConversationWithArtifacts(conv, messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex))
						m.viewport.GotoTop()
						m.viewport.SetYOffset(0) // Force to absolute top
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
						// Stay in find mode for n/N navigation
						m.findActive = false // Just blur text input
						m.textInput.Blur()
					}
				case "esc":
					m.findActive = false
					m.findQuery = ""
					m.findMatches = nil
					m.textInput.SetValue("")
					m.textInput.Blur()
					// ESC in find mode: clear find, stay in conversation - RETURN EARLY
					return m, nil
				default:
					ti, cmd := m.textInput.Update(msg)
					m.textInput = ti
					cmds = append(cmds, cmd)
				}
				// Find mode handled - skip conversation mode handlers
			} else {
				switch msg.String() {
				case "q":
					return m, tea.Quit
				case "esc":
					if m.findQuery != "" {
						// Clear find results and stay in conversation (browser back button)
						m.findQuery = ""
						m.findMatches = nil
						m.currentMatch = 0
					} else {
						// Normal conversation mode - go back to search results
						m.mode = ModeList
					}
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
				case "g":
					// Go to top of conversation
					m.viewport.GotoTop()
				case "G":
					// Go to bottom of conversation
					m.viewport.GotoBottom()
				case "tab", "a":
					// Toggle artifact focus
					if len(m.artifacts) > 0 {
						m.focusedOnArtifact = !m.focusedOnArtifact
						// Re-render with new focus state
						m.viewport.SetContent(RenderConversationWithArtifacts(m.conversation, m.messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex))
					}
				case "s":
					// Save current artifact if focused
					if m.focusedOnArtifact {
						m.saveCurrentArtifact()
					}
				case "left", "h":
					// Previous artifact in message
					if m.focusedOnArtifact && m.artifactIndex > 0 {
						m.artifactIndex--
						m.viewport.SetContent(RenderConversationWithArtifacts(m.conversation, m.messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex))
					}
				case "right", "l":
					// Next artifact in message
					if m.focusedOnArtifact {
						msgID := m.getCurrentMessageWithArtifact()
						if msgID > 0 && m.artifacts[msgID] != nil && m.artifactIndex < len(m.artifacts[msgID])-1 {
							m.artifactIndex++
							m.viewport.SetContent(RenderConversationWithArtifacts(m.conversation, m.messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex))
						}
					}
				default:
					vp, cmd := m.viewport.Update(msg)
					m.viewport = vp
					cmds = append(cmds, cmd)
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
		case ModeConversation:
			m.viewport, cmd = m.viewport.Update(msg)
		}
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
		// Use highlighted content if find is active or has matches
		if m.findQuery != "" {
			m.viewport.SetContent(m.renderConversationWithHighlights())
		}
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
		} else if len(m.artifacts) > 0 {
			if m.focusedOnArtifact {
				help = HelpStyle.Render("tab: unfocus • s: save • ←/→: navigate artifacts • esc: back • q: quit")
			} else {
				help = HelpStyle.Render("↑/↓: scroll • g/G: top/bottom • /f: find • n/N: next/prev • tab: focus artifact • esc: back • q: quit")
			}
		} else {
			help = HelpStyle.Render("↑/↓: scroll • g/G: top/bottom • /f: find • n/N: next/prev match • esc: back • q: quit")
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

// findInConversation searches for a query in the conversation and returns line numbers of matches
func (m searchModel) findInConversation(query string) []int {
	if m.conversation == nil || m.messages == nil || query == "" {
		return nil
	}

	// Generate the conversation text to search through
	content := RenderConversationWithArtifacts(m.conversation, m.messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex)
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

// renderConversationWithHighlights renders conversation with find matches highlighted
func (m searchModel) renderConversationWithHighlights() string {
	if m.conversation == nil || m.messages == nil {
		return ""
	}

	content := RenderConversationWithArtifacts(m.conversation, m.messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex)

	// If no find query, return content as-is
	if m.findQuery == "" {
		return content
	}

	// Highlight all instances of the find query
	queryLower := strings.ToLower(m.findQuery)
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, queryLower) {
			// Find and highlight all instances in this line
			start := 0
			var highlightedLine strings.Builder

			for {
				idx := strings.Index(lineLower[start:], queryLower)
				if idx == -1 {
					highlightedLine.WriteString(line[start:])
					break
				}

				actualIdx := start + idx
				highlightedLine.WriteString(line[start:actualIdx])
				matchText := line[actualIdx : actualIdx+len(m.findQuery)]
				highlightedLine.WriteString(FindHighlightStyle.Render(matchText))
				start = actualIdx + len(m.findQuery)
			}

			lines[i] = highlightedLine.String()
		}
	}

	return strings.Join(lines, "\n")
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
	_ = exec.Command(cmd, args...).Start()
}

// extractArtifacts extracts artifacts from the loaded messages
func (m *searchModel) extractArtifacts() {
	m.artifacts = make(map[int64][]*artifacts.Artifact)
	extractor := artifacts.NewExtractor()

	for _, msg := range m.messages {
		if msg.Sender == "assistant" {
			msgArtifacts, _ := extractor.ExtractFromMessage(msg)
			if len(msgArtifacts) > 0 {
				m.artifacts[msg.ID] = msgArtifacts
			}
		}
	}
}

// getCurrentMessageWithArtifact returns the ID of the current message that has artifacts
func (m *searchModel) getCurrentMessageWithArtifact() int64 {
	// For now, return the first message with artifacts
	// In a more sophisticated implementation, we'd track which message the user is viewing
	for _, msg := range m.messages {
		if m.artifacts[msg.ID] != nil && len(m.artifacts[msg.ID]) > 0 {
			return msg.ID
		}
	}
	return 0
}

// saveCurrentArtifact saves the currently focused artifact to a file
func (m *searchModel) saveCurrentArtifact() {
	msgID := m.getCurrentMessageWithArtifact()
	if msgID == 0 || m.artifacts[msgID] == nil || m.artifactIndex >= len(m.artifacts[msgID]) {
		return
	}

	artifact := m.artifacts[msgID][m.artifactIndex]

	// Generate filename
	filename := artifact.Title
	if filename == "" {
		filename = fmt.Sprintf("artifact_%d", m.artifactIndex+1)
	}
	filename = sanitizeFilename(filename)

	// Add extension
	ext := artifact.GetFileExtension()
	if !strings.HasSuffix(filename, ext) {
		filename += ext
	}

	// Save to current directory
	// In a real implementation, you might want to prompt for location
	err := os.WriteFile(filename, []byte(artifact.Content), 0644)
	if err != nil {
		fmt.Printf("Error saving artifact: %v\n", err)
	} else {
		fmt.Printf("Saved artifact to: %s\n", filename)
	}
}
