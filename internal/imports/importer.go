package imports

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
)

// Importer handles importing Claude export files into the database
type Importer struct {
	db        *db.DB
	batchSize int
	verbose   bool
}

// NewImporter creates a new importer
func NewImporter(database *db.DB, batchSize int, verbose bool) *Importer {
	return &Importer{
		db:        database,
		batchSize: batchSize,
		verbose:   verbose,
	}
}

// Import imports a Claude export file, skipping files that have already been
// imported unchanged.
func (i *Importer) Import(filePath string) (*models.ImportStats, error) {
	return i.importFile(filePath, false)
}

// ImportForce re-imports a file even when it has been imported before.
// Conversations and messages are matched by UUID and refreshed in place, so a
// forced re-import repairs previously stored content rather than duplicating
// it. This is how an export is re-read after the importer learns to extract
// more of each message.
func (i *Importer) ImportForce(filePath string) (*models.ImportStats, error) {
	return i.importFile(filePath, true)
}

func (i *Importer) importFile(filePath string, force bool) (*models.ImportStats, error) {
	stats := &models.ImportStats{}
	startTime := time.Now()

	// Stat the file once and reuse for both the unchanged-file fast path and
	// the streaming-vs-batch threshold below.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	inode, device := fileIdentity(fileInfo)

	// Fast path: if any prior import recorded this exact (path, size, mtime,
	// inode, device), the file is unchanged and we can skip without hashing
	// 25+ MB of JSON.
	if !force {
		if imported, err := i.isFileUnchangedSinceImport(filePath, fileInfo, inode, device); err != nil {
			return nil, err
		} else if imported {
			return nil, fmt.Errorf("file already imported (unchanged)")
		}
	}

	// Slow path: hash the file. Catches the case where the same content lives
	// at a different path (e.g. user renamed/moved the export folder) or a
	// cloud-drive sync rewrote mtime without touching content.
	hash, err := i.fileHash(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash file: %w", err)
	}

	if !force {
		if imported, err := i.isFileImported(hash); err != nil {
			return nil, err
		} else if imported {
			// Backfill the identity fields on the existing row so the next run
			// short-circuits via the fast path instead of re-hashing.
			_ = i.backfillImportIdentity(filePath, hash, fileInfo, inode, device)
			return nil, fmt.Errorf("file already imported (hash: %s)", hash)
		}
	}

	// Parse the export file
	parser, err := NewParser(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := parser.Close(); err != nil {
			// Log error but don't fail the import
			fmt.Fprintf(os.Stderr, "Warning: failed to close parser: %v\n", err)
		}
	}()

	// Start transaction
	tx, err := i.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			// Only log if it's not already committed and not a "transaction already committed" error
			if err != sql.ErrTxDone {
				fmt.Fprintf(os.Stderr, "Warning: failed to rollback transaction: %v\n", err)
			}
		}
	}()

	// Use streaming parse for large files
	if fileInfo.Size() > 100*1024*1024 { // 100MB
		err = i.streamImport(tx, parser, stats)
	} else {
		err = i.batchImport(tx, parser, stats)
	}

	if err != nil {
		_ = i.recordImport(filePath, hash, fileInfo, inode, device, stats, "failed", err.Error())
		return stats, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		_ = i.recordImport(filePath, hash, fileInfo, inode, device, stats, "failed", err.Error())
		return stats, fmt.Errorf("failed to commit: %w", err)
	}

	stats.Duration = time.Since(startTime)
	_ = i.recordImport(filePath, hash, fileInfo, inode, device, stats, "success", "")

	return stats, nil
}

func (i *Importer) batchImport(tx *sql.Tx, parser *Parser, stats *models.ImportStats) error {
	export, err := parser.Parse()
	if err != nil {
		return fmt.Errorf("failed to parse export: %w", err)
	}

	if err := ValidateExport(export); err != nil {
		return fmt.Errorf("invalid export: %w", err)
	}

	// Import conversations
	for _, conv := range export.Conversations {
		if err := i.importConversation(tx, &conv, stats); err != nil {
			stats.Errors = append(stats.Errors, fmt.Errorf("conversation %s: %w", conv.UUID, err))
			if i.verbose {
				fmt.Printf("Error importing conversation %s: %v\n", conv.UUID, err)
			}
		}
	}

	return nil
}

func (i *Importer) streamImport(tx *sql.Tx, parser *Parser, stats *models.ImportStats) error {
	return parser.StreamParse(func(conv *models.ClaudeConversation) error {
		if err := i.importConversation(tx, conv, stats); err != nil {
			stats.Errors = append(stats.Errors, fmt.Errorf("conversation %s: %w", conv.UUID, err))
			if i.verbose {
				fmt.Printf("Error importing conversation %s: %v\n", conv.UUID, err)
			}
		}
		return nil
	})
}

func (i *Importer) importConversation(tx *sql.Tx, conv *models.ClaudeConversation, stats *models.ImportStats) error {
	// Parse timestamps
	createdAt, err := ParseTime(conv.CreatedAt)
	if err != nil {
		return fmt.Errorf("invalid created_at: %w", err)
	}

	updatedAt, err := ParseTime(conv.UpdatedAt)
	if err != nil {
		return fmt.Errorf("invalid updated_at: %w", err)
	}

	// Check if conversation already exists and get existing message UUIDs
	existingMessages, err := i.getExistingMessages(tx, conv.UUID)
	if err != nil {
		return fmt.Errorf("failed to get existing messages: %w", err)
	}

	// Check if conversation exists
	var convID int64
	err = tx.QueryRow("SELECT id FROM conversations WHERE uuid = ?", conv.UUID).Scan(&convID)
	if err == sql.ErrNoRows {
		// Insert new conversation
		result, err := tx.Exec(`
			INSERT INTO conversations (uuid, name, created_at, updated_at, message_count)
			VALUES (?, ?, ?, ?, ?)
		`, conv.UUID, conv.Name, createdAt, updatedAt, len(conv.ChatMessages))

		if err != nil {
			return fmt.Errorf("failed to insert conversation: %w", err)
		}

		convID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get conversation ID: %w", err)
		}
		stats.ConversationsImported++
	} else if err != nil {
		return fmt.Errorf("failed to check existing conversation: %w", err)
	} else {
		// Update existing conversation
		_, err = tx.Exec(`
			UPDATE conversations 
			SET name = ?, updated_at = ?, message_count = ?
			WHERE id = ?
		`, conv.Name, updatedAt, len(conv.ChatMessages), convID)

		if err != nil {
			return fmt.Errorf("failed to update conversation: %w", err)
		}
	}

	// Get or create main branch
	mainBranchID, err := i.getOrCreateMainBranch(tx, convID)
	if err != nil {
		return fmt.Errorf("failed to get or create main branch: %w", err)
	}

	// Import only new messages using tree diff approach
	newMessagesCount, branchesDetected, err := i.importNewMessages(tx, convID, mainBranchID, conv.ChatMessages, existingMessages, stats)
	if err != nil {
		return fmt.Errorf("failed to import messages: %w", err)
	}

	stats.MessagesImported += newMessagesCount
	stats.BranchesDetected += branchesDetected

	return nil
}

// existingMessage holds the stored text of an already-imported message, so a
// re-import can tell whether the export now yields richer content.
type existingMessage struct {
	text       string
	searchText string
}

// getExistingMessages returns the already-imported messages of a conversation,
// keyed by UUID, along with the text currently stored for each.
func (i *Importer) getExistingMessages(tx *sql.Tx, convUUID string) (map[string]existingMessage, error) {
	query := `
		SELECT m.uuid, m.text, m.search_text
		FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
		WHERE c.uuid = ?
	`

	rows, err := tx.Query(query, convUUID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

	existing := make(map[string]existingMessage)
	for rows.Next() {
		var uuid string
		var prev existingMessage
		if err := rows.Scan(&uuid, &prev.text, &prev.searchText); err != nil {
			return nil, err
		}
		existing[uuid] = prev
	}

	return existing, rows.Err()
}

// getOrCreateMainBranch gets existing main branch or creates it
func (i *Importer) getOrCreateMainBranch(tx *sql.Tx, convID int64) (int64, error) {
	// Try to get existing main branch
	var branchID int64
	err := tx.QueryRow(`
		SELECT id FROM branches WHERE conversation_id = ? AND name = 'main'
	`, convID).Scan(&branchID)

	if err == sql.ErrNoRows {
		// Create main branch
		return i.createBranch(tx, convID, "main", nil)
	} else if err != nil {
		return 0, err
	}

	return branchID, nil
}

// importNewMessages imports only new messages, detecting branches based on parent relationships
func (i *Importer) importNewMessages(tx *sql.Tx, convID, mainBranchID int64, messages []models.ClaudeChatMessage, existingMessages map[string]existingMessage, stats *models.ImportStats) (int, int, error) {
	messageIDMap := make(map[string]int64)
	newMessagesCount := 0
	branchesDetected := 0

	// Load existing message ID mappings
	if err := i.loadExistingMessageIDs(tx, convID, messageIDMap); err != nil {
		return 0, 0, err
	}

	for idx, msg := range messages {
		// An already-imported message is refreshed rather than skipped: an
		// earlier version of the importer stored only part of each message,
		// so re-importing the same export is how that content is recovered.
		if prev, exists := existingMessages[msg.UUID]; exists {
			if err := i.refreshMessage(tx, &messages[idx], prev, stats); err != nil {
				return newMessagesCount, branchesDetected, err
			}
			continue
		}

		// This is a new message
		msgCreatedAt, err := ParseTime(msg.CreatedAt)
		if err != nil {
			return newMessagesCount, branchesDetected, fmt.Errorf("invalid message created_at: %w", err)
		}

		// Separate the prose a reader sees from everything worth indexing.
		text := DisplayText(&messages[idx])
		searchText := SearchText(&messages[idx])

		// Determine parent ID and branch logic
		var parentID *int64
		branchID := mainBranchID

		if msg.ParentID != nil && *msg.ParentID != "" {
			if pid, ok := messageIDMap[*msg.ParentID]; ok {
				parentID = &pid

				// Check if parent is in main branch - if not, this might be a new branch
				if isNewBranch, err := i.detectNewBranch(tx, pid, mainBranchID); err != nil {
					return newMessagesCount, branchesDetected, err
				} else if isNewBranch {
					// Create new branch
					branchName := fmt.Sprintf("branch-%d", time.Now().Unix())
					branchID, err = i.createBranch(tx, convID, branchName, &mainBranchID)
					if err != nil {
						return newMessagesCount, branchesDetected, err
					}
					branchesDetected++
				}
			}
		}

		// Insert message
		result, err := tx.Exec(`
			INSERT INTO messages (uuid, conversation_id, sender, text, search_text, created_at, parent_id, branch_id, sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.UUID, convID, msg.Sender, text, searchText, msgCreatedAt, parentID, branchID, idx)

		if err != nil {
			return newMessagesCount, branchesDetected, fmt.Errorf("failed to insert message: %w", err)
		}

		msgID, _ := result.LastInsertId()
		messageIDMap[msg.UUID] = msgID
		newMessagesCount++
	}

	return newMessagesCount, branchesDetected, nil
}

// loadExistingMessageIDs loads UUID to ID mappings for existing messages
func (i *Importer) loadExistingMessageIDs(tx *sql.Tx, convID int64, messageIDMap map[string]int64) error {
	rows, err := tx.Query(`
		SELECT id, uuid FROM messages WHERE conversation_id = ?
	`, convID)
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

	for rows.Next() {
		var id int64
		var uuid string
		if err := rows.Scan(&id, &uuid); err != nil {
			return err
		}
		messageIDMap[uuid] = id
	}

	return rows.Err()
}

// detectNewBranch determines if a new message creates a branch
func (i *Importer) detectNewBranch(tx *sql.Tx, parentID, mainBranchID int64) (bool, error) {
	// Check if parent already has children in main branch
	var childCount int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM messages 
		WHERE parent_id = ? AND branch_id = ?
	`, parentID, mainBranchID).Scan(&childCount)

	if err != nil {
		return false, err
	}

	// If parent already has children, this creates a new branch
	return childCount > 0, nil
}

func (i *Importer) createBranch(tx *sql.Tx, convID int64, name string, parentBranchID *int64) (int64, error) {
	result, err := tx.Exec(`
		INSERT INTO branches (conversation_id, name, parent_branch_id)
		VALUES (?, ?, ?)
	`, convID, name, parentBranchID)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (i *Importer) fileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close file: %v\n", err)
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (i *Importer) isFileImported(hash string) (bool, error) {
	var count int
	err := i.db.QueryRow(
		"SELECT COUNT(*) FROM import_history WHERE file_hash = ? AND status = 'success'",
		hash,
	).Scan(&count)
	return count > 0, err
}

// isFileUnchangedSinceImport returns true if a prior successful import for the
// same path is recorded with matching size, mtime, inode, and device — meaning
// the file on disk has not been touched since. Mtime is compared with a
// 1-second tolerance to tolerate filesystems and cloud-drive sync clients that
// round subsecond precision differently across runs.
func (i *Importer) isFileUnchangedSinceImport(filePath string, info os.FileInfo, inode, device uint64) (bool, error) {
	rows, err := i.db.Query(`
		SELECT file_size, file_mtime, file_inode, file_device
		FROM import_history
		WHERE file_path = ? AND status = 'success'
		  AND file_size IS NOT NULL AND file_mtime IS NOT NULL
		ORDER BY imported_at DESC
		LIMIT 5
	`, filePath)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			size      sql.NullInt64
			mtimeStr  sql.NullString
			rowInode  sql.NullInt64
			rowDevice sql.NullInt64
		)
		if err := rows.Scan(&size, &mtimeStr, &rowInode, &rowDevice); err != nil {
			return false, err
		}
		if !size.Valid || !mtimeStr.Valid {
			continue
		}
		if size.Int64 != info.Size() {
			continue
		}

		recordedMtime, perr := parseStoredTime(mtimeStr.String)
		if perr != nil {
			continue
		}
		drift := info.ModTime().Sub(recordedMtime)
		if drift < 0 {
			drift = -drift
		}
		if drift >= time.Second {
			continue
		}

		// inode/device are zero on Windows or on filesystems where Stat_t isn't
		// available — treat as a wildcard rather than a mismatch.
		if inode != 0 && rowInode.Valid && uint64(rowInode.Int64) != inode {
			continue
		}
		if device != 0 && rowDevice.Valid && uint64(rowDevice.Int64) != device {
			continue
		}

		return true, nil
	}
	return false, rows.Err()
}

// parseStoredTime accepts the various string forms SQLite may hand back for
// a DATETIME column written from a Go time.Time.
func parseStoredTime(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

func (i *Importer) recordImport(filePath, hash string, info os.FileInfo, inode, device uint64, stats *models.ImportStats, status, errorMsg string) error {
	_, err := i.db.Exec(`
		INSERT INTO import_history (
			file_path, file_hash, conversations_count, messages_count,
			status, error_message,
			file_size, file_mtime, file_inode, file_device
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		filePath, hash, stats.ConversationsImported, stats.MessagesImported,
		status, errorMsg,
		info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano),
		nullableU64(inode), nullableU64(device),
	)
	return err
}

// nullableU64 returns nil for zero (the sentinel for "not available"), so the
// stored column reflects that we don't know the inode/device rather than a
// real value of 0.
func nullableU64(v uint64) interface{} {
	if v == 0 {
		return nil
	}
	return int64(v)
}

// backfillImportIdentity updates the most recent successful import_history row
// for the given (file_path, file_hash) with the current size/mtime/inode/device
// so a future re-run can hit the fast path without re-hashing. Used when the
// hash check confirms the file was previously imported but the row predates
// migration001 (and thus has NULLs in those columns).
func (i *Importer) backfillImportIdentity(filePath, hash string, info os.FileInfo, inode, device uint64) error {
	_, err := i.db.Exec(`
		UPDATE import_history
		SET file_size = ?, file_mtime = ?, file_inode = ?, file_device = ?
		WHERE id = (
			SELECT id FROM import_history
			WHERE file_path = ? AND file_hash = ? AND status = 'success'
			ORDER BY imported_at DESC
			LIMIT 1
		)
	`,
		info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano),
		nullableU64(inode), nullableU64(device),
		filePath, hash,
	)
	return err
}

// refreshMessage updates an already-imported message when the export yields
// text the stored row is missing. Earlier versions of the importer kept only
// a message's first text block, discarding extended thinking and tool
// results, so re-importing an export repairs those rows in place. Rows that
// already match are left untouched, keeping repeat imports cheap.
func (i *Importer) refreshMessage(tx *sql.Tx, msg *models.ClaudeChatMessage, prev existingMessage, stats *models.ImportStats) error {
	text := DisplayText(msg)
	searchText := SearchText(msg)

	if text == prev.text && searchText == prev.searchText {
		return nil
	}

	// Never trade stored content for less of it: an export that omits a
	// message's body should not blank out what a previous import captured.
	if strings.TrimSpace(text) == "" {
		text = prev.text
	}
	if len(searchText) < len(prev.searchText) {
		searchText = prev.searchText
	}
	if text == prev.text && searchText == prev.searchText {
		return nil
	}

	if _, err := tx.Exec(
		`UPDATE messages SET text = ?, search_text = ? WHERE uuid = ?`,
		text, searchText, msg.UUID,
	); err != nil {
		return fmt.Errorf("failed to refresh message %s: %w", msg.UUID, err)
	}

	stats.MessagesUpdated++
	return nil
}
