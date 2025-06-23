package search

import (
	"fmt"
	"strings"

	"github.com/neilberkman/shannon/internal/artifacts"
	"github.com/neilberkman/shannon/internal/models"
)

// ArtifactSearchResult represents a search result for artifacts
type ArtifactSearchResult struct {
	Artifact     *artifacts.Artifact
	Conversation *models.Conversation
	Message      *models.Message
	Snippet      string
}

// SearchArtifacts searches for artifacts containing the query
func (e *Engine) SearchArtifacts(opts SearchOptions) ([]*ArtifactSearchResult, error) {
	// First, find messages that might contain artifacts
	// We'll search for messages containing "antArtifact" tag
	artifactOpts := opts
	if artifactOpts.Query != "" {
		// Combine artifact tag search with user query
		artifactOpts.Query = fmt.Sprintf(`antArtifact AND (%s)`, opts.Query)
	} else {
		artifactOpts.Query = "antArtifact"
	}

	// Get messages that potentially contain artifacts
	searchResults, err := e.Search(artifactOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to search for artifacts: %w", err)
	}

	// Extract artifacts from found messages
	extractor := artifacts.NewExtractor()
	var results []*ArtifactSearchResult

	for _, sr := range searchResults {
		// Create a message from search result
		msg := &models.Message{
			ID:             sr.MessageID,
			UUID:           sr.MessageUUID,
			ConversationID: sr.ConversationID,
			Sender:         sr.Sender,
			Text:           sr.Text,
			CreatedAt:      sr.CreatedAt,
		}

		// Extract artifacts from this message
		messageArtifacts, err := extractor.ExtractFromMessage(msg)
		if err != nil {
			continue // Skip messages that fail extraction
		}

		// Filter artifacts based on original query if provided
		for _, artifact := range messageArtifacts {
			if opts.Query == "" || e.artifactMatchesQuery(artifact, opts.Query) {
				result := &ArtifactSearchResult{
					Artifact: artifact,
					Conversation: &models.Conversation{
						ID:   sr.ConversationID,
						UUID: sr.ConversationUUID,
						Name: sr.ConversationName,
					},
					Message: msg,
					Snippet: e.generateArtifactSnippet(artifact, opts.Query),
				}
				results = append(results, result)
			}
		}
	}

	return results, nil
}

// GetConversationArtifacts extracts all artifacts from a conversation
func (e *Engine) GetConversationArtifacts(conversationID int64) ([]*artifacts.Artifact, error) {
	_, messages, err := e.GetConversation(conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	extractor := artifacts.NewExtractor()
	var allArtifacts []*artifacts.Artifact

	for _, msg := range messages {
		msgArtifacts, err := extractor.ExtractFromMessage(msg)
		if err != nil {
			continue // Skip messages that fail extraction
		}
		allArtifacts = append(allArtifacts, msgArtifacts...)
	}

	return allArtifacts, nil
}

// artifactMatchesQuery checks if an artifact matches the search query
func (e *Engine) artifactMatchesQuery(artifact *artifacts.Artifact, query string) bool {
	// Remove the "antArtifact AND" part we added earlier
	query = strings.TrimPrefix(query, "antArtifact AND (")
	query = strings.TrimSuffix(query, ")")

	// Simple case-insensitive search in artifact content and metadata
	queryLower := strings.ToLower(query)

	// Check title
	if strings.Contains(strings.ToLower(artifact.Title), queryLower) {
		return true
	}

	// Check content
	if strings.Contains(strings.ToLower(artifact.Content), queryLower) {
		return true
	}

	// Check language for code artifacts
	if artifact.Language != "" && strings.Contains(strings.ToLower(artifact.Language), queryLower) {
		return true
	}

	return false
}

// generateArtifactSnippet creates a snippet highlighting the match
func (e *Engine) generateArtifactSnippet(artifact *artifacts.Artifact, query string) string {
	// Remove the artifact search prefix
	query = strings.TrimPrefix(query, "antArtifact AND (")
	query = strings.TrimSuffix(query, ")")

	if query == "" {
		// No specific query, return first few lines
		return artifact.GetPreview(3)
	}

	// Find the query in content and return context around it
	content := artifact.Content
	queryLower := strings.ToLower(query)
	contentLower := strings.ToLower(content)

	index := strings.Index(contentLower, queryLower)
	if index == -1 {
		// Query not found in content, might be in title
		return artifact.GetPreview(3)
	}

	// Extract context around the match
	start := max(0, index-50)
	end := min(len(content), index+len(query)+50)

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}

	return snippet
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
