package models

import (
	"time"
)

// Conversation represents a Claude conversation
type Conversation struct {
	ID           int64     `db:"id"`
	UUID         string    `db:"uuid"`
	Name         string    `db:"name"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	MessageCount int       `db:"message_count"`
	ImportedAt   time.Time `db:"imported_at"`
}

// Message represents a single message in a conversation
type Message struct {
	ID             int64     `db:"id"`
	UUID           string    `db:"uuid"`
	ConversationID int64     `db:"conversation_id"`
	Sender         string    `db:"sender"` // "human" or "assistant"
	Text           string    `db:"text"`
	CreatedAt      time.Time `db:"created_at"`
	ParentID       *int64    `db:"parent_id"` // For branching support
	BranchID       int64     `db:"branch_id"` // To group messages in same branch
	Sequence       int       `db:"sequence"`  // Order within branch
}

// Branch represents a conversation branch
type Branch struct {
	ID             int64     `db:"id"`
	ConversationID int64     `db:"conversation_id"`
	Name           string    `db:"name"`
	ParentBranchID *int64    `db:"parent_branch_id"`
	CreatedAt      time.Time `db:"created_at"`
}

// SearchResult represents a search hit
type SearchResult struct {
	ConversationID   int64
	ConversationUUID string
	ConversationName string
	MessageID        int64
	MessageUUID      string
	Sender           string
	Text             string
	Snippet          string // Highlighted snippet
	CreatedAt        time.Time
	Rank             float64 // Relevance score
}

// ImportStats tracks import statistics
type ImportStats struct {
	ConversationsImported int
	MessagesImported      int
	BranchesDetected      int
	Duration              time.Duration
	Errors                []error
}

// ClaudeExport represents the structure of Claude's JSON export
type ClaudeExport struct {
	Conversations []ClaudeConversation
}

// ClaudeConversation represents a conversation in the export
type ClaudeConversation struct {
	UUID         string              `json:"uuid"`
	Name         string              `json:"name"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	ChatMessages []ClaudeChatMessage `json:"chat_messages"`
}

// ClaudeChatMessage represents a message in the export
type ClaudeChatMessage struct {
	UUID      string                 `json:"uuid"`
	Sender    string                 `json:"sender"`
	Text      string                 `json:"text"`
	Content   []ClaudeMessageContent `json:"content"`
	CreatedAt string                 `json:"created_at"`
	ParentID  *string                `json:"parent_message_uuid,omitempty"`
}

// ClaudeMessageContent represents the content structure
type ClaudeMessageContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
