package imports

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/user/shannon/internal/config"
	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/import"
)

var (
	batchSize int
	force     bool
)

// importCmd represents the import command
var ImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import a Claude export file",
	Long: `Import conversations from a Claude export JSON file into the local database.

The import process will:
- Parse the JSON export file
- Detect conversation branches
- Create full-text search indexes
- Skip files that have already been imported (unless --force is used)`,

	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	ImportCmd.Flags().IntVar(&batchSize, "batch-size", 1000, "number of messages to import at once")
	ImportCmd.Flags().BoolVar(&force, "force", false, "force re-import of already imported files")

	viper.BindPFlag("import.batch_size", ImportCmd.Flags().Lookup("batch-size"))
}

func runImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Get configuration
	cfg := config.Get()

	// Open database
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create importer
	importer := imports.NewImporter(database, cfg.Import.BatchSize, cfg.Import.Verbose || viper.GetBool("verbose"))

	// Import file
	fmt.Printf("Importing %s...\n", filePath)
	stats, err := importer.Import(filePath)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	// Print statistics
	fmt.Printf("\nImport completed in %s:\n", stats.Duration)
	fmt.Printf("  Conversations imported: %d\n", stats.ConversationsImported)
	fmt.Printf("  Messages imported: %d\n", stats.MessagesImported)
	fmt.Printf("  Branches detected: %d\n", stats.BranchesDetected)

	if len(stats.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(stats.Errors))
		if viper.GetBool("verbose") {
			for _, err := range stats.Errors {
				fmt.Printf("  - %v\n", err)
			}
		}
	}

	return nil
}
