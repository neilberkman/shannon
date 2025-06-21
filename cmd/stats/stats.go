package stats

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/search"
)

// StatsCmd represents the stats command
var StatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show database statistics",
	Long:  `Display statistics about your imported Claude conversations.`,
	RunE:  runStats,
}

func runStats(cmd *cobra.Command, args []string) error {
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

	// Get stats
	stats, err := engine.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	// Display stats
	fmt.Println("=== Claude Search Database Statistics ===")
	fmt.Printf("\nTotal Conversations: %d\n", stats["total_conversations"])
	fmt.Printf("Total Messages: %d\n", stats["total_messages"])

	if msgStats, ok := stats["messages_by_sender"].(map[string]int); ok {
		fmt.Printf("\nMessages by Sender:\n")
		fmt.Printf("  Human:     %d\n", msgStats["human"])
		fmt.Printf("  Assistant: %d\n", msgStats["assistant"])
	}

	if dateRange, ok := stats["date_range"].(map[string]time.Time); ok {
		fmt.Printf("\nDate Range:\n")
		fmt.Printf("  Oldest: %s\n", dateRange["oldest"].Format("2006-01-02"))
		fmt.Printf("  Newest: %s\n", dateRange["newest"].Format("2006-01-02"))

		duration := dateRange["newest"].Sub(dateRange["oldest"])
		fmt.Printf("  Span:   %.0f days\n", duration.Hours()/24)
	}

	return nil
}
