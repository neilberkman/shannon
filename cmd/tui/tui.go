package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/discovery"
	"github.com/neilberkman/shannon/internal/search"
	"github.com/spf13/cobra"
)

// ViewType represents the current active view
type ViewType int

const (
	ViewBrowse ViewType = iota
	ViewSearch
)

// mainModel is the root model that manages global state and child views
type mainModel struct {
	engine           *search.Engine
	currentView      tea.Model
	viewType         ViewType
	width            int
	height           int
	watchFiles       bool
	scanner          *discovery.Scanner
	notification     string
	notificationTime time.Time
}

// newMainModel creates a new main model
func newMainModel(engine *search.Engine, initialQuery string, watchFiles bool) mainModel {
	var currentView tea.Model
	var viewType ViewType

	if initialQuery != "" {
		// Start with search view
		opts := search.SearchOptions{
			Query:     initialQuery,
			Limit:     100,
			SortBy:    "relevance",
			SortOrder: "desc",
		}

		results, err := engine.Search(opts)
		if err == nil {
			currentView = newSearchModel(engine, results, initialQuery)
			viewType = ViewSearch
		} else {
			// Fallback to browse view on error
			currentView = newBrowseModel(engine)
			viewType = ViewBrowse
		}
	} else {
		// Start with browse view
		currentView = newBrowseModel(engine)
		viewType = ViewBrowse
	}

	var scanner *discovery.Scanner
	if watchFiles {
		scanner = discovery.NewScanner()
	}

	return mainModel{
		engine:      engine,
		currentView: currentView,
		viewType:    viewType,
		watchFiles:  watchFiles,
		scanner:     scanner,
	}
}

// checkExportsMsg is sent when we should check for new exports
type checkExportsMsg struct{}

// newExportsFoundMsg is sent when new exports are discovered
type newExportsFoundMsg struct {
	count int
}

// Init initializes the main model
func (m mainModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Initialize child view
	cmds = append(cmds, m.currentView.Init())

	// Start export checking if watching
	if m.watchFiles {
		cmds = append(cmds, tea.Tick(time.Minute*2, func(t time.Time) tea.Msg {
			return checkExportsMsg{}
		}))
	}

	return tea.Batch(cmds...)
}

// Update handles messages and routes them to child views
func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to current view
		var cmd tea.Cmd
		m.currentView, cmd = m.currentView.Update(msg)
		return m, cmd

	case checkExportsMsg:
		if m.scanner != nil {
			return m, tea.Batch(
				func() tea.Msg {
					exports, err := m.scanner.GetRecentExports(time.Minute * 5)
					if err != nil || len(exports) == 0 {
						return nil
					}
					return newExportsFoundMsg{count: len(exports)}
				},
				tea.Tick(time.Minute*2, func(t time.Time) tea.Msg {
					return checkExportsMsg{}
				}),
			)
		}

	case newExportsFoundMsg:
		m.notification = fmt.Sprintf("🆕 Found %d new Claude export(s) in Downloads", msg.count)
		m.notificationTime = time.Now()

	case tea.KeyMsg:
		// Handle global keybindings
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	// Forward all other messages to current view
	var cmd tea.Cmd
	m.currentView, cmd = m.currentView.Update(msg)

	// Check if the child view wants to switch views
	if newView, ok := checkViewSwitch(m.currentView); ok {
		m.currentView = newView
		// Update view type based on the new view
		switch newView.(type) {
		case browseModel:
			m.viewType = ViewBrowse
		case searchModel:
			m.viewType = ViewSearch
		}
	}

	return m, cmd
}

// View renders the current view
func (m mainModel) View() string {
	view := m.currentView.View()

	// Add notification if recent and watching
	if m.notification != "" && time.Since(m.notificationTime) < time.Second*10 {
		// Show notification at the bottom for 10 seconds
		view += "\n" + NotificationStyle.Render(m.notification)
	}

	return view
}

// checkViewSwitch checks if a child view wants to switch to another view
// This is a helper function to handle view transitions
func checkViewSwitch(currentView tea.Model) (tea.Model, bool) {
	// For now, we don't have view switching from child models
	// This could be extended later with custom messages
	return nil, false
}

var (
	initialQuery string
	watchFiles   bool
)

// TuiCmd represents the tui command
var TuiCmd = &cobra.Command{
	Use:   "tui [query]",
	Short: "Launch interactive TUI interface",
	Long: `Launch the interactive terminal user interface for ClaudeSearch.

This provides a visual interface for searching and browsing conversations.

Examples:
  # Launch TUI and search immediately
  claudesearch tui "machine learning"
  
  # Launch TUI in browse mode
  claudesearch tui`,
	RunE: runTUI,
}

func init() {
	TuiCmd.Flags().BoolVarP(&watchFiles, "watch", "w", false, "watch Downloads folder for new Claude exports")
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Get initial query if provided
	if len(args) > 0 {
		initialQuery = strings.Join(args, " ")
	}

	// Get configuration
	cfg := config.Get()

	// Open database
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

	// Create search engine
	engine := search.NewEngine(database)

	// Create main model
	model := newMainModel(engine, initialQuery, watchFiles)

	// Start TUI with logging for debugging
	debugFile, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		// If logging setup fails, continue without it
		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to run TUI: %w", err)
		}
		return nil
	}
	defer func() {
		if err := debugFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close debug file: %v\n", err)
		}
	}()

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}
