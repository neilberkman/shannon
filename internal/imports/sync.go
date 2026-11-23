package imports

import (
	"context"
	"strings"

	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/discovery"
)

// SyncStats summarizes an incremental sync across discovered export files.
type SyncStats struct {
	FilesScanned          int
	FilesImported         int
	FilesSkipped          int
	ConversationsImported int
	MessagesImported      int
	MessagesUpdated       int
	Errors                []error
}

// SyncDiscovered scans standard download directories (plus any extraPaths) for
// Claude exports (.zip archives and unzipped conversations.json files) and
// incrementally imports any new or modified ones. Unchanged files short-circuit
// on stat metadata without hashing or parsing JSON.
func SyncDiscovered(ctx context.Context, database *db.DB, extraPaths ...string) (*SyncStats, error) {
	scanner := discovery.NewScanner()
	for _, p := range extraPaths {
		if p != "" {
			scanner.AddSearchPath(p)
		}
	}
	return SyncFromScanner(ctx, database, scanner)
}

// SyncFromScanner runs incremental import over candidates found by scanner.
func SyncFromScanner(ctx context.Context, database *db.DB, scanner *discovery.Scanner) (*SyncStats, error) {
	candidates := scanner.ScanCandidatePaths()
	stats := &SyncStats{FilesScanned: len(candidates)}
	importer := NewImporter(database, 1000, false)

	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		impStats, err := importer.Import(path)
		if err != nil {
			if strings.Contains(err.Error(), "file already imported") {
				stats.FilesSkipped++
				continue
			}
			stats.Errors = append(stats.Errors, err)
			continue
		}
		stats.FilesImported++
		stats.ConversationsImported += impStats.ConversationsImported
		stats.MessagesImported += impStats.MessagesImported
		stats.MessagesUpdated += impStats.MessagesUpdated
	}
	return stats, nil
}
