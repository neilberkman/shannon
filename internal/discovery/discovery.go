package discovery

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/pkg/platform"
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
		// Normalize to absolute path
		if absPath, err := filepath.Abs(downloadsDir); err == nil {
			scanner.searchPaths = append(scanner.searchPaths, absPath)
		} else {
			scanner.searchPaths = append(scanner.searchPaths, downloadsDir)
		}
	}

	// Add browser-specific download locations
	scanner.addBrowserDownloadPaths()

	return scanner
}

// addBrowserDownloadPaths adds common browser-specific download directories
func (s *Scanner) addBrowserDownloadPaths() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Common additional download locations
	additionalPaths := []string{
		// Many users save to Desktop
		filepath.Join(home, "Desktop"),
		// Some Windows users might have Downloads in Documents
		filepath.Join(home, "Documents", "Downloads"),
	}

	// Add paths that exist
	for _, path := range additionalPaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			// Resolve to absolute path to handle case-insensitive filesystems and symlinks
			absPath, err := filepath.Abs(path)
			if err != nil {
				continue
			}

			// Check if we already have this path (comparing absolute paths)
			duplicate := false
			for _, existing := range s.searchPaths {
				existingAbs, _ := filepath.Abs(existing)
				if existingAbs == absPath {
					duplicate = true
					break
				}
			}
			if !duplicate {
				s.searchPaths = append(s.searchPaths, absPath)
			}
		}
	}
}

// AddSearchPath adds an additional directory to search
func (s *Scanner) AddSearchPath(path string) {
	s.searchPaths = append(s.searchPaths, path)
}

// GetSearchPaths returns the list of paths that will be searched
func (s *Scanner) GetSearchPaths() []string {
	return s.searchPaths
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

	// First, look for conversations.json directly in the directory
	convPath := filepath.Join(dir, "conversations.json")
	if info, err := os.Stat(convPath); err == nil && !info.IsDir() {
		if s.isLikelyClaudeExport(convPath, info) {
			export := &ExportFile{
				Path:    convPath,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			export.IsValid, export.ErrorMessage, export.Preview = s.validateAndPreview(convPath)
			exports = append(exports, export)
		}
	}

	// Then, look for data-YYYY* directories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return exports, nil // Return what we have so far
	}

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			// Check if this is a data export directory (data-YYYY-MM-DD-HH-MM-SS format)
			if strings.HasPrefix(name, "data-20") || strings.HasPrefix(name, "data-19") {
				// Look for conversations.json inside this directory
				subPath := filepath.Join(dir, name, "conversations.json")
				if info, err := os.Stat(subPath); err == nil && !info.IsDir() {
					export := &ExportFile{
						Path:    subPath,
						Size:    info.Size(),
						ModTime: info.ModTime(),
					}
					export.IsValid, export.ErrorMessage, export.Preview = s.validateAndPreview(subPath)
					exports = append(exports, export)
				}
			}
		} else {
			// Check if this is a zip file that might contain Claude exports
			if strings.HasSuffix(strings.ToLower(name), ".zip") &&
				(strings.Contains(name, "data-20") || strings.Contains(name, "claude") ||
					strings.Contains(name, "export") || strings.Contains(name, "conversations")) {
				zipPath := filepath.Join(dir, name)
				if zipExports := s.scanZipFile(zipPath); len(zipExports) > 0 {
					exports = append(exports, zipExports...)
				}
			}
		}
	}

	return exports, nil
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
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close file %s: %v\n", path, err)
		}
	}()

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
	if conv.UUID == "" {
		return false, "Invalid conversation structure - missing UUID", nil
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

// scanZipFile looks for Claude export files inside a zip archive
func (s *Scanner) scanZipFile(zipPath string) []*ExportFile {
	var exports []*ExportFile

	// Get file info for the zip
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return exports
	}

	// Open the zip file
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return exports
	}
	defer func() {
		if err := reader.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close zip reader: %v\n", err)
		}
	}()

	// Look for conversations.json files in the zip
	for _, file := range reader.File {
		// Check if this is a conversations.json file
		if filepath.Base(file.Name) == "conversations.json" {
			// Validate the file inside the zip
			isValid, errorMsg, preview := s.validateZipEntry(file)

			export := &ExportFile{
				Path:         fmt.Sprintf("%s!%s", zipPath, file.Name), // Use ! to indicate file inside zip
				Size:         int64(file.UncompressedSize64),
				ModTime:      zipInfo.ModTime(), // Use zip file's mod time
				IsValid:      isValid,
				ErrorMessage: errorMsg,
				Preview:      preview,
			}

			exports = append(exports, export)
		}
	}

	return exports
}

// validateZipEntry validates a conversations.json file inside a zip archive
func (s *Scanner) validateZipEntry(file *zip.File) (bool, string, *ExportPreview) {
	// Open the file inside the zip
	reader, err := file.Open()
	if err != nil {
		return false, fmt.Sprintf("Cannot open file in zip: %v", err), nil
	}
	defer func() {
		_ = reader.Close() // Best effort close for zip entries
	}()

	// Try to parse as JSON array of conversations
	var conversations []models.ClaudeConversation
	decoder := json.NewDecoder(reader)

	if err := decoder.Decode(&conversations); err != nil {
		return false, fmt.Sprintf("Invalid JSON format: %v", err), nil
	}

	if len(conversations) == 0 {
		return false, "No conversations found in export", nil
	}

	// Validate structure - check first conversation
	conv := conversations[0]
	if conv.UUID == "" {
		return false, "Invalid conversation structure - missing UUID", nil
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
