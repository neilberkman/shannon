package tui

import (
	"fmt"
	"os"
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

	placeholderFind = "Find in conversation..."
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
	viewport      viewport.Model
	mode          Mode
	searching     bool
	width         int
	height        int
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
		viewport:      viewport.New(width, height-3),
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
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3

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
							m.conversation = conv
							m.messages = messages
							m.mode = ModeConversation

							// Extract artifacts
							m.extractArtifacts()

							// Set content and go to top
							m.viewport.SetContent(RenderConversationWithArtifacts(conv, messages, m.artifacts, m.width, m.focusedOnArtifact, m.messageIndex, m.artifactIndex))
							m.viewport.GotoTop()
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
			if m.findActive {
				switch msg.String() {
				case keyEnter:
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
				case keyEsc:
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
					m.textInput.Placeholder = placeholderFind
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
				case "o":
					// Open conversation in Claude web interface
					if m.conversation != nil && m.conversation.UUID != "" {
						url := fmt.Sprintf("https://claude.ai/chat/%s", m.conversation.UUID)
						openURL(url)
					}
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
		content := m.viewport.View()

		// Find interface
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

		// Help
		var help string
		if m.findActive {
			help = HelpStyle.Render("enter: search • esc: cancel")
		} else {
			help = HelpStyle.Render("↑/↓: scroll • /f: find • n/N: next/prev match • o: open in claude.ai • esc: back • q: quit")
		}

		return findBar + content + "\n" + help
	}

	return ""
}

// findInConversation searches for a query in the conversation and returns line numbers of matches
func (m browseModel) findInConversation(query string) []int {
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

// extractArtifacts extracts artifacts from the loaded messages
func (m *browseModel) extractArtifacts() {
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
func (m *browseModel) getCurrentMessageWithArtifact() int64 {
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
func (m *browseModel) saveCurrentArtifact() {
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
