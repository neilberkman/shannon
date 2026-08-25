package models

import (
	"encoding/json"
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
	MessagesUpdated       int
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
	UUID        string                 `json:"uuid"`
	Sender      string                 `json:"sender"`
	Text        string                 `json:"text"`
	Content     []ClaudeMessageContent `json:"content"`
	Attachments []ClaudeAttachment     `json:"attachments,omitempty"`
	Files       []ClaudeFile           `json:"files,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	ParentID    *string                `json:"parent_message_uuid,omitempty"`
}

// ClaudeAttachment represents a file uploaded alongside a message. Claude's
// export carries the server-extracted text of the file, which is often the
// only place a document's contents appear in the transcript.
type ClaudeAttachment struct {
	FileName         string `json:"file_name"`
	FileType         string `json:"file_type,omitempty"`
	ExtractedContent string `json:"extracted_content,omitempty"`
}

// ClaudeFile represents a file uploaded with a message. Unlike an attachment,
// the export carries no extracted text for it, only the name it was uploaded
// under. That name is frequently the only searchable trace of a document in a
// message whose author typed nothing alongside it.
type ClaudeFile struct {
	FileUUID string `json:"file_uuid,omitempty"`
	FileName string `json:"file_name,omitempty"`
}

// ClaudeMessageContent represents a single content block in an exported
// message. Claude's export nests searchable prose well beyond plain "text"
// blocks: extended thinking, tool inputs, and tool results, the last of which
// contain their own nested blocks (web-search "knowledge" documents, files
// read from disk, and so on).
//
// Content and Input are held as raw JSON because their shapes vary by block
// type and by export vintage. Decoding them lazily keeps a single unexpected
// block from failing the whole conversation.
type ClaudeMessageContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Name     string          `json:"name,omitempty"`
	Title    string          `json:"title,omitempty"`
	URL      string          `json:"url,omitempty"`
	FilePath string          `json:"file_path,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Content  json.RawMessage `json:"content,omitempty"`
}

// NestedContent decodes a tool_result's nested block list. It returns nil when
// the field is absent or is not a block array, so callers can ignore shape
// drift rather than fail the import.
func (c *ClaudeMessageContent) NestedContent() []ClaudeMessageContent {
	if len(c.Content) == 0 {
		return nil
	}
	var blocks []ClaudeMessageContent
	if err := json.Unmarshal(c.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// ToolInput decodes a tool_use block's input object. It returns nil when the
// field is absent or is not an object.
func (c *ClaudeMessageContent) ToolInput() map[string]any {
	if len(c.Input) == 0 {
		return nil
	}
	var in map[string]any
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return nil
	}
	return in
}
