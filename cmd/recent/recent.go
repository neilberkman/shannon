package recent

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/spf13/cobra"
)

var (
	days   int
	limit  int
	format string
)

// RecentCmd represents the recent command
var RecentCmd = &cobra.Command{
	Use:   "recent",
	Short: "Show recent conversations",
	Long: `Show conversations from the last N days.

Examples:
  # Show conversations from last 7 days (default)
  claudesearch recent

  # Show conversations from last 30 days
  claudesearch recent --days 30

  # Show only 5 most recent
  claudesearch recent --limit 5`,
	RunE: runRecent,
}

func init() {
	RecentCmd.Flags().IntVarP(&days, "days", "d", 7, "number of days to look back")
	RecentCmd.Flags().IntVarP(&limit, "limit", "l", 20, "maximum number of conversations")
	RecentCmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table/id)")
}

func runRecent(cmd *cobra.Command, args []string) error {
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

	// Calculate date threshold
	threshold := time.Now().AddDate(0, 0, -days)

	// Query recent conversations
	query := `
		SELECT id, name, updated_at, message_count
		FROM conversations
		WHERE updated_at >= ?
		ORDER BY updated_at DESC
		LIMIT ?
	`

	rows, err := database.Query(query, threshold.Format("2006-01-02"), limit)
	if err != nil {
		return fmt.Errorf("failed to query conversations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

	// Collect results
	type conversation struct {
		ID           int64
		Name         string
		UpdatedAt    time.Time
		MessageCount int
	}

	var conversations []conversation
	for rows.Next() {
		var c conversation
		var updatedStr string
		err := rows.Scan(&c.ID, &c.Name, &updatedStr, &c.MessageCount)
		if err != nil {
			return fmt.Errorf("failed to scan conversation: %w", err)
		}
		// Parse time
		c.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
		conversations = append(conversations, c)
	}

	// Display results
	if len(conversations) == 0 {
		fmt.Printf("No conversations in the last %d days\n", days)
		return nil
	}

	switch format {
	case "id":
		// Just output IDs for piping
		for _, c := range conversations {
			fmt.Println(c.ID)
		}
	default:
		// Table format
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "ID\tMessages\tLast Updated\tName"); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := fmt.Fprintln(w, "--\t--------\t------------\t----"); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}

		for _, c := range conversations {
			// Format relative time
			relTime := formatRelativeTime(c.UpdatedAt)
			name := truncate(c.Name, 60)
			if _, err := fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", c.ID, c.MessageCount, relTime, name); err != nil {
				return fmt.Errorf("failed to write conversation: %w", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("failed to flush output: %w", err)
		}
	}

	return nil
}

func formatRelativeTime(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	} else if duration < 7*24*time.Hour {
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}

	return t.Format("Jan 2")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
