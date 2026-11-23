package search

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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

// DB returns the underlying database connection. This is intended for use in tests.
func (e *Engine) DB() *db.DB {
	return e.db
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

// HasFilters reports whether any non-query filter is active.
func (o SearchOptions) HasFilters() bool {
	return o.ConversationID != nil || o.Sender != "" || o.StartDate != nil || o.EndDate != nil
}

// ConversationSearchResult represents search results grouped and ranked by conversation.
type ConversationSearchResult struct {
	Conversation *models.Conversation
	Matches      []*models.SearchResult
	Score        float64
}

var (
	codePatterns = []*regexp.Regexp{
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
		regexp.MustCompile(`\b\w+\.(js|ts|py|go|rs|cpp|c|h|java|kt|swift|rb|php|cs|scala|clj|hs|ml|elm|dart|vue|jsx|tsx|css|scss|sass|less|html|xml|json|yaml|yml|toml|ini|cfg|conf|sh|bash|zsh)\b`), // file extensions
	}

	// Whole-word technical identifiers that benefit from unstemmed tokenization.
	technicalTerms = map[string]struct{}{
		"api": {}, "json": {}, "xml": {}, "http": {}, "https": {}, "url": {}, "uri": {}, "sql": {},
		"graphql": {}, "grpc": {}, "oauth": {}, "jwt": {}, "regex": {}, "regexp": {}, "sqlite": {},
		"postgresql": {}, "postgres": {}, "mysql": {}, "redis": {}, "mongodb": {}, "docker": {},
		"kubernetes": {}, "k8s": {}, "kubectl": {}, "terraform": {}, "webpack": {}, "eslint": {},
		"tsconfig": {}, "pyproject": {}, "dockerfile": {}, "makefile": {}, "github": {}, "gitlab": {},
	}
)

// Search performs a message-level full-text (or filter-only) search.
func (e *Engine) Search(opts SearchOptions) ([]*models.SearchResult, error) {
	if strings.TrimSpace(opts.Query) == "" && opts.HasFilters() {
		return e.searchFilterOnlyMessages(opts)
	}

	query, args := e.buildSearchQuery(opts)

	rows, err := e.db.Query(query, args...)
	if err != nil {
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
		var createdRaw interface{}
		err := rows.Scan(
			&r.ConversationID,
			&r.ConversationUUID,
			&r.ConversationName,
			&r.MessageID,
			&r.MessageUUID,
			&r.Sender,
			&r.Text,
			&r.Snippet,
			&createdRaw,
			&r.Rank,
			&r.Sequence,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		r.CreatedAt = scanTimeValue(createdRaw)
		if !withinDateRange(r.CreatedAt, opts.StartDate, opts.EndDate) {
			continue
		}
		results = append(results, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Apply offset/limit after Go-side date filtering when date bounds are active.
	if opts.StartDate != nil || opts.EndDate != nil {
		if opts.Offset > 0 {
			if opts.Offset >= len(results) {
				return nil, nil
			}
			results = results[opts.Offset:]
		}
		if opts.Limit > 0 && len(results) > opts.Limit {
			results = results[:opts.Limit]
		}
	}

	return results, nil
}

// SearchConversationsGrouped searches message content and conversation titles,
// groups matches by conversation, and ranks conversations using BM25 relevance,
// match count, title boost, and recency decay (matching ccrider's ranking model).
// Also supports filter-only queries (date range / sender with empty query).
func (e *Engine) SearchConversationsGrouped(opts SearchOptions) ([]*ConversationSearchResult, error) {
	trimmed := strings.TrimSpace(opts.Query)
	if trimmed == "" {
		if !opts.HasFilters() {
			return nil, nil
		}
		return e.filterOnlyConversations(opts)
	}

	// Fetch a generous pool of message hits so grouping & ranking sees all
	// strong candidate conversations rather than starving on one chatty thread.
	msgOpts := opts
	msgOpts.Offset = 0
	if msgOpts.Limit <= 0 || msgOpts.Limit < 500 {
		msgOpts.Limit = 500
	}
	msgResults, err := e.Search(msgOpts)
	if err != nil {
		return nil, err
	}

	convMap := make(map[int64]*ConversationSearchResult)
	var convOrder []int64

	for _, r := range msgResults {
		entry, exists := convMap[r.ConversationID]
		if !exists {
			entry = &ConversationSearchResult{
				Conversation: &models.Conversation{
					ID:   r.ConversationID,
					UUID: r.ConversationUUID,
					Name: r.ConversationName,
				},
			}
			convMap[r.ConversationID] = entry
			convOrder = append(convOrder, r.ConversationID)
		}
		entry.Matches = append(entry.Matches, r)
	}

	// Also merge conversations whose title matches the query (unless restricted
	// to a specific sender, since titles have no sender).
	if opts.Sender == "" && opts.ConversationID == nil {
		titleMatches, err := e.searchConversationTitles(trimmed, 100)
		if err == nil {
			for _, c := range titleMatches {
				if !withinDateRange(c.UpdatedAt, opts.StartDate, opts.EndDate) {
					continue
				}
				if entry, exists := convMap[c.ID]; exists {
					entry.Conversation = c
				} else {
					convMap[c.ID] = &ConversationSearchResult{
						Conversation: c,
					}
					convOrder = append(convOrder, c.ID)
				}
			}
		}
	}

	// Hydrate conversation metadata (created_at, updated_at, message_count) in one query.
	if err := e.hydrateConversations(convMap); err != nil {
		return nil, err
	}

	now := time.Now()
	grouped := make([]*ConversationSearchResult, 0, len(convOrder))
	for _, id := range convOrder {
		entry := convMap[id]
		entry.Score = calculateConversationScore(trimmed, entry, now)
		grouped = append(grouped, entry)
	}

	// Sort conversations.
	sort.SliceStable(grouped, func(i, j int) bool {
		if opts.SortBy == "date" {
			if opts.SortOrder == "asc" {
				return grouped[i].Conversation.UpdatedAt.Before(grouped[j].Conversation.UpdatedAt)
			}
			return grouped[i].Conversation.UpdatedAt.After(grouped[j].Conversation.UpdatedAt)
		}
		if opts.SortOrder == "asc" {
			return grouped[i].Score < grouped[j].Score
		}
		return grouped[i].Score > grouped[j].Score
	})

	if opts.Offset > 0 {
		if opts.Offset >= len(grouped) {
			return nil, nil
		}
		grouped = grouped[opts.Offset:]
	}
	if opts.Limit > 0 && len(grouped) > opts.Limit {
		grouped = grouped[:opts.Limit]
	}

	return grouped, nil
}

// calculateConversationScore mirrors ccrider's composite relevance scoring:
//   - Best BM25 match quality: 0-25 points
//   - Match count: 10 points per match (capped at 10 matches so huge threads don't drown out focused ones)
//   - Title match boost: +50 points when query terms match the conversation name
//   - Recency boost: 0-20 points with exponential decay (1-week constant)
func calculateConversationScore(query string, conv *ConversationSearchResult, now time.Time) float64 {
	score := 0.0

	matchCount := len(conv.Matches)
	if matchCount > 10 {
		matchCount = 10
	}
	score += float64(matchCount) * 10.0

	// FTS5 BM25 rank is negative (more negative = stronger match).
	if len(conv.Matches) > 0 {
		best := 0.0
		for _, m := range conv.Matches {
			if -m.Rank > best {
				best = -m.Rank
			}
		}
		if best > 0 {
			score += 25.0 * (best / (best + 5.0))
		}
	}

	// Title boost: full phrase or all significant query words present in title.
	nameLower := strings.ToLower(conv.Conversation.Name)
	queryLower := strings.ToLower(strings.Trim(query, `"`))
	if queryLower != "" && nameLower != "" {
		if strings.Contains(nameLower, queryLower) {
			score += 50.0
		} else {
			words := extractContentWords(queryLower)
			if len(words) > 0 {
				matched := 0
				for _, w := range words {
					if strings.Contains(nameLower, w) {
						matched++
					}
				}
				if matched == len(words) {
					score += 40.0
				} else if matched > 0 {
					score += 15.0 * float64(matched) / float64(len(words))
				}
			}
		}
	}

	// Recency boost: 0-20 points with 1-week decay constant.
	ref := conv.Conversation.UpdatedAt
	if ref.IsZero() && len(conv.Matches) > 0 {
		ref = conv.Matches[0].CreatedAt
	}
	if !ref.IsZero() {
		ageHours := now.Sub(ref).Hours()
		if ageHours < 1 {
			ageHours = 1
		}
		score += 20.0 * math.Exp(-ageHours/168.0)
	}

	return score
}

func extractContentWords(queryLower string) []string {
	var words []string
	for _, tok := range strings.Fields(queryLower) {
		tok = strings.Trim(tok, `"'*`)
		if tok == "" || tok == "and" || tok == "or" || tok == "not" {
			continue
		}
		words = append(words, tok)
	}
	return words
}

func (e *Engine) searchConversationTitles(query string, limit int) ([]*models.Conversation, error) {
	words := extractContentWords(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}

	var conds []string
	var args []interface{}
	for _, w := range words {
		conds = append(conds, "LOWER(name) LIKE ?")
		args = append(args, "%"+w+"%")
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT ?
	`, strings.Join(conds, " AND "))

	rows, err := e.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*models.Conversation
	for rows.Next() {
		var c models.Conversation
		var createdRaw, updatedRaw, importedRaw interface{}
		if err := rows.Scan(&c.ID, &c.UUID, &c.Name, &createdRaw, &updatedRaw, &c.MessageCount, &importedRaw); err != nil {
			return nil, err
		}
		c.CreatedAt = scanTimeValue(createdRaw)
		c.UpdatedAt = scanTimeValue(updatedRaw)
		c.ImportedAt = scanTimeValue(importedRaw)
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (e *Engine) hydrateConversations(convMap map[int64]*ConversationSearchResult) error {
	var missing []interface{}
	var placeholders []string
	for id, entry := range convMap {
		if entry.Conversation.UpdatedAt.IsZero() || entry.Conversation.MessageCount == 0 {
			missing = append(missing, id)
			placeholders = append(placeholders, "?")
		}
	}
	if len(missing) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := e.db.Query(query, missing...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var c models.Conversation
		var createdRaw, updatedRaw, importedRaw interface{}
		if err := rows.Scan(&c.ID, &c.UUID, &c.Name, &createdRaw, &updatedRaw, &c.MessageCount, &importedRaw); err != nil {
			return err
		}
		c.CreatedAt = scanTimeValue(createdRaw)
		c.UpdatedAt = scanTimeValue(updatedRaw)
		c.ImportedAt = scanTimeValue(importedRaw)
		if entry, ok := convMap[c.ID]; ok {
			entry.Conversation = &c
		}
	}
	return rows.Err()
}

func (e *Engine) searchFilterOnlyMessages(opts SearchOptions) ([]*models.SearchResult, error) {
	var conditions []string
	var args []interface{}

	if opts.ConversationID != nil {
		conditions = append(conditions, "m.conversation_id = ?")
		args = append(args, *opts.ConversationID)
	}
	if opts.Sender != "" {
		conditions = append(conditions, "m.sender = ?")
		args = append(args, opts.Sender)
	}

	query := `
		SELECT
			c.id,
			c.uuid,
			c.name,
			m.id,
			m.uuid,
			m.sender,
			m.text,
			substr(m.text, 1, 200) as snippet,
			m.created_at,
			0.0 as rank,
			m.sequence
		FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	if opts.SortOrder == "asc" {
		query += " ORDER BY m.created_at ASC"
	} else {
		query += " ORDER BY m.created_at DESC"
	}
	if opts.StartDate == nil && opts.EndDate == nil && opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	} else {
		query += " LIMIT 2000"
	}

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("filter search failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.SearchResult
	for rows.Next() {
		var r models.SearchResult
		var createdRaw interface{}
		if err := rows.Scan(
			&r.ConversationID,
			&r.ConversationUUID,
			&r.ConversationName,
			&r.MessageID,
			&r.MessageUUID,
			&r.Sender,
			&r.Text,
			&r.Snippet,
			&createdRaw,
			&r.Rank,
			&r.Sequence,
		); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		r.CreatedAt = scanTimeValue(createdRaw)
		if !withinDateRange(r.CreatedAt, opts.StartDate, opts.EndDate) {
			continue
		}
		results = append(results, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if opts.StartDate != nil || opts.EndDate != nil {
		if opts.Offset > 0 {
			if opts.Offset >= len(results) {
				return nil, nil
			}
			results = results[opts.Offset:]
		}
		if opts.Limit > 0 && len(results) > opts.Limit {
			results = results[:opts.Limit]
		}
	}
	return results, nil
}

func (e *Engine) filterOnlyConversations(opts SearchOptions) ([]*ConversationSearchResult, error) {
	// If filtering by sender or specific conversation, filter via messages so we
	// include representative matching snippets.
	if opts.Sender != "" || opts.ConversationID != nil {
		msgOpts := opts
		msgOpts.Limit = 1000
		msgOpts.Offset = 0
		msgs, err := e.searchFilterOnlyMessages(msgOpts)
		if err != nil {
			return nil, err
		}
		convMap := make(map[int64]*ConversationSearchResult)
		var order []int64
		for _, r := range msgs {
			entry, ok := convMap[r.ConversationID]
			if !ok {
				entry = &ConversationSearchResult{
					Conversation: &models.Conversation{
						ID:   r.ConversationID,
						UUID: r.ConversationUUID,
						Name: r.ConversationName,
					},
				}
				convMap[r.ConversationID] = entry
				order = append(order, r.ConversationID)
			}
			entry.Matches = append(entry.Matches, r)
		}
		if err := e.hydrateConversations(convMap); err != nil {
			return nil, err
		}
		out := make([]*ConversationSearchResult, 0, len(order))
		for _, id := range order {
			out = append(out, convMap[id])
		}
		sort.SliceStable(out, func(i, j int) bool {
			if opts.SortOrder == "asc" {
				return out[i].Conversation.UpdatedAt.Before(out[j].Conversation.UpdatedAt)
			}
			return out[i].Conversation.UpdatedAt.After(out[j].Conversation.UpdatedAt)
		})
		if opts.Offset > 0 {
			if opts.Offset >= len(out) {
				return nil, nil
			}
			out = out[opts.Offset:]
		}
		if opts.Limit > 0 && len(out) > opts.Limit {
			out = out[:opts.Limit]
		}
		return out, nil
	}

	// Date-only filter over conversations.
	convs, err := e.GetRecentConversations(0, opts.StartDate, opts.EndDate)
	if err != nil {
		return nil, err
	}
	if opts.SortOrder == "asc" {
		sort.SliceStable(convs, func(i, j int) bool {
			return convs[i].UpdatedAt.Before(convs[j].UpdatedAt)
		})
	}
	if opts.Offset > 0 {
		if opts.Offset >= len(convs) {
			return nil, nil
		}
		convs = convs[opts.Offset:]
	}
	if opts.Limit > 0 && len(convs) > opts.Limit {
		convs = convs[:opts.Limit]
	}

	out := make([]*ConversationSearchResult, 0, len(convs))
	for _, c := range convs {
		out = append(out, &ConversationSearchResult{Conversation: c})
	}
	return out, nil
}

func (e *Engine) buildSearchQuery(opts SearchOptions) (string, []interface{}) {
	var conditions []string
	var args []interface{}

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
			rank,
			m.sequence
		FROM %s
		JOIN messages m ON %s.rowid = m.id
		JOIN conversations c ON m.conversation_id = c.id
		WHERE %s MATCH ?
	`, ftsTable, ftsTable, ftsTable, ftsTable)

	// Process search query for FTS5
	ftsQuery := e.processFTSQuery(opts.Query)
	args = append(args, ftsQuery)

	// Add additional filters
	if opts.ConversationID != nil {
		conditions = append(conditions, "m.conversation_id = ?")
		args = append(args, *opts.ConversationID)
	}

	if opts.Sender != "" {
		conditions = append(conditions, "m.sender = ?")
		args = append(args, opts.Sender)
	}

	// Build final query
	query := baseQuery
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add sorting.
	// Note: SQLite FTS5 BM25 rank is negative (more negative = better match),
	// so "relevance desc" (best first) orders by rank ASC.
	switch opts.SortBy {
	case "date":
		if opts.SortOrder == "asc" {
			query += " ORDER BY m.created_at ASC"
		} else {
			query += " ORDER BY m.created_at DESC"
		}
	default: // relevance
		if opts.SortOrder == "asc" {
			query += " ORDER BY rank DESC"
		} else {
			query += " ORDER BY rank ASC"
		}
	}

	// Only push LIMIT/OFFSET to SQL when no Go-side date filter is active;
	// otherwise fetch a generous pool and apply pagination after date filtering.
	if opts.StartDate == nil && opts.EndDate == nil {
		if opts.Limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", opts.Limit)
			if opts.Offset > 0 {
				query += fmt.Sprintf(" OFFSET %d", opts.Offset)
			}
		}
	} else {
		query += " LIMIT 2000"
	}

	return query, args
}

// processFTSQuery converts a user query to safe FTS5 syntax while preserving
// quoted phrases, prefix wildcards (foo*), and case-insensitive boolean
// operators (AND, OR, NOT). Tokens containing FTS5 special punctuation (such
// as hyphens, colons, dots, slashes, or @) are safely quoted so they never
// trigger FTS5 syntax errors.
func (e *Engine) processFTSQuery(userQuery string) string {
	query := strings.TrimSpace(userQuery)
	if query == "" {
		return `""`
	}

	// Unbalanced quotes: treat the whole query as a single literal phrase.
	if strings.Count(query, `"`)%2 != 0 {
		return escapeFTSQuery(query)
	}

	tokens := splitPreservingQuotes(query)
	if len(tokens) == 0 {
		return `""`
	}

	var out []string
	prevWasOp := true // start-of-query behaves like after an operator
	for i, tok := range tokens {
		upper := strings.ToUpper(tok)

		// Treat standalone AND / OR / NOT between terms as boolean operators.
		if !strings.HasPrefix(tok, `"`) && (upper == "AND" || upper == "OR" || upper == "NOT") {
			if !prevWasOp && i < len(tokens)-1 {
				out = append(out, upper)
				prevWasOp = true
				continue
			}
		}

		// Insert implicit AND between consecutive terms without an explicit operator.
		if !prevWasOp {
			out = append(out, "AND")
		}

		out = append(out, formatFTSToken(tok))
		prevWasOp = false
	}

	if len(out) == 0 {
		return `""`
	}
	return strings.Join(out, " ")
}

// formatFTSToken formats a single search token for FTS5 MATCH.
// Quoted phrases are preserved, trailing * wildcards stay outside quotes, and
// tokens with FTS5 punctuation are quoted so FTS5 never rejects them.
func formatFTSToken(tok string) string {
	if strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`) && len(tok) >= 2 {
		return tok
	}

	hasWildcard := strings.HasSuffix(tok, "*") && len(tok) > 1
	core := tok
	if hasWildcard {
		core = tok[:len(tok)-1]
	}

	if needsFTSQuoting(core) {
		escaped := `"` + strings.ReplaceAll(core, `"`, `""`) + `"`
		if hasWildcard {
			return escaped + "*"
		}
		return escaped
	}

	return tok
}

func needsFTSQuoting(tok string) bool {
	if tok == "" {
		return true
	}
	// Reserved FTS5 keywords used as literal terms (e.g., at start/end of query).
	switch strings.ToUpper(tok) {
	case "AND", "OR", "NOT", "NEAR":
		return true
	}
	for _, r := range tok {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return true
		}
	}
	return false
}

// escapeFTSQuery escapes special characters for FTS5 by wrapping the query as a literal phrase.
func escapeFTSQuery(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// isCodeQuery determines if a query should use the code-specific FTS table.
func (e *Engine) isCodeQuery(query string) bool {
	for _, pattern := range codePatterns {
		if pattern.MatchString(query) {
			return true
		}
	}

	// Whole-word check only, so words like "science" or "capital" stay on the
	// porter-stemmed natural language index.
	for _, word := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if _, ok := technicalTerms[word]; ok {
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
		var createdRaw, updatedRaw, importedRaw interface{}
		err := rows.Scan(&c.ID, &c.UUID, &c.Name, &createdRaw, &updatedRaw, &c.MessageCount, &importedRaw)
		if err != nil {
			return nil, err
		}
		c.CreatedAt = scanTimeValue(createdRaw)
		c.UpdatedAt = scanTimeValue(updatedRaw)
		c.ImportedAt = scanTimeValue(importedRaw)
		conversations = append(conversations, &c)
	}

	return conversations, rows.Err()
}

// GetConversationByUUID retrieves a conversation and its main-branch messages by UUID.
func (e *Engine) GetConversationByUUID(uuid string) (*models.Conversation, []*models.Message, error) {
	var id int64
	err := e.db.QueryRow(`SELECT id FROM conversations WHERE uuid = ?`, uuid).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("conversation %s not found", uuid)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lookup failed: %w", err)
	}
	return e.GetConversation(id)
}

// GetConversation retrieves a full conversation with all messages
func (e *Engine) GetConversation(conversationID int64) (*models.Conversation, []*models.Message, error) {
	// Get conversation
	var conv models.Conversation
	var createdRaw, updatedRaw, importedRaw interface{}
	err := e.db.QueryRow(`
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		WHERE id = ?
	`, conversationID).Scan(&conv.ID, &conv.UUID, &conv.Name, &createdRaw, &updatedRaw, &conv.MessageCount, &importedRaw)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("conversation not found")
		}
		return nil, nil, err
	}
	conv.CreatedAt = scanTimeValue(createdRaw)
	conv.UpdatedAt = scanTimeValue(updatedRaw)
	conv.ImportedAt = scanTimeValue(importedRaw)

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
		var msgCreatedRaw interface{}
		err := rows.Scan(&m.ID, &m.UUID, &m.ConversationID, &m.Sender, &m.Text, &msgCreatedRaw, &m.ParentID, &m.BranchID, &m.Sequence)
		if err != nil {
			return nil, nil, err
		}
		m.CreatedAt = scanTimeValue(msgCreatedRaw)
		messages = append(messages, &m)
	}

	return &conv, messages, rows.Err()
}

// GetRecentConversations returns conversations ordered by updated_at DESC,
// optionally filtered by startDate / endDate.
func (e *Engine) GetRecentConversations(limit int, startDate, endDate *time.Time) ([]*models.Conversation, error) {
	query := `
		SELECT id, uuid, name, created_at, updated_at, message_count, imported_at
		FROM conversations
		ORDER BY updated_at DESC
	`
	var args []interface{}
	if startDate == nil && endDate == nil && limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var conversations []*models.Conversation
	for rows.Next() {
		var c models.Conversation
		var createdRaw, updatedRaw, importedRaw interface{}
		if err := rows.Scan(&c.ID, &c.UUID, &c.Name, &createdRaw, &updatedRaw, &c.MessageCount, &importedRaw); err != nil {
			return nil, err
		}
		c.CreatedAt = scanTimeValue(createdRaw)
		c.UpdatedAt = scanTimeValue(updatedRaw)
		c.ImportedAt = scanTimeValue(importedRaw)
		if !withinDateRange(c.UpdatedAt, startDate, endDate) {
			continue
		}
		conversations = append(conversations, &c)
		if limit > 0 && len(conversations) >= limit {
			break
		}
	}
	return conversations, rows.Err()
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
		oldest := parseDBTimestamp(oldestStr.String)
		newest := parseDBTimestamp(newestStr.String)
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
		var createdRaw, updatedRaw, importedRaw interface{}
		err := rows.Scan(&c.ID, &c.UUID, &c.Name, &createdRaw, &updatedRaw, &c.MessageCount, &importedRaw)
		if err != nil {
			return nil, err
		}
		c.CreatedAt = scanTimeValue(createdRaw)
		c.UpdatedAt = scanTimeValue(updatedRaw)
		c.ImportedAt = scanTimeValue(importedRaw)
		conversations = append(conversations, &c)
	}

	return conversations, rows.Err()
}

func scanTimeValue(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		return parseDBTimestamp(t)
	case []byte:
		return parseDBTimestamp(string(t))
	default:
		return time.Time{}
	}
}

func withinDateRange(t time.Time, start, end *time.Time) bool {
	if start != nil && (t.IsZero() || t.Before(*start)) {
		return false
	}
	if end != nil && (t.IsZero() || t.After(*end)) {
		return false
	}
	return true
}
