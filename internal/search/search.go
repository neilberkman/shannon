package search

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/shannon/internal/db"
	"github.com/user/shannon/internal/models"
)

// Engine handles search operations
type Engine struct {
	db *db.DB
}

// NewEngine creates a new search engine
func NewEngine(database *db.DB) *Engine {
	return &Engine{db: database}
}

// SearchOptions contains search parameters
type SearchOptions struct {
	Query          string
	ConversationID *int64
	Sender         string // "human", "assistant", or empty for both
	StartDate      *time.Time
	EndDate        *time.Time
	Limit          int
	Offset         int
	SortBy         string // "relevance" or "date"
	SortOrder      string // "asc" or "desc"
}

// Search performs a full-text search
func (e *Engine) Search(opts SearchOptions) ([]*models.SearchResult, error) {
	// Build the query
	query, args := e.buildSearchQuery(opts)

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []*models.SearchResult
	for rows.Next() {
		var r models.SearchResult
		err := rows.Scan(
			&r.ConversationID,
			&r.ConversationUUID,
			&r.ConversationName,
			&r.MessageID,
			&r.MessageUUID,
			&r.Sender,
			&r.Text,
			&r.Snippet,
			&r.CreatedAt,
			&r.Rank,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, &r)
	}

	return results, rows.Err()
}

func (e *Engine) buildSearchQuery(opts SearchOptions) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Base query with FTS5
	baseQuery := `
		SELECT 
			c.id,
			c.uuid,
			c.name,
			m.id,
			m.uuid,
			m.sender,
			m.text,
			snippet(messages_fts, 0, '<mark>', '</mark>', '...', 32) as snippet,
			m.created_at,
			rank
		FROM messages_fts
		JOIN messages m ON messages_fts.rowid = m.id
		JOIN conversations c ON m.conversation_id = c.id
		WHERE messages_fts MATCH ?
	`

	// Process search query for FTS5
	ftsQuery := e.processFTSQuery(opts.Query)
	args = append(args, ftsQuery)
	argIndex++

	// Add additional filters
	if opts.ConversationID != nil {
		conditions = append(conditions, fmt.Sprintf("m.conversation_id = $%d", argIndex))
		args = append(args, *opts.ConversationID)
		argIndex++
	}

	if opts.Sender != "" {
		conditions = append(conditions, fmt.Sprintf("m.sender = $%d", argIndex))
		args = append(args, opts.Sender)
		argIndex++
	}

	if opts.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("m.created_at >= $%d", argIndex))
		args = append(args, *opts.StartDate)
		argIndex++
	}

	if opts.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("m.created_at <= $%d", argIndex))
		args = append(args, *opts.EndDate)
	}

	// Build final query
	query := baseQuery
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add sorting
	switch opts.SortBy {
	case "date":
		query += " ORDER BY m.created_at"
	default: // relevance
		query += " ORDER BY rank"
	}

	if opts.SortOrder == "asc" {
		query += " ASC"
	} else {
		query += " DESC"
	}

	// Add pagination
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	return query, args
}

// processFTSQuery converts user query to FTS5 syntax
func (e *Engine) processFTSQuery(userQuery string) string {
	// Handle special characters and operators
	query := strings.TrimSpace(userQuery)

	// If query contains FTS5 operators, return as-is
	if strings.ContainsAny(query, `"*-+`) {
		return query
	}

	// Otherwise, treat as phrase search for multi-word queries
	if strings.Contains(query, " ") {
		return fmt.Sprintf(`"%s"`, query)
	}

	return query
}

// SearchConversations searches conversation titles
func (e *Engine) SearchConversations(query string, limit int) ([]*models.Conversation, error) {
	sqlQuery := `
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		WHERE name LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`

	rows, err := e.db.Query(sqlQuery, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*models.Conversation
	for rows.Next() {
		var c models.Conversation
		err := rows.Scan(&c.ID, &c.UUID, &c.Name, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount, &c.ImportedAt)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, &c)
	}

	return conversations, rows.Err()
}

// GetConversation retrieves a full conversation with all messages
func (e *Engine) GetConversation(conversationID int64) (*models.Conversation, []*models.Message, error) {
	// Get conversation
	var conv models.Conversation
	err := e.db.QueryRow(`
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		WHERE id = ?
	`, conversationID).Scan(&conv.ID, &conv.UUID, &conv.Name, &conv.CreatedAt, &conv.UpdatedAt, &conv.MessageCount, &conv.ImportedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("conversation not found")
		}
		return nil, nil, err
	}

	// Get messages
	rows, err := e.db.Query(`
		SELECT id, uuid, conversation_id, sender, text, created_at, parent_id, branch_id, sequence
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
	`, conversationID)

	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var messages []*models.Message
	for rows.Next() {
		var m models.Message
		err := rows.Scan(&m.ID, &m.UUID, &m.ConversationID, &m.Sender, &m.Text, &m.CreatedAt, &m.ParentID, &m.BranchID, &m.Sequence)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, &m)
	}

	return &conv, messages, rows.Err()
}

// GetStats returns database statistics
func (e *Engine) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total conversations
	var totalConversations int
	err := e.db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&totalConversations)
	if err != nil {
		return nil, err
	}
	stats["total_conversations"] = totalConversations

	// Total messages
	var totalMessages int
	err = e.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&totalMessages)
	if err != nil {
		return nil, err
	}
	stats["total_messages"] = totalMessages

	// Messages by sender
	var humanCount, assistantCount int
	err = e.db.QueryRow("SELECT COUNT(*) FROM messages WHERE sender = 'human'").Scan(&humanCount)
	if err != nil {
		return nil, err
	}
	err = e.db.QueryRow("SELECT COUNT(*) FROM messages WHERE sender = 'assistant'").Scan(&assistantCount)
	if err != nil {
		return nil, err
	}

	stats["messages_by_sender"] = map[string]int{
		"human":     humanCount,
		"assistant": assistantCount,
	}

	// Date range
	var oldestStr, newestStr sql.NullString
	err = e.db.QueryRow("SELECT MIN(created_at), MAX(created_at) FROM messages").Scan(&oldestStr, &newestStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if oldestStr.Valid && newestStr.Valid {
		// Try multiple date formats
		formats := []string{
			"2006-01-02 15:04:05.999999 -0700 MST",
			"2006-01-02 15:04:05",
			time.RFC3339,
		}

		var oldest, newest time.Time
		for _, format := range formats {
			if t, err := time.Parse(format, oldestStr.String); err == nil {
				oldest = t
				break
			}
		}
		for _, format := range formats {
			if t, err := time.Parse(format, newestStr.String); err == nil {
				newest = t
				break
			}
		}

		if !oldest.IsZero() && !newest.IsZero() {
			stats["date_range"] = map[string]time.Time{
				"oldest": oldest,
				"newest": newest,
			}
		}
	}

	return stats, nil
}

// GetAllConversations retrieves all conversations with pagination
func (e *Engine) GetAllConversations(limit, offset int) ([]*models.Conversation, error) {
	rows, err := e.db.Query(`
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*models.Conversation
	for rows.Next() {
		var conv models.Conversation
		err := rows.Scan(
			&conv.ID,
			&conv.UUID,
			&conv.Name,
			&conv.CreatedAt,
			&conv.UpdatedAt,
			&conv.MessageCount,
			&conv.ImportedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, &conv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversations: %w", err)
	}

	return conversations, nil
}
