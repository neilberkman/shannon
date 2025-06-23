package search

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/rendering"
	"github.com/neilberkman/shannon/internal/search"
	"github.com/spf13/cobra"
)

var (
	conversationID string
	sender         string
	startDate      string
	endDate        string
	limit          int
	offset         int
	sortBy         string
	sortOrder      string
	format         string
	showSnippets   bool
	showContext    bool
	contextLines   int
	quiet          bool
	markdown       bool
	noMarkdown     bool
)

// searchCmd represents the search command
var SearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search through conversations",
	Long: `Search through your Claude conversations using full-text search.

Query Syntax:
  Simple search:      shannon search "machine learning"
  AND (implicit):     shannon search machine learning  
  AND (explicit):     shannon search "python AND django"
  OR operator:        shannon search "react OR vue OR angular"
  NOT operator:       shannon search "error NOT timeout"
  Exact phrase:       shannon search '"exact phrase match"'
  Wildcard (prefix):  shannon search "data*"

Filters:
  By sender:          shannon search "api" --sender human
  By date range:      shannon search "bug" --after 2024-01-01 --before 2024-12-31
  By date (alt):      shannon search "bug" --start-date 2024-01-01 --end-date 2024-12-31
  Within conversation: shannon search "function" -c 1234

Note: Boolean operators (AND, OR, NOT) are case-insensitive.`,

	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	SearchCmd.Flags().StringVarP(&conversationID, "conversation", "c", "", "search within specific conversation ID")
	SearchCmd.Flags().StringVarP(&sender, "sender", "s", "", "filter by sender (human/assistant)")
	SearchCmd.Flags().StringVar(&startDate, "start-date", "", "filter by start date (YYYY-MM-DD)")
	SearchCmd.Flags().StringVar(&endDate, "end-date", "", "filter by end date (YYYY-MM-DD)")
	// Add shorter aliases
	SearchCmd.Flags().StringVar(&startDate, "after", "", "filter by start date (alias for --start-date)")
	SearchCmd.Flags().StringVar(&endDate, "before", "", "filter by end date (alias for --end-date)")
	SearchCmd.Flags().IntVarP(&limit, "limit", "l", 50, "maximum number of results")
	SearchCmd.Flags().IntVar(&offset, "offset", 0, "offset for pagination")
	SearchCmd.Flags().StringVar(&sortBy, "sort-by", "relevance", "sort by relevance or date")
	SearchCmd.Flags().StringVar(&sortOrder, "sort-order", "desc", "sort order (asc/desc)")
	SearchCmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table/json/csv)")
	SearchCmd.Flags().BoolVar(&showSnippets, "snippets", true, "show text snippets")
	SearchCmd.Flags().BoolVar(&showContext, "context", false, "show full message context")
	SearchCmd.Flags().IntVar(&contextLines, "context-lines", 2, "number of context messages to show")
	SearchCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress extra output (pipe-friendly)")
	SearchCmd.Flags().BoolVarP(&markdown, "markdown", "m", true, "render markdown formatting in output")
	SearchCmd.Flags().BoolVar(&noMarkdown, "no-markdown", false, "disable markdown rendering (plain text only)")
	// Make no-markdown override markdown
	SearchCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if noMarkdown {
			markdown = false
		}
	}
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Validate query
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("search query cannot be empty")
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

	// Build search options
	opts := search.SearchOptions{
		Query:     query,
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	// Parse optional filters
	if conversationID != "" {
		var id int64
		if _, err := fmt.Sscanf(conversationID, "%d", &id); err != nil {
			return fmt.Errorf("invalid conversation ID: %w", err)
		}
		opts.ConversationID = &id
	}

	if sender != "" {
		opts.Sender = sender
	}

	if startDate != "" {
		t, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		opts.StartDate = &t
	}

	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}
		opts.EndDate = &t
	}

	// Perform search
	results, err := engine.Search(opts)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Display results
	switch format {
	case "json":
		return outputJSON(results)
	case "csv":
		return outputCSV(results)
	default:
		return outputTable(results, showSnippets, showContext, contextLines, database, quiet)
	}
}

func outputTable(results []*models.SearchResult, showSnippets bool, showContext bool, contextLines int, database *db.DB, quiet bool) error {
	if len(results) == 0 {
		if !quiet {
			fmt.Println("No results found.")
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	if showSnippets {
		if _, err := fmt.Fprintln(w, "ID\tDate\tConversation\tSender\tSnippet"); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := fmt.Fprintln(w, "--\t----\t------------\t------\t-------"); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(w, "ID\tDate\tConversation\tSender\tMessage ID"); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := fmt.Fprintln(w, "--\t----\t------------\t------\t----------"); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	}

	// Results
	for _, r := range results {
		date := r.CreatedAt.Format("2006-01-02 15:04")
		convName := truncate(r.ConversationName, 50)

		// Create clickable conversation ID if hyperlinks are supported
		convIDDisplay := fmt.Sprintf("%d", r.ConversationID)
		if rendering.IsHyperlinksSupported() {
			// Create a link that runs "shannon view <id>"
			convIDDisplay = rendering.MakeHyperlinkWithID(convIDDisplay, fmt.Sprintf("shannon://view/%d", r.ConversationID), fmt.Sprintf("conv-%d", r.ConversationID))
		}

		if showSnippets {
			snippet := r.Snippet

			// Apply markdown rendering if enabled
			if markdown {
				renderer, err := rendering.NewMarkdownRenderer(60)
				if err == nil {
					rendered, err := renderer.RenderMessage(r.Snippet, r.Sender, true)
					if err == nil {
						snippet = rendered
					}
				}
			}

			// Enhance snippet with hyperlinks
			if rendering.IsHyperlinksSupported() {
				snippet = rendering.EnhanceTextWithLinks(snippet)
			}

			// Clean up for tabular display
			snippet = strings.ReplaceAll(snippet, "\n", " ")
			snippet = truncate(snippet, 60)
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", convIDDisplay, date, convName, r.Sender, snippet); err != nil {
				return fmt.Errorf("failed to write result row: %w", err)
			}
		} else {
			messageUUID := r.MessageUUID[:8]
			if rendering.IsHyperlinksSupported() {
				// Create a link to view the specific message
				messageUUID = rendering.MakeHyperlinkWithID(messageUUID, fmt.Sprintf("shannon://message/%s", r.MessageUUID), fmt.Sprintf("msg-%s", r.MessageUUID[:8]))
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", convIDDisplay, date, convName, r.Sender, messageUUID); err != nil {
				return fmt.Errorf("failed to write result row: %w", err)
			}
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	if !quiet {
		fmt.Printf("\nFound %d results", len(results))
		if len(results) == limit {
			fmt.Printf(" (showing first %d)", limit)
		}
		fmt.Println()
	}

	// Show context if requested
	if showContext && database != nil {
		if !quiet {
			fmt.Println("\n--- Message Context ---")
		}
		for _, r := range results {
			if err := showMessageContext(database, r, contextLines); err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Error showing context for message %s: %v\n", r.MessageUUID, err)
				}
			}
		}
	}

	return nil
}

func outputJSON(results []*models.SearchResult) error {
	output := map[string]interface{}{
		"results": results,
		"count":   len(results),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputCSV(results []*models.SearchResult) error {
	w := csv.NewWriter(os.Stdout)

	// Header
	if err := w.Write([]string{"conversation_id", "conversation_name", "message_uuid", "sender", "created_at", "snippet"}); err != nil {
		return err
	}

	// Results
	for _, r := range results {
		record := []string{
			fmt.Sprintf("%d", r.ConversationID),
			r.ConversationName,
			r.MessageUUID,
			r.Sender,
			r.CreatedAt.Format("2006-01-02 15:04:05"),
			strings.ReplaceAll(r.Snippet, "\n", " "),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func showMessageContext(database *db.DB, result *models.SearchResult, contextLines int) error {
	// Get messages before and after the found message
	query := `
		SELECT m.id, m.uuid, m.text, m.sender, m.created_at
		FROM messages m
		WHERE m.conversation_id = ?
		ORDER BY m.created_at
	`

	rows, err := database.Query(query, result.ConversationID)
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

	// Collect all messages
	var messages []struct {
		ID        int64
		UUID      string
		Text      string
		Sender    string
		CreatedAt string
	}

	targetIndex := -1
	for rows.Next() {
		var msg struct {
			ID        int64
			UUID      string
			Text      string
			Sender    string
			CreatedAt string
		}
		err := rows.Scan(&msg.ID, &msg.UUID, &msg.Text, &msg.Sender, &msg.CreatedAt)
		if err != nil {
			return err
		}

		if msg.UUID == result.MessageUUID {
			targetIndex = len(messages)
		}
		messages = append(messages, msg)
	}

	if targetIndex == -1 {
		return fmt.Errorf("message not found in conversation")
	}

	// Display context
	fmt.Printf("\n[Conversation %d: %s]\n", result.ConversationID, result.ConversationName)
	fmt.Println(strings.Repeat("-", 80))

	// Calculate range
	start := targetIndex - contextLines
	if start < 0 {
		start = 0
	}
	end := targetIndex + contextLines + 1
	if end > len(messages) {
		end = len(messages)
	}

	// Show messages with highlighting for the found message
	for i := start; i < end; i++ {
		msg := messages[i]
		prefix := "  "
		if i == targetIndex {
			prefix = "→ "
		}

		timestamp := msg.CreatedAt[:16] // Just date and time
		sender := rendering.FormatSender(msg.Sender)

		// Apply markdown rendering if enabled
		text := msg.Text
		if markdown {
			renderer, err := rendering.NewMarkdownRenderer(100)
			if err == nil {
				rendered, err := renderer.RenderMessage(msg.Text, msg.Sender, false)
				if err == nil {
					text = rendered
				}
			}
		}

		// Clean up for display
		text = strings.ReplaceAll(text, "\n", " ")
		text = truncate(text, 100)

		if i == targetIndex {
			fmt.Printf("%s[%s] %s: %s\n", prefix, timestamp, sender, text)
		} else {
			fmt.Printf("%s[%s] %s: %s\n", prefix, timestamp, sender, text)
		}
	}

	return nil
}
