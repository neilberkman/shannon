package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/user/shannon/internal/models"
	"github.com/user/shannon/pkg/platform"
)

// ExportFile represents a discovered Claude export file
type ExportFile struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsValid      bool
	ErrorMessage string
	Preview      *ExportPreview
}

// ExportPreview contains basic info about the export
type ExportPreview struct {
	ConversationCount int
	MessageCount      int
	DateRange         string
	FirstConvName     string
}

// Scanner handles discovery of Claude export files
type Scanner struct {
	searchPaths []string
}

// NewScanner creates a new export file scanner
func NewScanner() *Scanner {
	scanner := &Scanner{}
	
	// Add default Downloads directory
	if downloadsDir, err := platform.GetDownloadsDir(); err == nil {
		scanner.searchPaths = append(scanner.searchPaths, downloadsDir)
	}
	
	return scanner
}

// AddSearchPath adds an additional directory to search
func (s *Scanner) AddSearchPath(path string) {
	s.searchPaths = append(s.searchPaths, path)
}

// ScanForExports finds Claude export files in the configured paths
func (s *Scanner) ScanForExports() ([]*ExportFile, error) {
	var exports []*ExportFile
	
	for _, searchPath := range s.searchPaths {
		files, err := s.scanDirectory(searchPath)
		if err != nil {
			// Log error but continue with other directories
			fmt.Fprintf(os.Stderr, "Warning: failed to scan %s: %v\n", searchPath, err)
			continue
		}
		exports = append(exports, files...)
	}
	
	// Sort by modification time (newest first)
	sort.Slice(exports, func(i, j int) bool {
		return exports[i].ModTime.After(exports[j].ModTime)
	})
	
	return exports, nil
}

// scanDirectory scans a single directory for Claude export files
func (s *Scanner) scanDirectory(dir string) ([]*ExportFile, error) {
	var exports []*ExportFile
	
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return exports, nil // Empty slice, no error
	}
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Check if this looks like a Claude export
		if s.isLikelyClaudeExport(path, info) {
			export := &ExportFile{
				Path:    path,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			
			// Validate and preview the file
			export.IsValid, export.ErrorMessage, export.Preview = s.validateAndPreview(path)
			
			exports = append(exports, export)
		}
		
		return nil
	})
	
	return exports, err
}

// isLikelyClaudeExport checks if a file looks like a Claude export
func (s *Scanner) isLikelyClaudeExport(path string, info os.FileInfo) bool {
	// Must be JSON file
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		return false
	}
	
	// Skip very small files (< 1KB)
	if info.Size() < 1024 {
		return false
	}
	
	// Skip very large files (> 500MB) - probably not exports
	if info.Size() > 500*1024*1024 {
		return false
	}
	
	filename := strings.ToLower(filepath.Base(path))
	
	// Common Claude export filename patterns
	patterns := []string{
		"conversations",
		"claude",
		"export",
		"chat",
		"messages",
	}
	
	for _, pattern := range patterns {
		if strings.Contains(filename, pattern) {
			return true
		}
	}
	
	// If filename has a date pattern and ends with .json, it might be an export
	if strings.Contains(filename, "2024") || strings.Contains(filename, "2023") {
		return true
	}
	
	return false
}

// validateAndPreview checks if the file is a valid Claude export and extracts preview info
func (s *Scanner) validateAndPreview(path string) (bool, string, *ExportPreview) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Sprintf("Cannot open file: %v", err), nil
	}
	defer file.Close()
	
	// Try to parse as JSON array of conversations
	var conversations []models.ClaudeConversation
	decoder := json.NewDecoder(file)
	
	if err := decoder.Decode(&conversations); err != nil {
		return false, fmt.Sprintf("Invalid JSON format: %v", err), nil
	}
	
	if len(conversations) == 0 {
		return false, "No conversations found in export", nil
	}
	
	// Validate structure - check first conversation
	conv := conversations[0]
	if conv.UUID == "" || conv.Name == "" {
		return false, "Invalid conversation structure - missing required fields", nil
	}
	
	// Create preview
	preview := &ExportPreview{
		ConversationCount: len(conversations),
		FirstConvName:     conv.Name,
	}
	
	// Count total messages and find date range
	var messageCount int
	var minDate, maxDate time.Time
	
	for _, c := range conversations {
		messageCount += len(c.ChatMessages)
		
		if convTime, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
			if minDate.IsZero() || convTime.Before(minDate) {
				minDate = convTime
			}
			if maxDate.IsZero() || convTime.After(maxDate) {
				maxDate = convTime
			}
		}
	}
	
	preview.MessageCount = messageCount
	
	if !minDate.IsZero() && !maxDate.IsZero() {
		if minDate.Year() == maxDate.Year() && minDate.Month() == maxDate.Month() {
			preview.DateRange = minDate.Format("Jan 2006")
		} else {
			preview.DateRange = fmt.Sprintf("%s - %s", 
				minDate.Format("Jan 2006"), 
				maxDate.Format("Jan 2006"))
		}
	}
	
	return true, "", preview
}

// GetRecentExports returns exports modified within the specified duration
func (s *Scanner) GetRecentExports(since time.Duration) ([]*ExportFile, error) {
	exports, err := s.ScanForExports()
	if err != nil {
		return nil, err
	}
	
	cutoff := time.Now().Add(-since)
	var recent []*ExportFile
	
	for _, export := range exports {
		if export.ModTime.After(cutoff) {
			recent = append(recent, export)
		}
	}
	
	return recent, nil
}