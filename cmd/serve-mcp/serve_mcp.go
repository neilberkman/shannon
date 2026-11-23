package servemcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/neilberkman/shannon/cmd/root"
	"github.com/neilberkman/shannon/internal/config"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/imports"
	"github.com/neilberkman/shannon/internal/search"
)

// MaxResponseTokens is the response budget. Claude Code warns at 10k tokens
// and hard-rejects at 25k. Targeting 9k leaves headroom. Token estimate
// approximates len(json) / 4, which is conservative for mixed JSON/English.
const MaxResponseTokens = 9000

const (
	maxTitleRunes   = 300
	maxSnippetRunes = 500
)

func maxResponseBytes() int { return MaxResponseTokens * 4 }

// ServeMCPCmd exposes shannon's search and conversation retrieval tools to
// MCP clients (e.g. Claude Code) over stdio.
var ServeMCPCmd = &cobra.Command{
	Use:   "serve-mcp",
	Short: "Run an MCP server exposing shannon search over stdio",
	Long: `Run shannon as an MCP server. Connect from Claude Code with:

  claude mcp add --scope user shannon $(which shannon) serve-mcp

Tools exposed:
  - search_conversations:      ranked full-text & filter search across imported conversations
  - list_recent_conversations: most recently updated conversations (with optional date bounds)
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

	syncFn := func(ctx context.Context) {
		syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if _, err := imports.SyncDiscovered(syncCtx, database); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: background export sync incomplete; serving cached data: %v\n", err)
		}
	}

	version := root.Version
	if version == "" {
		version = "dev"
	}
	s := server.NewMCPServer("shannon", version)

	registerSearchTool(s, engine, syncFn)
	registerListRecentTool(s, engine, syncFn)
	registerGetMessagesTool(s, engine, syncFn)

	return server.ServeStdio(s)
}

// SearchConversationsArgs are the user-supplied arguments for search.
type SearchConversationsArgs struct {
	Query      string `json:"query,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Sender     string `json:"sender,omitempty"`
	AfterDate  string `json:"after_date,omitempty"`
	BeforeDate string `json:"before_date,omitempty"`
	ExactMatch bool   `json:"exact_match,omitempty"`
	SortBy     string `json:"sort_by,omitempty"`
}

// ConversationMatch groups search hits by conversation.
type ConversationMatch struct {
	ConversationUUID string         `json:"conversation_uuid"`
	Name             string         `json:"name"`
	UpdatedAt        string         `json:"updated_at"`
	MessageCount     int            `json:"message_count"`
	MatchCount       int            `json:"match_count"`
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

func registerSearchTool(s *server.MCPServer, engine *search.Engine, syncFn func(context.Context)) {
	tool := mcp.NewTool("search_conversations",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("Full-text and filtered search across imported Claude.ai conversations (titles, messages, extended thinking, tool inputs/results, attachments, and uploaded filenames). Results are grouped by conversation and ranked by BM25 relevance, match count, title match, and recency. Automatically imports any newly downloaded Claude.ai .zip or JSON exports before searching."),
		mcp.WithString("query",
			mcp.Description("FTS5 query. Supports AND/OR/NOT (case-insensitive), quoted phrases, prefix wildcards (foo*), and inline filters (after:7d, after:yesterday, before:2026-01-01, sender:human). May be omitted when date or sender filters are provided.")),
		mcp.WithNumber("limit",
			mcp.Description("Max conversations to return (default: 10).")),
		mcp.WithString("sender",
			mcp.Description("Restrict matches to one sender: 'human' or 'assistant'.")),
		mcp.WithString("after_date",
			mcp.Description("Only matches/conversations on or after this date (YYYY-MM-DD, RFC3339, or relative like '7d', 'yesterday', '3-days-ago').")),
		mcp.WithString("before_date",
			mcp.Description("Only matches/conversations on or before this date (YYYY-MM-DD, RFC3339, or relative).")),
		mcp.WithBoolean("exact_match",
			mcp.Description("Treat query as an exact phrase (auto-quoted).")),
		mcp.WithString("sort_by",
			mcp.Description("Sort order: 'relevance' (default) or 'date'.")),
	)
	s.AddTool(tool, makeSearchHandlerWithSync(engine, syncFn))
}

func makeSearchHandler(engine *search.Engine) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return makeSearchHandlerWithSync(engine, nil)
}

func makeSearchHandlerWithSync(engine *search.Engine, syncFn func(context.Context)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if syncFn != nil {
			syncFn(ctx)
		}

		args, err := parseArgs[SearchConversationsArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}

		var opts search.SearchOptions
		if args.ExactMatch {
			q := strings.TrimSpace(args.Query)
			if q != "" && !strings.HasPrefix(q, `"`) {
				q = `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
			}
			opts.Query = q
		} else {
			opts = search.ParseQuery(args.Query)
		}

		opts.Limit = limit
		if args.Sender != "" {
			opts.Sender = strings.ToLower(args.Sender)
		}
		if args.SortBy != "" {
			opts.SortBy = strings.ToLower(args.SortBy)
		}
		if opts.SortBy == "" {
			opts.SortBy = "relevance"
		}
		opts.SortOrder = "desc"

		if args.AfterDate != "" {
			d, err := parseFlexDate(args.AfterDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid after_date: %v", err)), nil
			}
			opts.StartDate = &d
		}
		if args.BeforeDate != "" {
			d, err := parseFlexEndDate(args.BeforeDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid before_date: %v", err)), nil
			}
			opts.EndDate = &d
		}

		if strings.TrimSpace(opts.Query) == "" && !opts.HasFilters() {
			return mcp.NewToolResultError("query or at least one filter (after_date, before_date, sender) is required"), nil
		}

		coreResults, err := engine.SearchConversationsGrouped(opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		const matchesPerConv = 3
		grouped := make([]*ConversationMatch, 0, len(coreResults))
		for _, cr := range coreResults {
			entry := &ConversationMatch{
				ConversationUUID: cr.Conversation.UUID,
				Name:             truncateText(cr.Conversation.Name, maxTitleRunes),
				UpdatedAt:        cr.Conversation.UpdatedAt.Format(time.RFC3339),
				MessageCount:     cr.Conversation.MessageCount,
				MatchCount:       len(cr.Matches),
				Matches:          []MatchSnippet{},
			}
			for i, m := range cr.Matches {
				if i >= matchesPerConv {
					break
				}
				entry.Matches = append(entry.Matches, MatchSnippet{
					MessageUUID: m.MessageUUID,
					Sender:      m.Sender,
					CreatedAt:   m.CreatedAt.Format(time.RFC3339),
					Snippet:     truncateText(m.Snippet, maxSnippetRunes),
					Sequence:    m.Sequence,
				})
			}
			grouped = append(grouped, entry)
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
	Limit      int    `json:"limit,omitempty"`
	AfterDate  string `json:"after_date,omitempty"`
	BeforeDate string `json:"before_date,omitempty"`
}

// ConversationSummary is a single row in the recent-conversations response.
type ConversationSummary struct {
	ConversationUUID string `json:"conversation_uuid"`
	Name             string `json:"name"`
	UpdatedAt        string `json:"updated_at"`
	CreatedAt        string `json:"created_at"`
	MessageCount     int    `json:"message_count"`
}

func registerListRecentTool(s *server.MCPServer, engine *search.Engine, syncFn func(context.Context)) {
	tool := mcp.NewTool("list_recent_conversations",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("List the most recently updated Claude.ai conversations, optionally filtered by date range. Automatically imports any newly downloaded exports first."),
		mcp.WithNumber("limit",
			mcp.Description("Max conversations to return (default: 20).")),
		mcp.WithString("after_date",
			mcp.Description("Only conversations updated on or after this date (YYYY-MM-DD, RFC3339, or relative like '7d', 'yesterday').")),
		mcp.WithString("before_date",
			mcp.Description("Only conversations updated on or before this date.")),
	)
	s.AddTool(tool, makeListRecentHandlerWithSync(engine, syncFn))
}

func makeListRecentHandlerWithSync(engine *search.Engine, syncFn func(context.Context)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if syncFn != nil {
			syncFn(ctx)
		}

		args, err := parseArgs[ListRecentConversationsArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}

		var startDate, endDate *time.Time
		if args.AfterDate != "" {
			d, err := parseFlexDate(args.AfterDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid after_date: %v", err)), nil
			}
			startDate = &d
		}
		if args.BeforeDate != "" {
			d, err := parseFlexEndDate(args.BeforeDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid before_date: %v", err)), nil
			}
			endDate = &d
		}

		convs, err := engine.GetRecentConversations(limit, startDate, endDate)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
		}

		out := make([]ConversationSummary, 0, len(convs))
		for _, c := range convs {
			out = append(out, ConversationSummary{
				ConversationUUID: c.UUID,
				Name:             truncateText(c.Name, maxTitleRunes),
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
	}
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

func registerGetMessagesTool(s *server.MCPServer, engine *search.Engine, syncFn func(context.Context)) {
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
	s.AddTool(tool, makeGetMessagesHandlerWithSync(engine, syncFn))
}

func makeGetMessagesHandler(engine *search.Engine) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return makeGetMessagesHandlerWithSync(engine, nil)
}

func makeGetMessagesHandlerWithSync(engine *search.Engine, syncFn func(context.Context)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if syncFn != nil {
			syncFn(ctx)
		}

		args, err := parseArgs[GetConversationMessagesArgs](request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if args.ConversationUUID == "" {
			return mcp.NewToolResultError("conversation_uuid is required"), nil
		}

		conv, messages, err := engine.GetConversationByUUID(args.ConversationUUID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
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

		details, truncated := trimMessagesToFit(details, args.ConversationUUID, conv.Name, total, tailMode)

		var fromSeq, toSeq int
		if len(details) > 0 {
			fromSeq = details[0].Sequence
			toSeq = details[len(details)-1].Sequence
		}

		resp := ConversationMessagesResponse{
			ConversationUUID: args.ConversationUUID,
			Name:             truncateText(conv.Name, maxTitleRunes),
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

func truncateText(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
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

// parseFlexDate accepts ISO dates (YYYY-MM-DD), RFC3339 timestamps, and
// relative expressions (7d, yesterday, 3-days-ago). Empty input returns zero
// time without error.
func parseFlexDate(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	return search.ParseDate(s, false)
}

func parseFlexEndDate(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	return search.ParseDate(s, true)
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
