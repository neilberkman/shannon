package search

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
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
		// Provide more helpful error messages
		errStr := err.Error()
		if strings.Contains(errStr, "syntax error") {
			return nil, fmt.Errorf("invalid search syntax: %s", opts.Query)
		}
		if strings.Contains(errStr, "unknown special query") {
			return nil, fmt.Errorf("invalid wildcard usage in: %s (hint: wildcards must not be quoted)", opts.Query)
		}
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

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

	// Determine which FTS table to use based on query characteristics
	useCodeTable := e.isCodeQuery(opts.Query)
	ftsTable := "messages_fts"
	if useCodeTable {
		ftsTable = "messages_fts_code"
	}

	// Base query with dynamic FTS table selection
	baseQuery := fmt.Sprintf(`
		SELECT 
			c.id,
			c.uuid,
			c.name,
			m.id,
			m.uuid,
			m.sender,
			m.text,
			snippet(%s, 0, '<mark>', '</mark>', '...', 32) as snippet,
			m.created_at,
			rank
		FROM %s
		JOIN messages m ON %s.rowid = m.id
		JOIN conversations c ON m.conversation_id = c.id
		WHERE %s MATCH ?
	`, ftsTable, ftsTable, ftsTable, ftsTable)

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
		args = append(args, opts.StartDate.Format("2006-01-02 15:04:05"))
		argIndex++
	}

	if opts.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("m.created_at <= $%d", argIndex))
		args = append(args, opts.EndDate.Format("2006-01-02 15:04:05"))
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
	
	// Empty query check
	if query == "" {
		return `""`
	}

	// If query already contains FTS5 operators or quotes, validate and return
	if strings.ContainsAny(query, `"*`) {
		// Basic validation - ensure quotes are balanced
		quoteCount := strings.Count(query, `"`)
		if quoteCount%2 != 0 {
			// Unbalanced quotes - escape the whole query
			return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
		}
		return query
	}

	// Check for explicit boolean operators (AND, OR, NOT) - case insensitive
	upperQuery := strings.ToUpper(query)
	if strings.Contains(upperQuery, " AND ") || strings.Contains(upperQuery, " OR ") || strings.Contains(upperQuery, " NOT ") {
		// Convert to uppercase operators for FTS5
		query = strings.ReplaceAll(query, " and ", " AND ")
		query = strings.ReplaceAll(query, " or ", " OR ")
		query = strings.ReplaceAll(query, " not ", " NOT ")
		return query
	}

	// For multi-word queries without explicit operators, treat as implicit AND
	// This is more intuitive behavior - searching "machine learning" finds documents with both words
	if strings.Contains(query, " ") {
		// Split on spaces and join with AND
		words := strings.Fields(query)
		return strings.Join(words, " AND ")
	}

	return query
}

// escapeFTSQuery escapes special characters for FTS5
func escapeFTSQuery(query string) string {
	// FTS5 special characters that need escaping when not used as operators
	// We'll wrap the query in quotes to treat it as a phrase
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// isCodeQuery determines if a query should use the code-specific FTS table
func (e *Engine) isCodeQuery(query string) bool {
	// Patterns that indicate code-related searches
	codePatterns := []*regexp.Regexp{
		regexp.MustCompile(`[a-z][A-Z]`),            // camelCase
		regexp.MustCompile(`[A-Z][a-z]+[A-Z]`),      // PascalCase
		regexp.MustCompile(`\w+_\w+`),               // snake_case
		regexp.MustCompile(`\w+\.\w+`),              // method.calls or file.ext
		regexp.MustCompile(`\w+::\w+`),              // namespace::function
		regexp.MustCompile(`\w+\(\)`),               // function()
		regexp.MustCompile(`\w+\[\]`),               // array[]
		regexp.MustCompile(`[{}()\[\]<>]`),          // brackets/braces
		regexp.MustCompile(`[=!<>]=?`),              // operators
		regexp.MustCompile(`\+\+|--|&&|\|\||->|=>`), // compound operators
		regexp.MustCompile(`\b(def|function|class|import|export|const|let|var|if|else|for|while|return|async|await|interface|type|struct|enum)\b`), // keywords
		regexp.MustCompile(`\b[A-Z_][A-Z0-9_]{2,}\b`), // CONSTANTS
		regexp.MustCompile(`#\w+`),                    // #hashtags or CSS/preprocessor
		regexp.MustCompile(`\$\w+`),                   // $variables
		regexp.MustCompile(`@\w+`),                    // @decorators
		regexp.MustCompile(`\\\w+`),                   // \commands
		regexp.MustCompile(`\b\w+\.(js|ts|py|go|rs|cpp|c|h|java|kt|swift|rb|php|cs|scala|clj|hs|ml|elm|dart|vue|jsx|tsx|css|scss|sass|less|html|xml|json|yaml|yml|toml|ini|cfg|conf|sh|bash|zsh|fish|ps1|bat|cmd|sql|md|rst|tex|r|m|pl|lua|vim|emacs)\b`), // file extensions
	}

	// Check if query matches any code patterns
	for _, pattern := range codePatterns {
		if pattern.MatchString(query) {
			return true
		}
	}

	// Check for technical terms that commonly appear in code discussions
	technicalTerms := []string{
		"api", "json", "xml", "http", "https", "url", "uri", "sql", "database", "db",
		"frontend", "backend", "fullstack", "devops", "ci", "cd", "git", "github", "gitlab",
		"docker", "kubernetes", "aws", "azure", "gcp", "serverless", "microservice",
		"framework", "library", "package", "dependency", "npm", "pip", "cargo", "maven",
		"compiler", "interpreter", "runtime", "virtual", "container", "deployment",
		"authentication", "authorization", "oauth", "jwt", "token", "session", "cookie",
		"cache", "redis", "mongodb", "postgresql", "mysql", "sqlite", "nosql",
		"async", "sync", "promise", "callback", "event", "listener", "handler",
		"component", "module", "service", "controller", "model", "view", "template",
		"regex", "regexp", "pattern", "match", "parse", "serialize", "deserialize",
		"algorithm", "optimization", "performance", "benchmark", "profiling", "debug",
		"test", "unit", "integration", "e2e", "mock", "stub", "fixture", "spec",
		"build", "compile", "transpile", "bundle", "minify", "lint", "format",
		"version", "release", "deploy", "staging", "production", "environment",
	}

	queryLower := strings.ToLower(query)
	for _, term := range technicalTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}

	return false
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
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

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

	// Get messages from main branch only (for consistent conversation view)
	rows, err := e.db.Query(`
		SELECT m.id, m.uuid, m.conversation_id, m.sender, m.text, m.created_at, m.parent_id, m.branch_id, m.sequence
		FROM messages m
		JOIN branches b ON m.branch_id = b.id
		WHERE m.conversation_id = ? AND b.name = 'main'
		ORDER BY m.sequence ASC, m.created_at ASC
	`, conversationID)

	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

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
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", err)
		}
	}()

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
