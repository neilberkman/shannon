package imports

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/models"
)

// Sender constants
const (
	senderHuman     = "human"
	senderAssistant = "assistant"
)

// splitZipPath parses a plain path, a .zip path, or a "file.zip!entry" path
// produced by discovery. diskPath is always the real file on disk.
func splitZipPath(filePath string) (diskPath, entryName string, isZip bool) {
	if idx := strings.Index(filePath, "!"); idx != -1 {
		prefix := filePath[:idx]
		if strings.HasSuffix(strings.ToLower(prefix), ".zip") {
			return prefix, filePath[idx+1:], true
		}
	}
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		return filePath, "", true
	}
	return filePath, "", false
}

// DiskPath returns the underlying file path on disk for plain or zip!entry paths.
func DiskPath(filePath string) string {
	diskPath, _, _ := splitZipPath(filePath)
	return diskPath
}

// Parser handles parsing Claude export files (both raw conversations.json and
// .zip archives containing conversations.json).
type Parser struct {
	file      *os.File
	zipReader *zip.ReadCloser
	zipEntry  *zip.File
}

// NewParser creates a new parser for the given JSON or ZIP export file.
func NewParser(filePath string) (*Parser, error) {
	diskPath, entryName, isZip := splitZipPath(filePath)
	if !isZip {
		file, err := os.Open(diskPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		return &Parser{file: file}, nil
	}

	zr, err := zip.OpenReader(diskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip archive: %w", err)
	}

	var matched *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if entryName != "" {
			if f.Name == entryName {
				matched = f
				break
			}
			continue
		}
		if filepath.Base(f.Name) == "conversations.json" {
			matched = f
			break
		}
	}
	if matched == nil {
		_ = zr.Close()
		if entryName != "" {
			return nil, fmt.Errorf("entry %q not found in zip archive %s", entryName, diskPath)
		}
		return nil, fmt.Errorf("no conversations.json found in zip archive %s", diskPath)
	}

	return &Parser{
		zipReader: zr,
		zipEntry:  matched,
	}, nil
}

// Close closes the underlying file or zip archive.
func (p *Parser) Close() error {
	if p.zipReader != nil {
		return p.zipReader.Close()
	}
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}

// openStream returns a fresh reader positioned at the start of the JSON export.
func (p *Parser) openStream() (io.ReadCloser, int64, error) {
	if p.zipEntry != nil {
		rc, err := p.zipEntry.Open()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to open zip entry %s: %w", p.zipEntry.Name, err)
		}
		return rc, int64(p.zipEntry.UncompressedSize64), nil
	}

	stat, err := p.file.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat file: %w", err)
	}
	if _, err := p.file.Seek(0, 0); err != nil {
		return nil, 0, fmt.Errorf("failed to seek: %w", err)
	}
	return io.NopCloser(p.file), stat.Size(), nil
}

// Parse parses the export file and returns the data
func (p *Parser) Parse() (*models.ClaudeExport, error) {
	rc, size, err := p.openStream()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	if size > 1<<30 { // 1GB
		return nil, fmt.Errorf("file too large (%d bytes), use streaming parser", size)
	}

	decoder := json.NewDecoder(rc)
	var conversations []models.ClaudeConversation
	if err := decoder.Decode(&conversations); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return &models.ClaudeExport{
		Conversations: conversations,
	}, nil
}

// StreamParse parses the export file in a streaming fashion for memory efficiency.
func (p *Parser) StreamParse(callback func(*models.ClaudeConversation) error) error {
	rc, _, err := p.openStream()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	decoder := json.NewDecoder(rc)

	// Read opening bracket for array
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("failed to read opening token: %w", err)
	}

	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected array, got %v", token)
	}

	count := 0
	for decoder.More() {
		var conv models.ClaudeConversation
		if err := decoder.Decode(&conv); err != nil {
			return fmt.Errorf("failed to decode conversation: %w", err)
		}
		if err := validateConversation(&conv, count); err != nil {
			return fmt.Errorf("invalid export: %w", err)
		}
		if err := callback(&conv); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
		count++
	}

	// Read closing bracket
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("failed to read closing token: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("invalid export: no conversations found in export")
	}

	return nil
}

// ParseTime parses Claude's timestamp format
func ParseTime(timestamp string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, timestamp)
}

func validateConversation(conv *models.ClaudeConversation, idx int) error {
	if conv.UUID == "" {
		return fmt.Errorf("conversation %d missing UUID", idx)
	}
	if conv.CreatedAt == "" {
		return fmt.Errorf("conversation %d missing created_at", idx)
	}
	for j, msg := range conv.ChatMessages {
		if msg.UUID == "" {
			return fmt.Errorf("message %d in conversation %d missing UUID", j, idx)
		}
		if msg.Sender != senderHuman && msg.Sender != senderAssistant {
			return fmt.Errorf("message %d in conversation %d has invalid sender: %s", j, idx, msg.Sender)
		}
	}
	return nil
}

// ValidateExport performs basic validation on the export data
func ValidateExport(export *models.ClaudeExport) error {
	if len(export.Conversations) == 0 {
		return fmt.Errorf("no conversations found in export")
	}
	for i := range export.Conversations {
		if err := validateConversation(&export.Conversations[i], i); err != nil {
			return err
		}
	}
	return nil
}
