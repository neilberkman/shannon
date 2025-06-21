package imports

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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

// Import imports a Claude export file
func (i *Importer) Import(filePath string) (*models.ImportStats, error) {
	stats := &models.ImportStats{}
	startTime := time.Now()

	// Check if file has already been imported
	hash, err := i.fileHash(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash file: %w", err)
	}

	if imported, err := i.isFileImported(hash); err != nil {
		return nil, err
	} else if imported {
		return nil, fmt.Errorf("file already imported (hash: %s)", hash)
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
			// Only log if it's not already committed
			fmt.Fprintf(os.Stderr, "Warning: failed to rollback transaction: %v\n", err)
		}
	}()

	// Use streaming parse for large files
	fileInfo, _ := os.Stat(filePath)
	if fileInfo.Size() > 100*1024*1024 { // 100MB
		err = i.streamImport(tx, parser, stats)
	} else {
		err = i.batchImport(tx, parser, stats)
	}

	if err != nil {
		_ = i.recordImport(filePath, hash, stats, "failed", err.Error())
		return stats, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		_ = i.recordImport(filePath, hash, stats, "failed", err.Error())
		return stats, fmt.Errorf("failed to commit: %w", err)
	}

	stats.Duration = time.Since(startTime)
	_ = i.recordImport(filePath, hash, stats, "success", "")

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
	existingMessages, err := i.getExistingMessageUUIDs(tx, conv.UUID)
	if err != nil {
		return fmt.Errorf("failed to get existing messages: %w", err)
	}

	// Insert or update conversation
	result, err := tx.Exec(`
		INSERT OR REPLACE INTO conversations (uuid, name, created_at, updated_at, message_count)
		VALUES (?, ?, ?, ?, ?)
	`, conv.UUID, conv.Name, createdAt, updatedAt, len(conv.ChatMessages))

	if err != nil {
		return fmt.Errorf("failed to insert conversation: %w", err)
	}

	convID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get conversation ID: %w", err)
	}

	// Only increment if it's a new conversation
	if len(existingMessages) == 0 {
		stats.ConversationsImported++
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

// getExistingMessageUUIDs returns a map of existing message UUIDs for a conversation
func (i *Importer) getExistingMessageUUIDs(tx *sql.Tx, convUUID string) (map[string]struct{}, error) {
	query := `
		SELECT m.uuid 
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

	existing := make(map[string]struct{})
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		existing[uuid] = struct{}{}
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
func (i *Importer) importNewMessages(tx *sql.Tx, convID, mainBranchID int64, messages []models.ClaudeChatMessage, existingMessages map[string]struct{}, stats *models.ImportStats) (int, int, error) {
	messageIDMap := make(map[string]int64)
	newMessagesCount := 0
	branchesDetected := 0

	// Load existing message ID mappings
	if err := i.loadExistingMessageIDs(tx, convID, messageIDMap); err != nil {
		return 0, 0, err
	}

	for idx, msg := range messages {
		// Skip if message already exists
		if _, exists := existingMessages[msg.UUID]; exists {
			continue
		}

		// This is a new message
		msgCreatedAt, err := ParseTime(msg.CreatedAt)
		if err != nil {
			return newMessagesCount, branchesDetected, fmt.Errorf("invalid message created_at: %w", err)
		}

		// Get message text
		text := msg.Text
		if text == "" && len(msg.Content) > 0 {
			for _, content := range msg.Content {
				if content.Type == "text" && content.Text != "" {
					text = content.Text
					break
				}
			}
		}

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
			INSERT INTO messages (uuid, conversation_id, sender, text, created_at, parent_id, branch_id, sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.UUID, convID, msg.Sender, text, msgCreatedAt, parentID, branchID, idx)

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
	err := i.db.QueryRow("SELECT COUNT(*) FROM import_history WHERE file_hash = ?", hash).Scan(&count)
	return count > 0, err
}

func (i *Importer) recordImport(filePath, hash string, stats *models.ImportStats, status, errorMsg string) error {
	_, err := i.db.Exec(`
		INSERT INTO import_history (file_path, file_hash, conversations_count, messages_count, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?)
	`, filePath, hash, stats.ConversationsImported, stats.MessagesImported, status, errorMsg)
	return err
}
