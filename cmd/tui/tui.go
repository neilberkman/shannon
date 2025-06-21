package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/search"
)

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

	// Perform initial search if query provided
	var model tea.Model
	if initialQuery != "" {
		opts := search.SearchOptions{
			Query:     initialQuery,
			Limit:     100,
			SortBy:    "relevance",
			SortOrder: "desc",
		}

		results, err := engine.Search(opts)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		// Create search model
		model = newSearchModel(engine, results, initialQuery)
	} else {
		// Create browse model
		model = newBrowseModel(engine)
	}

	// Start TUI
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}
