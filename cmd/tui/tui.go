package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/search"
)

// ViewType represents the current active view
type ViewType int

const (
	ViewBrowse ViewType = iota
	ViewSearch
)

// mainModel is the root model that manages global state and child views
type mainModel struct {
	engine      *search.Engine
	currentView tea.Model
	viewType    ViewType
	width       int
	height      int
}

// newMainModel creates a new main model
func newMainModel(engine *search.Engine, initialQuery string) mainModel {
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

	return mainModel{
		engine:      engine,
		currentView: currentView,
		viewType:    viewType,
	}
}

// Init initializes the main model
func (m mainModel) Init() tea.Cmd {
	return m.currentView.Init()
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
	return m.currentView.View()
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
	// No special flags needed for now
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
	defer database.Close()

	// Create search engine
	engine := search.NewEngine(database)

	// Create main model
	model := newMainModel(engine, initialQuery)

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
