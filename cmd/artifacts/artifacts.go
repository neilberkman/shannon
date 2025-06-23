package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/search"
	"github.com/spf13/cobra"
)

var (
	outputDir    string
	format       string
	artifactType string
	language     string
	limit        int
)

// NewCmd creates the artifacts command
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifacts",
		Short: "Extract and manage artifacts from conversations",
		Long: `Extract and manage artifacts (code, documents, etc.) from Claude conversations.

Artifacts are special content blocks that Claude generates, such as code files,
markdown documents, SVG images, and more.`,
	}

	// Add subcommands
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newExtractCmd())
	cmd.AddCommand(newViewCmd())

	return cmd
}

// newListCmd creates the list subcommand
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [conversation-id]",
		Short: "List artifacts in a conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conversationID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid conversation ID: %w", err)
			}

			// Get database
			database, err := getDatabase()
			if err != nil {
				return err
			}
			defer func() {
				if err := database.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
				}
			}()

			engine := search.NewEngine(database)
			artifactsList, err := engine.GetConversationArtifacts(conversationID)
			if err != nil {
				return fmt.Errorf("failed to get artifacts: %w", err)
			}

			// Filter by type or language if specified
			filtered := filterArtifacts(artifactsList, artifactType, language)

			// Render the list
			renderer := getRenderer(format)
			fmt.Println(renderer.RenderList(filtered))

			return nil
		},
	}

	cmd.Flags().StringVar(&artifactType, "type", "", "filter by artifact type (code, markdown, html, svg, react, mermaid)")
	cmd.Flags().StringVar(&language, "language", "", "filter by programming language (for code artifacts)")
	cmd.Flags().StringVarP(&format, "format", "f", "terminal", "output format (terminal, markdown)")

	return cmd
}

// newSearchCmd creates the search subcommand
func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for artifacts containing specific text",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")

			// Get database
			database, err := getDatabase()
			if err != nil {
				return err
			}
			defer func() {
				if err := database.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
				}
			}()

			engine := search.NewEngine(database)
			results, err := engine.SearchArtifacts(search.SearchOptions{
				Query: query,
				Limit: limit,
			})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if len(results) == 0 {
				fmt.Println("No artifacts found matching your query.")
				return nil
			}

			// Display results
			renderer := getRenderer(format)
			for i, result := range results {
				fmt.Printf("\n[%d] Conversation: %s\n", i+1, result.Conversation.Name)
				fmt.Printf("    %s\n", renderer.RenderDetail(result.Artifact))
				if result.Snippet != "" {
					fmt.Printf("    Match: %s\n", result.Snippet)
				}
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "maximum number of results")
	cmd.Flags().StringVarP(&format, "format", "f", "terminal", "output format (terminal, markdown)")

	return cmd
}

// newExtractCmd creates the extract subcommand
func newExtractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract [conversation-id]",
		Short: "Extract artifacts from a conversation to files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conversationID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid conversation ID: %w", err)
			}

			// Get database
			database, err := getDatabase()
			if err != nil {
				return err
			}
			defer func() {
				if err := database.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
				}
			}()

			engine := search.NewEngine(database)

			// Get conversation details
			conv, _, err := engine.GetConversation(conversationID)
			if err != nil {
				return fmt.Errorf("failed to get conversation: %w", err)
			}

			// Get artifacts
			artifactsList, err := engine.GetConversationArtifacts(conversationID)
			if err != nil {
				return fmt.Errorf("failed to get artifacts: %w", err)
			}

			if len(artifactsList) == 0 {
				fmt.Println("No artifacts found in this conversation.")
				return nil
			}

			// Create output directory
			if outputDir == "" {
				// Default to conversation name (sanitized)
				outputDir = sanitizeFilename(conv.Name)
			}

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Extract each artifact
			fmt.Printf("Extracting %d artifacts to %s/\n", len(artifactsList), outputDir)

			for i, artifact := range artifactsList {
				filename := generateFilename(artifact, i)
				path := filepath.Join(outputDir, filename)

				if err := os.WriteFile(path, []byte(artifact.Content), 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", filename, err)
				}

				fmt.Printf("  ✓ %s\n", filename)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "output directory (defaults to conversation name)")

	return cmd
}

// newViewCmd creates the view subcommand
func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view [conversation-id] [artifact-index]",
		Short: "View a specific artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			conversationID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid conversation ID: %w", err)
			}

			index, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid artifact index: %w", err)
			}

			// Get database
			database, err := getDatabase()
			if err != nil {
				return err
			}
			defer func() {
				if err := database.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
				}
			}()

			engine := search.NewEngine(database)
			artifactsList, err := engine.GetConversationArtifacts(conversationID)
			if err != nil {
				return fmt.Errorf("failed to get artifacts: %w", err)
			}

			if index < 1 || index > len(artifactsList) {
				return fmt.Errorf("artifact index out of range (1-%d)", len(artifactsList))
			}

			artifact := artifactsList[index-1]
			renderer := getRenderer(format)
			fmt.Println(renderer.RenderDetail(artifact))

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "terminal", "output format (terminal, markdown)")

	return cmd
}

// Helper functions

func getRenderer(format string) artifacts.Renderer {
	switch format {
	case "markdown":
		return artifacts.NewMarkdownRenderer()
	default:
		return artifacts.NewTerminalRenderer()
	}
}

func filterArtifacts(list []*artifacts.Artifact, artifactType, language string) []*artifacts.Artifact {
	if artifactType == "" && language == "" {
		return list
	}

	var filtered []*artifacts.Artifact
	for _, a := range list {
		if artifactType != "" && !strings.Contains(strings.ToLower(a.Type), strings.ToLower(artifactType)) {
			continue
		}
		if language != "" && !strings.EqualFold(a.Language, language) {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered
}

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

func generateFilename(artifact *artifacts.Artifact, index int) string {
	// Use title if available, otherwise use index
	base := artifact.Title
	if base == "" {
		base = fmt.Sprintf("artifact_%d", index+1)
	}

	// Sanitize the base name
	base = sanitizeFilename(base)

	// Add appropriate extension
	ext := artifact.GetFileExtension()

	// Ensure we don't duplicate extensions
	if !strings.HasSuffix(base, ext) {
		base += ext
	}

	return base
}

// getDatabase returns a database connection
func getDatabase() (*db.DB, error) {
	cfg := config.Get()
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return database, nil
}
