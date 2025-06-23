package imports

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/neilberkman/shannon/internal/models"
)

// Sender constants
const (
	senderHuman     = "human"
	senderAssistant = "assistant"
)

// Parser handles parsing Claude export files
type Parser struct {
	file *os.File
}

// NewParser creates a new parser for the given file
func NewParser(filePath string) (*Parser, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &Parser{file: file}, nil
}

// Close closes the underlying file
func (p *Parser) Close() error {
	return p.file.Close()
}

// Parse parses the export file and returns the data
func (p *Parser) Parse() (*models.ClaudeExport, error) {
	// Get file size for progress tracking
	stat, err := p.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// For large files, we might want to use a streaming JSON parser
	// For now, we'll use standard JSON decoding
	if stat.Size() > 1<<30 { // 1GB
		return nil, fmt.Errorf("file too large (%d bytes), streaming parser not yet implemented", stat.Size())
	}

	// Read and decode JSON array
	decoder := json.NewDecoder(p.file)
	var conversations []models.ClaudeConversation
	if err := decoder.Decode(&conversations); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	export := &models.ClaudeExport{
		Conversations: conversations,
	}

	return export, nil
}

// StreamParse parses the export file in a streaming fashion for large files
// This is more memory efficient for large exports
func (p *Parser) StreamParse(callback func(*models.ClaudeConversation) error) error {
	// Seek to beginning
	if _, err := p.file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	decoder := json.NewDecoder(p.file)

	// Read opening bracket for array
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("failed to read opening token: %w", err)
	}

	// Verify it's an array
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected array, got %v", token)
	}

	// Read conversations one by one
	for decoder.More() {
		var conv models.ClaudeConversation
		if err := decoder.Decode(&conv); err != nil {
			return fmt.Errorf("failed to decode conversation: %w", err)
		}

		if err := callback(&conv); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	// Read closing bracket
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("failed to read closing token: %w", err)
	}

	return nil
}

// ParseTime parses Claude's timestamp format
func ParseTime(timestamp string) (time.Time, error) {
	// Claude uses ISO 8601 format: "2023-12-06T19:45:30.123456+00:00"
	return time.Parse(time.RFC3339Nano, timestamp)
}

// ValidateExport performs basic validation on the export data
func ValidateExport(export *models.ClaudeExport) error {
	if len(export.Conversations) == 0 {
		return fmt.Errorf("no conversations found in export")
	}

	// Check for required fields
	for i, conv := range export.Conversations {
		if conv.UUID == "" {
			return fmt.Errorf("conversation %d missing UUID", i)
		}
		if conv.CreatedAt == "" {
			return fmt.Errorf("conversation %d missing created_at", i)
		}

		// Validate messages
		for j, msg := range conv.ChatMessages {
			if msg.UUID == "" {
				return fmt.Errorf("message %d in conversation %d missing UUID", j, i)
			}
			if msg.Sender != senderHuman && msg.Sender != senderAssistant {
				return fmt.Errorf("message %d in conversation %d has invalid sender: %s", j, i, msg.Sender)
			}
		}
	}

	return nil
}
