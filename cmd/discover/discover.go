package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	imports "github.com/neilberkman/shannon/cmd/import"
	"github.com/neilberkman/shannon/internal/discovery"
	"github.com/spf13/cobra"
)

var (
	includePaths   []string
	recent         bool
	recentDuration string
	autoImport     bool
	showInvalid    bool
	verbose        bool
)

// DiscoverCmd represents the discover command
var DiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Find Claude export files in common locations",
	Long: `Discover Claude conversation export files in your Downloads folder and other common locations.

This command scans for JSON files that look like Claude exports and validates their structure.
It's useful for finding exports you may have forgotten about or for setting up automatic imports.

Examples:
  shannon discover                                    # Find all exports
  shannon discover --recent                          # Find exports from last 7 days
  shannon discover --recent --duration 30d           # Find exports from last 30 days
  shannon discover --include ~/Documents             # Also search Documents folder
  shannon discover --auto-import                     # Import any new valid exports found
  shannon discover --show-invalid                    # Show files that look like exports but are invalid`,
	RunE: runDiscover,
}

func init() {
	DiscoverCmd.Flags().StringSliceVarP(&includePaths, "include", "i", nil, "additional directories to search")
	DiscoverCmd.Flags().BoolVarP(&recent, "recent", "r", false, "only show recent exports (last 7 days)")
	DiscoverCmd.Flags().StringVarP(&recentDuration, "duration", "d", "7d", "duration for recent exports (e.g., 1h, 24h, 7d, 30d)")
	DiscoverCmd.Flags().BoolVarP(&autoImport, "auto-import", "a", false, "automatically import any new valid exports found")
	DiscoverCmd.Flags().BoolVar(&showInvalid, "show-invalid", false, "show files that look like exports but are invalid")
	DiscoverCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show which directories are being searched")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	scanner := discovery.NewScanner()

	// Add additional search paths
	for _, path := range includePaths {
		scanner.AddSearchPath(path)
	}

	// Show search paths if verbose
	if verbose {
		paths := scanner.GetSearchPaths()
		fmt.Println("Searching in:")
		for _, path := range paths {
			fmt.Printf("  - %s\n", path)
		}
		fmt.Println()
	}

	var exports []*discovery.ExportFile
	var err error

	if recent {
		duration, err := parseDuration(recentDuration)
		if err != nil {
			return fmt.Errorf("invalid duration '%s': %w", recentDuration, err)
		}
		exports, err = scanner.GetRecentExports(duration)
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
	} else {
		exports, err = scanner.ScanForExports()
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
	}

	// Filter exports based on flags
	var displayExports []*discovery.ExportFile
	var validExports []*discovery.ExportFile

	for _, export := range exports {
		if export.IsValid {
			validExports = append(validExports, export)
			displayExports = append(displayExports, export)
		} else if showInvalid {
			displayExports = append(displayExports, export)
		}
	}

	// Display results
	if len(displayExports) == 0 {
		if recent {
			fmt.Println("No Claude exports found in the specified time range.")
		} else {
			fmt.Println("No Claude exports found.")
		}
		fmt.Println("\nTip: Try 'shannon discover --show-invalid' to see files that look like exports but couldn't be validated.")
		return nil
	}

	if err := displayExportTable(displayExports); err != nil {
		return err
	}

	// Auto-import if requested
	if autoImport && len(validExports) > 0 {
		// Get unique paths to avoid importing duplicates
		uniqueExports := make(map[string]*discovery.ExportFile)
		for _, export := range validExports {
			uniqueExports[export.Path] = export
		}

		fmt.Printf("\nImporting %d unique export(s)...\n\n", len(uniqueExports))

		successCount := 0
		for path, export := range uniqueExports {
			// Skip zip files for now (would need to extract first)
			if strings.Contains(export.Path, "!") {
				fmt.Printf("⚠️  Skipping zip file: %s (extraction not yet supported)\n", filepath.Base(path))
				continue
			}

			if err := imports.ImportFile(path, false); err != nil {
				fmt.Printf("❌ Failed to import %s: %v\n", filepath.Base(path), err)
			} else {
				successCount++
			}
			fmt.Println() // Add spacing between imports
		}

		if successCount > 0 {
			fmt.Printf("✓ Successfully imported %d file(s)\n", successCount)
		}
	}

	return nil
}

func displayExportTable(exports []*discovery.ExportFile) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(w, "STATUS\tFILE\tSIZE\tMODIFIED\tCONVS\tMSGS\tDATE RANGE"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "------\t----\t----\t--------\t-----\t----\t----------"); err != nil {
		return err
	}

	for _, export := range exports {
		status := "✓"
		if !export.IsValid {
			status = "✗"
		}

		filename := filepath.Base(export.Path)
		if len(filename) > 30 {
			filename = filename[:27] + "..."
		}

		size := formatSize(export.Size)
		modified := export.ModTime.Format("Jan 2 15:04")

		var convs, msgs, dateRange string
		if export.Preview != nil {
			convs = fmt.Sprintf("%d", export.Preview.ConversationCount)
			msgs = fmt.Sprintf("%d", export.Preview.MessageCount)
			dateRange = export.Preview.DateRange
		} else {
			convs = "-"
			msgs = "-"
			dateRange = "-"
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			status, filename, size, modified, convs, msgs, dateRange); err != nil {
			return err
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	// Show summary
	validCount := 0
	for _, export := range exports {
		if export.IsValid {
			validCount++
		}
	}

	fmt.Printf("\nFound %d file(s): %d valid, %d invalid\n", len(exports), validCount, len(exports)-validCount)

	// Show invalid files with errors
	for _, export := range exports {
		if !export.IsValid {
			fmt.Printf("⚠️  %s: %s\n", filepath.Base(export.Path), export.ErrorMessage)
		}
	}

	// Show import suggestion if we found valid exports and not in the main runDiscover function
	if validCount > 0 {
		// Get unique valid export paths (to handle duplicates)
		uniquePaths := make(map[string]bool)
		for _, export := range exports {
			if export.IsValid {
				uniquePaths[export.Path] = true
			}
		}

		if len(uniquePaths) > 0 {
			fmt.Println("\nTo import a file:")
			// Show the first unique valid export as an example
			for path := range uniquePaths {
				fmt.Printf("  shannon import \"%s\"\n", path)
				break
			}
			if len(uniquePaths) > 1 {
				fmt.Println("\nOr import all discovered files:")
				fmt.Println("  shannon discover --auto-import")
			}
		}
	}

	return nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func parseDuration(s string) (time.Duration, error) {
	// Handle simple cases like "7d", "30d", "24h"
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		if d, err := time.ParseDuration(days + "h"); err == nil {
			return d * 24, nil
		}
	}

	// Use standard time.ParseDuration for other formats
	return time.ParseDuration(s)
}
