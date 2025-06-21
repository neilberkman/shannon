package imports

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/models"
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
	defer parser.Close()

	// Start transaction
	tx, err := i.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

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

	// Insert conversation
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

	stats.ConversationsImported++

	// Detect branches using the new branch detector
	detector := NewBranchDetector(conv.ChatMessages)
	branches := detector.DetectBranches()
	stats.BranchesDetected += len(branches) - 1 // Subtract 1 for main branch

	// Create main branch
	mainBranchID, err := i.createBranch(tx, convID, "main", nil)
	if err != nil {
		return fmt.Errorf("failed to create main branch: %w", err)
	}

	// Import messages
	messageIDMap := make(map[string]int64)
	for idx, msg := range conv.ChatMessages {
		msgCreatedAt, err := ParseTime(msg.CreatedAt)
		if err != nil {
			return fmt.Errorf("invalid message created_at: %w", err)
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

		// Determine parent ID
		var parentID *int64
		if msg.ParentID != nil && *msg.ParentID != "" {
			if pid, ok := messageIDMap[*msg.ParentID]; ok {
				parentID = &pid
			}
		}

		// Insert message
		result, err := tx.Exec(`
			INSERT INTO messages (uuid, conversation_id, sender, text, created_at, parent_id, branch_id, sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.UUID, convID, msg.Sender, text, msgCreatedAt, parentID, mainBranchID, idx)

		if err != nil {
			return fmt.Errorf("failed to insert message: %w", err)
		}

		msgID, _ := result.LastInsertId()
		messageIDMap[msg.UUID] = msgID
		stats.MessagesImported++
	}

	return nil
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
	defer file.Close()

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
