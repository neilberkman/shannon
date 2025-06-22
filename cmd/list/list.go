package list

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/rendering"
	"github.com/spf13/cobra"
)

var (
	limit      int
	sortBy     string
	searchTerm string
	quiet      bool
	format     string
)

type conversation struct {
	ID           int64
	UUID         string
	Name         string
	CreatedAt    string
	UpdatedAt    string
	MessageCount int
}

// ListCmd represents the list command
var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all conversations",
	Long: `List all conversations in the database with their IDs, names, dates, and message counts.

Examples:
  claudesearch list
  claudesearch list --limit 20
  claudesearch list --search "python"
  claudesearch list --sort date`,
	RunE: runList,
}

func init() {
	ListCmd.Flags().IntVarP(&limit, "limit", "l", 50, "maximum number of conversations to show")
	ListCmd.Flags().StringVarP(&sortBy, "sort", "s", "date", "sort by: date, name, or messages")
	ListCmd.Flags().StringVar(&searchTerm, "search", "", "filter conversations by name")
	ListCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress extra output (pipe-friendly)")
	ListCmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table/json/csv)")
}

func runList(cmd *cobra.Command, args []string) error {
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

	// Build query
	query := `
		SELECT id, uuid, name, created_at, updated_at, message_count 
		FROM conversations
	`

	var queryArgs []interface{}

	// Add search filter if provided
	if searchTerm != "" {
		query += " WHERE name LIKE ?"
		queryArgs = append(queryArgs, "%"+searchTerm+"%")
	}

	// Add sorting
	switch sortBy {
	case "name":
		query += " ORDER BY name ASC"
	case "messages":
		query += " ORDER BY message_count DESC"
	default: // date
		query += " ORDER BY updated_at DESC"
	}

	// Add limit
	query += fmt.Sprintf(" LIMIT %d", limit)

	// Execute query
	rows, err := database.Query(query, queryArgs...)
	if err != nil {
		return fmt.Errorf("failed to query conversations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

	// Collect results
	var conversations []conversation
	for rows.Next() {
		var c conversation
		err := rows.Scan(&c.ID, &c.UUID, &c.Name, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount)
		if err != nil {
			return fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, c)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Display results
	if len(conversations) == 0 {
		if !quiet {
			fmt.Println("No conversations found.")
		}
		return nil
	}

	switch format {
	case "json":
		return outputJSON(conversations, getTotalCount(database, searchTerm))
	case "csv":
		return outputCSV(conversations)
	default:
		return outputTable(conversations, getTotalCount(database, searchTerm), searchTerm, quiet)
	}
}

func getTotalCount(database *db.DB, searchTerm string) int {
	query := "SELECT COUNT(*) FROM conversations"
	var args []interface{}

	if searchTerm != "" {
		query += " WHERE name LIKE ?"
		args = append(args, "%"+searchTerm+"%")
	}

	var count int
	if err := database.QueryRow(query, args...).Scan(&count); err != nil {
		// Log the error but return 0 to continue operation
		fmt.Fprintf(os.Stderr, "Warning: failed to get total count: %v\n", err)
		return 0
	}
	return count
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func outputTable(conversations []conversation, total int, searchTerm string, quiet bool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tMessages\tUpdated\tName"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "--\t--------\t-------\t----"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	for _, c := range conversations {
		// Parse and format date
		updatedAt := c.UpdatedAt[:10] // Just the date part
		name := truncate(c.Name, 80)

		// Create clickable conversation ID if hyperlinks are supported
		convIDDisplay := fmt.Sprintf("%d", c.ID)
		if rendering.IsHyperlinksSupported() {
			convIDDisplay = rendering.MakeHyperlinkWithID(convIDDisplay, fmt.Sprintf("shannon://view/%d", c.ID), fmt.Sprintf("conv-%d", c.ID))
		}

		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", convIDDisplay, c.MessageCount, updatedAt, name); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	if !quiet {
		fmt.Printf("\nShowing %d of %d total conversations", len(conversations), total)
		if searchTerm != "" {
			fmt.Printf(" (filtered by '%s')", searchTerm)
		}
		fmt.Println()
	}

	return nil
}

func outputJSON(conversations []conversation, total int) error {
	// Parse dates properly for JSON
	for i := range conversations {
		conversations[i].CreatedAt = parseTime(conversations[i].CreatedAt).Format(time.RFC3339)
		conversations[i].UpdatedAt = parseTime(conversations[i].UpdatedAt).Format(time.RFC3339)
	}

	output := map[string]interface{}{
		"conversations": conversations,
		"count":         len(conversations),
		"total":         total,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputCSV(conversations []conversation) error {
	w := csv.NewWriter(os.Stdout)

	// Header
	if err := w.Write([]string{"id", "uuid", "name", "message_count", "created_at", "updated_at"}); err != nil {
		return err
	}

	// Data
	for _, c := range conversations {
		record := []string{
			fmt.Sprintf("%d", c.ID),
			c.UUID,
			c.Name,
			fmt.Sprintf("%d", c.MessageCount),
			c.CreatedAt,
			c.UpdatedAt,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func parseTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return t
}
