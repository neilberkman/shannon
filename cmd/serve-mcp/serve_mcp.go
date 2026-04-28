package servemcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/search"
)

// MaxResponseTokens is the response budget. Claude Code warns at 10k tokens
// and hard-rejects at 25k. Targeting 9k leaves headroom. Token estimate
// approximates len(json) / 4, which is conservative for mixed JSON/English.
const MaxResponseTokens = 9000

func maxResponseBytes() int { return MaxResponseTokens * 4 }

// ServeMCPCmd exposes shannon's search and conversation retrieval tools to
// MCP clients (e.g. Claude Code) over stdio.
var ServeMCPCmd = &cobra.Command{
	Use:   "serve-mcp",
	Short: "Run an MCP server exposing shannon search over stdio",
	Long: `Run shannon as an MCP server. Connect from Claude Code with:

  claude mcp add --scope user shannon $(which shannon) serve-mcp

Tools exposed:
  - search_conversations:    full-text search across imported conversations
  - list_recent_conversations: most recently updated conversations
  - get_conversation_messages: fetch messages from a conversation by UUID`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

func run() error {
	cfg := config.Get()

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = database.Close() }()

	engine := search.NewEngine(database)

	s := server.NewMCPServer("shannon", "1.0.0")

	registerSearchTool(s, engine)
	registerListRecentTool(s, engine)
	registerGetMessagesTool(s, engine)

	return server.ServeStdio(s)
}

// SearchConversationsArgs are the user-supplied arguments for search.
type SearchConversationsArgs struct {
	Query      string `json:"query"`
	Limit      int    `json:"limit,omitempty"`
	Sender     string `json:"sender,omitempty"`
	AfterDate  string `json:"after_date,omitempty"`
	BeforeDate string `json:"before_date,omitempty"`
	ExactMatch bool   `json:"exact_match,omitempty"`
}

// ConversationMatch groups search hits by conversation.
type ConversationMatch struct {
	ConversationUUID string         `json:"conversation_uuid"`
	Name             string         `json:"name"`
	UpdatedAt        string         `json:"updated_at"`
	MessageCount     int            `json:"message_count"`
	Matches          []MatchSnippet `json:"matches"`
}

// MatchSnippet is a single message hit within a conversation.
type MatchSnippet struct {
	MessageUUID string `json:"message_uuid"`
	Sender      string `json:"sender"`
	CreatedAt   string `json:"created_at"`
	Snippet     string `json:"snippet"`
	Sequence    int    `json:"sequence"`
}

func registerSearchTool(s *server.MCPServer, engine *search.Engine) {
	tool := mcp.NewTool("search_conversations",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("Full-text search across imported Claude.ai conversations. Returns matched messages grouped by conversation, with snippets and conversation UUIDs that can be passed to get_conversation_messages."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("FTS5 query. Supports AND/OR/NOT (case-insensitive), quoted phrases, and prefix wildcards (foo*).")),
		mcp.WithNumber("limit",
			mcp.Description("Max conversations to return (default: 10).")),
		mcp.WithString("sender",
			mcp.Description("Restrict matches to one sender: 'human' or 'assistant'.")),
		mcp.WithString("after_date",
			mcp.Description("Only messages created on or after this ISO 8601 date (e.g. 2026-01-01).")),
		mcp.WithString("before_date",
			mcp.Description("Only messages created on or before this ISO 8601 date.")),
		mcp.WithBoolean("exact_match",
			mcp.Description("Treat query as an exact phrase (auto-quoted).")),
	)
	s.AddTool(tool, makeSearchHandler(engine))
}

func makeSearchHandler(engine *search.Engine) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs[SearchConversationsArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if args.Query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}

		query := args.Query
		if args.ExactMatch && len(query) > 0 && query[0] != '"' {
			query = `"` + query + `"`
		}

		opts := search.SearchOptions{
			Query:     query,
			Sender:    args.Sender,
			Limit:     limit * 5, // overfetch so we can group by conversation and still hit the limit
			SortBy:    "relevance",
			SortOrder: "desc",
		}
		if d, err := parseFlexDate(args.AfterDate); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid after_date: %v", err)), nil
		} else if !d.IsZero() {
			opts.StartDate = &d
		}
		if d, err := parseFlexDate(args.BeforeDate); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid before_date: %v", err)), nil
		} else if !d.IsZero() {
			opts.EndDate = &d
		}

		results, err := engine.Search(opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		grouped := groupByConversation(results, limit)

		// Enrich with updated_at and message_count so callers can rank
		// conversations without a follow-up call.
		for _, c := range grouped {
			if meta, err := loadConversationMeta(engine, c.ConversationUUID); err == nil {
				c.UpdatedAt = meta.updatedAt
				c.MessageCount = meta.messageCount
			}
		}

		grouped, truncated := trimToFit(grouped, func(v []*ConversationMatch) []byte {
			return mustMarshal(map[string]interface{}{"conversations": v})
		})

		payload := map[string]interface{}{"conversations": grouped}
		if truncated {
			payload["truncated"] = true
			payload["truncated_message"] = "Some conversations were dropped to fit the response budget. Narrow the query or lower the limit."
		}
		out, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// ListRecentConversationsArgs are the user-supplied arguments for the
// list_recent_conversations tool.
type ListRecentConversationsArgs struct {
	Limit int `json:"limit,omitempty"`
}

// ConversationSummary is a single row in the recent-conversations response.
type ConversationSummary struct {
	ConversationUUID string `json:"conversation_uuid"`
	Name             string `json:"name"`
	UpdatedAt        string `json:"updated_at"`
	CreatedAt        string `json:"created_at"`
	MessageCount     int    `json:"message_count"`
}

func registerListRecentTool(s *server.MCPServer, engine *search.Engine) {
	tool := mcp.NewTool("list_recent_conversations",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("List the most recently updated conversations. Useful for 'what was I working on yesterday' queries."),
		mcp.WithNumber("limit",
			mcp.Description("Max conversations to return (default: 20).")),
	)
	s.AddTool(tool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs[ListRecentConversationsArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}

		convs, err := engine.GetAllConversations(limit, 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
		}

		out := make([]ConversationSummary, 0, len(convs))
		for _, c := range convs {
			out = append(out, ConversationSummary{
				ConversationUUID: c.UUID,
				Name:             c.Name,
				UpdatedAt:        c.UpdatedAt.Format(time.RFC3339),
				CreatedAt:        c.CreatedAt.Format(time.RFC3339),
				MessageCount:     c.MessageCount,
			})
		}

		out, truncated := trimToFit(out, func(v []ConversationSummary) []byte {
			return mustMarshal(map[string]interface{}{"conversations": v})
		})

		payload := map[string]interface{}{"conversations": out}
		if truncated {
			payload["truncated"] = true
		}
		buf, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
		}
		return mcp.NewToolResultText(string(buf)), nil
	})
}

// GetConversationMessagesArgs are the user-supplied arguments for fetching
// messages from a specific conversation.
type GetConversationMessagesArgs struct {
	ConversationUUID string `json:"conversation_uuid"`
	LastN            int    `json:"last_n,omitempty"`
	AroundSequence   int    `json:"around_sequence,omitempty"`
	ContextSize      int    `json:"context_size,omitempty"`
}

// MessageDetail is a single message in the get_conversation_messages response.
type MessageDetail struct {
	MessageUUID string `json:"message_uuid"`
	Sender      string `json:"sender"`
	Text        string `json:"text"`
	CreatedAt   string `json:"created_at"`
	Sequence    int    `json:"sequence"`
}

// ConversationMessagesResponse wraps a list of messages with metadata.
type ConversationMessagesResponse struct {
	ConversationUUID string          `json:"conversation_uuid"`
	Name             string          `json:"name"`
	TotalCount       int             `json:"total_count"`
	ReturnedFrom     int             `json:"returned_from"`
	ReturnedTo       int             `json:"returned_to"`
	Messages         []MessageDetail `json:"messages"`
	Truncated        bool            `json:"truncated,omitempty"`
	TruncatedMessage string          `json:"truncated_message,omitempty"`
}

func registerGetMessagesTool(s *server.MCPServer, engine *search.Engine) {
	tool := mcp.NewTool("get_conversation_messages",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("Fetch messages from one conversation. Use last_n for the tail (e.g. 'what did I last say'), around_sequence to load context around a search hit, or neither for the full transcript (subject to the response budget)."),
		mcp.WithString("conversation_uuid",
			mcp.Required(),
			mcp.Description("Conversation UUID, as returned by search_conversations or list_recent_conversations.")),
		mcp.WithNumber("last_n",
			mcp.Description("Return only the last N messages.")),
		mcp.WithNumber("around_sequence",
			mcp.Description("Return messages whose sequence is near this value (paired with context_size).")),
		mcp.WithNumber("context_size",
			mcp.Description("Number of messages on either side of around_sequence (default: 10).")),
	)
	s.AddTool(tool, makeGetMessagesHandler(engine))
}

func makeGetMessagesHandler(engine *search.Engine) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs[GetConversationMessagesArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if args.ConversationUUID == "" {
			return mcp.NewToolResultError("conversation_uuid is required"), nil
		}

		convID, name, err := lookupConversationByUUID(engine, args.ConversationUUID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		_, messages, err := engine.GetConversation(convID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to load conversation: %v", err)), nil
		}

		total := len(messages)
		details := make([]MessageDetail, 0, total)
		for _, m := range messages {
			details = append(details, MessageDetail{
				MessageUUID: m.UUID,
				Sender:      m.Sender,
				Text:        m.Text,
				CreatedAt:   m.CreatedAt.Format(time.RFC3339),
				Sequence:    m.Sequence,
			})
		}

		// Apply windowing requested by the caller.
		tailMode := args.LastN > 0
		switch {
		case args.AroundSequence > 0:
			ctxSize := args.ContextSize
			if ctxSize <= 0 {
				ctxSize = 10
			}
			details = windowAround(details, args.AroundSequence, ctxSize)
		case tailMode && args.LastN < len(details):
			details = details[len(details)-args.LastN:]
		}

		details, truncated := trimMessagesToFit(details, args.ConversationUUID, name, total, tailMode)

		var fromSeq, toSeq int
		if len(details) > 0 {
			fromSeq = details[0].Sequence
			toSeq = details[len(details)-1].Sequence
		}

		resp := ConversationMessagesResponse{
			ConversationUUID: args.ConversationUUID,
			Name:             name,
			TotalCount:       total,
			ReturnedFrom:     fromSeq,
			ReturnedTo:       toSeq,
			Messages:         details,
		}
		if truncated {
			resp.Truncated = true
			resp.TruncatedMessage = fmt.Sprintf("Response truncated to fit the budget (%d of %d messages). Use last_n or around_sequence for targeted retrieval.", len(details), total)
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// windowAround returns the slice of messages whose Sequence values are
// within ctxSize of target. Assumes input is sorted by Sequence.
func windowAround(msgs []MessageDetail, target, ctxSize int) []MessageDetail {
	lo, hi := -1, -1
	for i, m := range msgs {
		if m.Sequence >= target-ctxSize && lo == -1 {
			lo = i
		}
		if m.Sequence <= target+ctxSize {
			hi = i
		}
	}
	if lo == -1 || hi == -1 || lo > hi {
		return nil
	}
	return msgs[lo : hi+1]
}

type convMeta struct {
	updatedAt    string
	messageCount int
}

func loadConversationMeta(engine *search.Engine, uuid string) (convMeta, error) {
	var (
		t time.Time
		n int
	)
	err := engine.DB().QueryRow(
		`SELECT updated_at, message_count FROM conversations WHERE uuid = ?`,
		uuid,
	).Scan(&t, &n)
	if err != nil {
		return convMeta{}, err
	}
	return convMeta{updatedAt: t.Format(time.RFC3339), messageCount: n}, nil
}

func lookupConversationByUUID(engine *search.Engine, uuid string) (int64, string, error) {
	var (
		id   int64
		name string
	)
	err := engine.DB().QueryRow(
		`SELECT id, name FROM conversations WHERE uuid = ?`,
		uuid,
	).Scan(&id, &name)
	if err == sql.ErrNoRows {
		return 0, "", fmt.Errorf("conversation %s not found", uuid)
	}
	if err != nil {
		return 0, "", fmt.Errorf("lookup failed: %w", err)
	}
	return id, name, nil
}

// groupByConversation collapses message-level search results into
// conversation-level entries, preserving the original ranking by keeping the
// first occurrence of each conversation and capping per-conversation matches
// at 3 to keep responses readable.
func groupByConversation(results []*models.SearchResult, limit int) []*ConversationMatch {
	const matchesPerConv = 3
	byUUID := map[string]*ConversationMatch{}
	order := []*ConversationMatch{}
	for _, r := range results {
		entry, ok := byUUID[r.ConversationUUID]
		if !ok {
			entry = &ConversationMatch{
				ConversationUUID: r.ConversationUUID,
				Name:             r.ConversationName,
			}
			byUUID[r.ConversationUUID] = entry
			order = append(order, entry)
			if len(order) > limit {
				// Stop accumulating new conversations once we have enough; we
				// still let further hits land on already-tracked conversations
				// (caller may have asked for them) up to matchesPerConv.
				break
			}
		}
		if len(entry.Matches) >= matchesPerConv {
			continue
		}
		entry.Matches = append(entry.Matches, MatchSnippet{
			MessageUUID: r.MessageUUID,
			Sender:      r.Sender,
			CreatedAt:   r.CreatedAt.Format(time.RFC3339),
			Snippet:     r.Snippet,
		})
	}
	if len(order) > limit {
		order = order[:limit]
	}
	return order
}

func parseArgs[T any](request mcp.CallToolRequest) (T, error) {
	var out T
	if m, ok := request.Params.Arguments.(map[string]interface{}); ok {
		coerceStringNumbers(m)
	}
	raw, _ := json.Marshal(request.Params.Arguments)
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("invalid arguments: %w", err)
	}
	return out, nil
}

// coerceStringNumbers converts string-encoded numbers in a map to floats so
// json.Unmarshal will accept them as int fields. Some MCP clients send all
// scalars as strings.
func coerceStringNumbers(m map[string]interface{}) {
	for k, v := range m {
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseFloat(s, 64); err == nil {
				m[k] = n
			}
		}
	}
}

// parseFlexDate accepts either an ISO date (YYYY-MM-DD) or a full RFC3339
// timestamp. Empty input returns zero time without error.
func parseFlexDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339, got %q", s)
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// trimToFit drops trailing items from the slice until the serialized payload
// fits the response byte budget.
func trimToFit[T any](items []T, marshal func([]T) []byte) ([]T, bool) {
	budget := maxResponseBytes()
	truncated := false
	for len(items) > 0 {
		if len(marshal(items)) <= budget {
			break
		}
		truncated = true
		items = items[:len(items)-1]
	}
	return items, truncated
}

// trimMessagesToFit trims a message window until the full
// ConversationMessagesResponse fits the budget. In tail mode it only drops
// from the front to preserve the most recent messages the caller asked for.
func trimMessagesToFit(msgs []MessageDetail, uuid, name string, total int, tailMode bool) ([]MessageDetail, bool) {
	budget := maxResponseBytes()
	truncated := false

	build := func(m []MessageDetail) []byte {
		resp := ConversationMessagesResponse{
			ConversationUUID: uuid,
			Name:             name,
			TotalCount:       total,
			Messages:         m,
		}
		if len(m) > 0 {
			resp.ReturnedFrom = m[0].Sequence
			resp.ReturnedTo = m[len(m)-1].Sequence
		}
		return mustMarshal(resp)
	}

	for len(msgs) > 1 {
		if len(build(msgs)) <= budget {
			break
		}
		truncated = true
		if tailMode {
			msgs = msgs[1:]
			continue
		}
		// Symmetric trim from whichever end carries more bytes.
		if len(msgs[0].Text) >= len(msgs[len(msgs)-1].Text) {
			msgs = msgs[1:]
		} else {
			msgs = msgs[:len(msgs)-1]
		}
	}
	if len(build(msgs)) > budget {
		return nil, true
	}
	return msgs, truncated
}
