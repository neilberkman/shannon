package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/models"
	clipboard "golang.design/x/clipboard"
)

// conversationView handles the display and interaction for a single conversation
// This is shared by both browse and search models to ensure consistent behavior
type conversationView struct {
	viewport     viewport.Model
	textInput    textinput.Model
	conversation *models.Conversation
	messages     []*models.Message
	width        int
	height       int

	// Find functionality
	findQuery    string
	findActive   bool
	findMatches  []int // line numbers that match the find query
	currentMatch int   // current match index

	// Artifact support
	artifacts         map[int64][]*artifacts.Artifact // message ID -> artifacts
	focusedOnArtifact bool
	artifactIndex     int             // which artifact in current message
	messageIndex      int             // which message we're viewing artifacts for
	expandedArtifacts map[string]bool // artifact ID -> expanded state

	// Notification support
	notification      string
	notificationTimer int // frames until notification disappears
}

// newConversationView creates a new conversation view
func newConversationView(conv *models.Conversation, messages []*models.Message, width, height int) conversationView {
	ti := textinput.New()
	ti.Placeholder = "Find in conversation..."
	ti.CharLimit = 100
	ti.Width = 50

	cv := conversationView{
		viewport:          viewport.New(width, height-3),
		textInput:         ti,
		conversation:      conv,
		messages:          messages,
		width:             width,
		height:            height,
		artifacts:         make(map[int64][]*artifacts.Artifact),
		expandedArtifacts: make(map[string]bool),
	}

	// Extract artifacts on creation
	cv.extractArtifacts()

	// Set initial content
	cv.updateContent()
	cv.viewport.GotoTop()

	return cv
}

// Init initializes the conversation view
func (cv conversationView) Init() tea.Cmd {
	return nil
}

// tickMsg is sent to update the notification timer
type tickMsg struct{}

// Update handles messages for the conversation view
func (cv conversationView) Update(msg tea.Msg) (conversationView, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle notification timer
	if cv.notificationTimer > 0 {
		cv.notificationTimer--
		if cv.notificationTimer == 0 {
			cv.notification = ""
		} else {
			// Schedule next tick
			cmds = append(cmds, tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
				return tickMsg{}
			}))
		}
	}

	switch msg := msg.(type) {
	case tickMsg:
		// Handled above

	case tea.WindowSizeMsg:
		cv.width = msg.Width
		cv.height = msg.Height
		cv.viewport.Width = msg.Width
		cv.viewport.Height = msg.Height - 3
		cv.updateContent()

	case tea.KeyMsg:
		if cv.findActive {
			switch msg.String() {
			case "enter":
				if cv.textInput.Value() != "" {
					cv.findQuery = cv.textInput.Value()
					cv.findMatches = cv.findInConversation(cv.findQuery)
					cv.currentMatch = 0
					if len(cv.findMatches) > 0 {
						cv.viewport.SetYOffset(cv.findMatches[0])
					}
				}
				cv.findActive = false
				cv.textInput.Blur()
			case "esc":
				cv.findActive = false
				cv.findQuery = ""
				cv.findMatches = nil
				cv.textInput.SetValue("")
				cv.textInput.Blur()
			default:
				ti, cmd := cv.textInput.Update(msg)
				cv.textInput = ti
				cmds = append(cmds, cmd)
			}
		} else {
			switch msg.String() {
			case "/", "f":
				cv.findActive = true
				cv.textInput.SetValue("")
				cv.textInput.Focus()
				cmds = append(cmds, textinput.Blink)
			case "n":
				if cv.focusedOnArtifact {
					// Navigate to next artifact
					cv.moveToNextArtifact()
					cv.updateContent()
					cv.scrollToFocusedArtifact()
				} else if len(cv.findMatches) > 0 {
					// Next search match
					cv.currentMatch = (cv.currentMatch + 1) % len(cv.findMatches)
					cv.viewport.SetYOffset(cv.findMatches[cv.currentMatch])
				}
			case "N":
				if cv.focusedOnArtifact {
					// Navigate to previous artifact
					cv.moveToPreviousArtifact()
					cv.updateContent()
					cv.scrollToFocusedArtifact()
				} else if len(cv.findMatches) > 0 {
					// Previous search match
					cv.currentMatch = (cv.currentMatch - 1 + len(cv.findMatches)) % len(cv.findMatches)
					cv.viewport.SetYOffset(cv.findMatches[cv.currentMatch])
				}
			case "g":
				cv.viewport.GotoTop()
			case "G":
				cv.viewport.GotoBottom()
			case "a":
				// Enter artifact focus mode
				if len(cv.artifacts) > 0 && !cv.focusedOnArtifact {
					cv.focusedOnArtifact = true
					cv.messageIndex = cv.findFirstMessageWithArtifacts()
					cv.artifactIndex = 0
					cv.updateContent()
					cv.scrollToFocusedArtifact()
				}
			case "esc":
				// Exit artifact focus mode
				if cv.focusedOnArtifact {
					savedY := cv.viewport.YOffset
					cv.focusedOnArtifact = false
					cv.updateContent()
					cv.viewport.SetYOffset(savedY)
				}
			case "tab":
				// Toggle expand/collapse current artifact
				if cv.focusedOnArtifact {
					// Toggle the expansion state of the current artifact
					msgID := cv.getCurrentMessageWithArtifact()
					if msgID > 0 && cv.artifacts[msgID] != nil && cv.artifactIndex < len(cv.artifacts[msgID]) {
						artifact := cv.artifacts[msgID][cv.artifactIndex]
						// Toggle expanded state
						if cv.expandedArtifacts == nil {
							cv.expandedArtifacts = make(map[string]bool)
						}
						cv.expandedArtifacts[artifact.ID] = !cv.expandedArtifacts[artifact.ID]
						cv.updateContent()
					}
				}
			case "s":
				// Save current artifact if focused
				if cv.focusedOnArtifact {
					cv.saveCurrentArtifact()
					cmds = append(cmds, tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
						return tickMsg{}
					}))
				}
			case "c":
				// Copy current artifact to clipboard if focused
				if cv.focusedOnArtifact {
					cv.copyCurrentArtifact()
					cmds = append(cmds, tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
						return tickMsg{}
					}))
				}
			case "o":
				// Open conversation in Claude web interface
				if cv.conversation != nil && cv.conversation.UUID != "" {
					url := fmt.Sprintf("https://claude.ai/chat/%s", cv.conversation.UUID)
					openURL(url)
				}
			default:
				// Handle viewport scrolling
				vp, cmd := cv.viewport.Update(msg)
				cv.viewport = vp
				cmds = append(cmds, cmd)
			}
		}
	}

	return cv, tea.Batch(cmds...)
}

// View renders the conversation view
func (cv conversationView) View() string {
	content := cv.viewport.View()

	// Find interface
	var findBar string
	if cv.findActive {
		findBar = TitleStyle.Render("Find: ") + cv.textInput.View() + "\n"
	} else if cv.findQuery != "" {
		if len(cv.findMatches) > 0 {
			findBar = HelpStyle.Render(fmt.Sprintf("Found %d matches for '%s' • Match %d/%d • n: next • N: prev",
				len(cv.findMatches), cv.findQuery, cv.currentMatch+1, len(cv.findMatches))) + "\n"
		} else {
			findBar = HelpStyle.Render(fmt.Sprintf("No matches found for '%s' • Press / to search again", cv.findQuery)) + "\n"
		}
	}

	// Help text
	var help string
	if cv.findActive {
		help = HelpStyle.Render("enter: search • esc: cancel")
	} else if len(cv.artifacts) > 0 {
		if cv.focusedOnArtifact {
			help = HelpStyle.Render("esc: exit focus • tab: expand/collapse • n/N: navigate • s: save • c: copy • o: open • q: quit")
		} else {
			help = HelpStyle.Render("↑/↓: scroll • g/G: top/bottom • /f: find • n/N: next/prev • a: focus artifact • o: open in claude.ai • esc: back • q: quit")
		}
	} else {
		help = HelpStyle.Render("↑/↓: scroll • g/G: top/bottom • /f: find • n/N: next/prev match • o: open in claude.ai • esc: back • q: quit")
	}

	// Add notification if present
	if cv.notification != "" {
		// Create a centered notification box
		notifStyle := NotificationStyle.Width(len(cv.notification) + 4).Align(lipgloss.Center)
		notification := notifStyle.Render(" " + cv.notification + " ")

		// Position it near the top of the viewport
		lines := strings.Split(content, "\n")
		if len(lines) > 3 {
			// Insert notification at line 3
			lines[2] = lipgloss.PlaceHorizontal(cv.width, lipgloss.Center, notification)
		}
		content = strings.Join(lines, "\n")
	}

	return findBar + content + "\n" + help
}

// Helper methods

// updateContent updates the viewport content
func (cv *conversationView) updateContent() {
	cv.viewport.SetContent(RenderConversationWithArtifacts(
		cv.conversation,
		cv.messages,
		cv.artifacts,
		cv.width,
		cv.focusedOnArtifact,
		cv.messageIndex,
		cv.artifactIndex,
		cv.expandedArtifacts,
	))
}

// findInConversation searches for a query in the conversation
func (cv conversationView) findInConversation(query string) []int {
	if cv.conversation == nil || cv.messages == nil || query == "" {
		return nil
	}

	content := RenderConversationWithArtifacts(cv.conversation, cv.messages, cv.artifacts, cv.width, cv.focusedOnArtifact, cv.messageIndex, cv.artifactIndex, cv.expandedArtifacts)
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
func (cv *conversationView) extractArtifacts() {
	cv.artifacts = make(map[int64][]*artifacts.Artifact)
	extractor := artifacts.NewExtractor()

	for _, msg := range cv.messages {
		if msg.Sender == "assistant" {
			msgArtifacts, _ := extractor.ExtractFromMessage(msg)
			if len(msgArtifacts) > 0 {
				cv.artifacts[msg.ID] = msgArtifacts
			}
		}
	}
}

// findFirstMessageWithArtifacts returns the index of the first message with artifacts
func (cv *conversationView) findFirstMessageWithArtifacts() int {
	for i, msg := range cv.messages {
		if len(cv.artifacts[msg.ID]) > 0 {
			return i
		}
	}
	return 0
}

// moveToNextArtifact moves to the next artifact, potentially in the next message
func (cv *conversationView) moveToNextArtifact() {
	if cv.messageIndex < 0 || cv.messageIndex >= len(cv.messages) {
		return
	}

	currentMsgID := cv.messages[cv.messageIndex].ID
	currentArtifacts := cv.artifacts[currentMsgID]

	// Try to move to next artifact in current message
	if cv.artifactIndex < len(currentArtifacts)-1 {
		cv.artifactIndex++
		return
	}

	// Move to first artifact of next message with artifacts
	for i := cv.messageIndex + 1; i < len(cv.messages); i++ {
		if len(cv.artifacts[cv.messages[i].ID]) > 0 {
			cv.messageIndex = i
			cv.artifactIndex = 0
			return
		}
	}
}

// moveToPreviousArtifact moves to the previous artifact, potentially in the previous message
func (cv *conversationView) moveToPreviousArtifact() {
	if cv.messageIndex < 0 || cv.messageIndex >= len(cv.messages) {
		return
	}

	// Try to move to previous artifact in current message
	if cv.artifactIndex > 0 {
		cv.artifactIndex--
		return
	}

	// Move to last artifact of previous message with artifacts
	for i := cv.messageIndex - 1; i >= 0; i-- {
		if len(cv.artifacts[cv.messages[i].ID]) > 0 {
			cv.messageIndex = i
			cv.artifactIndex = len(cv.artifacts[cv.messages[i].ID]) - 1
			return
		}
	}
}

// scrollToFocusedArtifact scrolls the viewport to show the currently focused artifact
func (cv *conversationView) scrollToFocusedArtifact() {
	// Get the rendered content to find exact line positions
	content := RenderConversationWithArtifacts(cv.conversation, cv.messages, cv.artifacts, cv.width, cv.focusedOnArtifact, cv.messageIndex, cv.artifactIndex, cv.expandedArtifacts)
	lines := strings.Split(content, "\n")

	// Find the current artifact by looking for the focused indicator
	artifactCount := 0
	targetArtifactIndex := cv.getTotalArtifactIndex()

	for i, line := range lines {
		// Look for artifact headers - they are inside a box and contain "┌─" with an emoji and title
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "┌─") && (strings.Contains(trimmed, "📄") || strings.Contains(trimmed, "💻") || strings.Contains(trimmed, "🌐") || strings.Contains(trimmed, "🖼️") || strings.Contains(trimmed, "⚛️") || strings.Contains(trimmed, "📊")) {
			if artifactCount == targetArtifactIndex {
				// Found our artifact! Scroll to show it with some padding above
				// Look for the box border above the artifact header
				targetLine := i - 1
				// Check if previous line is the top border of the box
				if i > 0 && strings.Contains(lines[i-1], "╭") {
					targetLine = i - 1
				}
				if targetLine < 0 {
					targetLine = 0
				}
				cv.viewport.SetYOffset(targetLine)
				return
			}
			artifactCount++
		}
	}
}

// getTotalArtifactIndex returns the total index of the current artifact across all messages
func (cv *conversationView) getTotalArtifactIndex() int {
	total := 0
	for i := 0; i < cv.messageIndex; i++ {
		if arts := cv.artifacts[cv.messages[i].ID]; len(arts) > 0 {
			total += len(arts)
		}
	}
	return total + cv.artifactIndex
}

// getCurrentMessageWithArtifact returns the ID of the current message that has artifacts
func (cv *conversationView) getCurrentMessageWithArtifact() int64 {
	if cv.messageIndex >= 0 && cv.messageIndex < len(cv.messages) {
		return cv.messages[cv.messageIndex].ID
	}
	return 0
}

// saveCurrentArtifact saves the currently focused artifact to a file
func (cv *conversationView) saveCurrentArtifact() {
	msgID := cv.getCurrentMessageWithArtifact()
	if msgID == 0 || cv.artifacts[msgID] == nil || cv.artifactIndex >= len(cv.artifacts[msgID]) {
		return
	}

	artifact := cv.artifacts[msgID][cv.artifactIndex]

	// Generate filename
	filename := artifact.Title
	if filename == "" {
		filename = fmt.Sprintf("artifact_%d", cv.artifactIndex+1)
	}
	filename = sanitizeFilename(filename)

	// Add extension
	ext := artifact.GetFileExtension()
	if !strings.HasSuffix(filename, ext) {
		filename += ext
	}

	// Save to current directory
	err := os.WriteFile(filename, []byte(artifact.Content), 0644)
	if err != nil {
		cv.notification = fmt.Sprintf("Error: %v", err)
		cv.notificationTimer = 30 // 3 seconds
	} else {
		cv.notification = fmt.Sprintf("✓ Saved to %s", filename)
		cv.notificationTimer = 20 // 2 seconds
	}
}

// copyCurrentArtifact copies the currently focused artifact to clipboard
func (cv *conversationView) copyCurrentArtifact() {
	msgID := cv.getCurrentMessageWithArtifact()
	if msgID == 0 || cv.artifacts[msgID] == nil || cv.artifactIndex >= len(cv.artifacts[msgID]) {
		return
	}

	artifact := cv.artifacts[msgID][cv.artifactIndex]

	// Initialize clipboard if not already initialized
	err := clipboard.Init()
	if err != nil {
		cv.notification = fmt.Sprintf("Clipboard init error: %v", err)
		cv.notificationTimer = 30 // 3 seconds
		return
	}

	// Always write as text format
	clipboard.Write(clipboard.FmtText, []byte(artifact.Content))

	// Also write with custom MIME type if applicable
	switch artifact.Type {
	case artifacts.TypeHTML:
		// Write HTML with proper MIME type
		clipboard.Write(clipboard.FmtText, []byte(artifact.Content))
		// TODO: Once the library supports custom MIME types, use:
		// clipboard.WriteAll([]clipboard.Data{
		//     {Format: clipboard.FmtText, Data: []byte(artifact.Content)},
		//     {Format: "text/html", Data: []byte(artifact.Content)},
		// })
	case artifacts.TypeSVG:
		// SVG is XML-based text
		clipboard.Write(clipboard.FmtText, []byte(artifact.Content))
		// TODO: Add image format when SVG is rendered
	case artifacts.TypeMarkdown:
		// Markdown as plain text
		clipboard.Write(clipboard.FmtText, []byte(artifact.Content))
	case artifacts.TypeCode:
		// Code as plain text with language hint
		clipboard.Write(clipboard.FmtText, []byte(artifact.Content))
	default:
		// Default to text
		clipboard.Write(clipboard.FmtText, []byte(artifact.Content))
	}

	cv.notification = "✓ Copied to clipboard"
	cv.notificationTimer = 20 // 2 seconds
}
