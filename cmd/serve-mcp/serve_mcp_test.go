package servemcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/imports"
	"github.com/neilberkman/shannon/internal/search"
)

// setupEngine builds a temp shannon DB with a couple of synthetic
// conversations and returns a search.Engine wired to it.
func setupEngine(t *testing.T) *search.Engine {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shannon-mcp-test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	exportPath := filepath.Join(dir, "conversations.json")
	body := `[
		{
			"uuid": "11111111-1111-1111-1111-111111111111",
			"name": "Test Project Alpha",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:10Z",
			"chat_messages": [
				{"uuid": "11111111-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "sender": "human", "text": "How do I use Python for machine learning?", "created_at": "2026-01-01T00:00:00Z"},
				{"uuid": "11111111-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "sender": "assistant", "text": "Python is great for data science with pandas and numpy.", "created_at": "2026-01-01T00:00:01Z"}
			]
		},
		{
			"uuid": "22222222-2222-2222-2222-222222222222",
			"name": "Cooking Notes",
			"created_at": "2026-02-01T00:00:00Z",
			"updated_at": "2026-02-01T00:00:10Z",
			"chat_messages": [
				{"uuid": "22222222-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "sender": "human", "text": "Best ratio for vinaigrette?", "created_at": "2026-02-01T00:00:00Z"},
				{"uuid": "22222222-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "sender": "assistant", "text": "Three parts oil to one part acid.", "created_at": "2026-02-01T00:00:01Z"}
			]
		}
	]`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	imp := imports.NewImporter(database, 100, false)
	if _, err := imp.Import(exportPath); err != nil {
		t.Fatalf("import: %v", err)
	}

	return search.NewEngine(database)
}

func callHandler[T any](
	t *testing.T,
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
	args map[string]interface{},
) T {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler returned tool error: %+v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty response content")
	}
	textBlock, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var out T
	if err := json.Unmarshal([]byte(textBlock.Text), &out); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, textBlock.Text)
	}
	return out
}

func TestSearchHandler_FindsConversationByContent(t *testing.T) {
	engine := setupEngine(t)
	handler := makeSearchHandler(engine)

	type response struct {
		Conversations []ConversationMatch `json:"conversations"`
	}
	resp := callHandler[response](t, handler, map[string]interface{}{
		"query": "pandas",
	})

	if len(resp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation match, got %d: %+v", len(resp.Conversations), resp.Conversations)
	}
	if resp.Conversations[0].ConversationUUID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("unexpected uuid: %s", resp.Conversations[0].ConversationUUID)
	}
	if resp.Conversations[0].MessageCount == 0 {
		t.Error("expected enriched message_count to be populated")
	}
}

func TestSearchHandler_RespectsSenderFilter(t *testing.T) {
	engine := setupEngine(t)
	handler := makeSearchHandler(engine)

	type response struct {
		Conversations []ConversationMatch `json:"conversations"`
	}
	resp := callHandler[response](t, handler, map[string]interface{}{
		"query":  "python",
		"sender": "human",
	})

	if len(resp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(resp.Conversations))
	}
	for _, m := range resp.Conversations[0].Matches {
		if m.Sender != "human" {
			t.Errorf("sender filter leaked: got %q", m.Sender)
		}
	}
}

func TestSearchHandler_EmptyQueryRejected(t *testing.T) {
	engine := setupEngine(t)
	handler := makeSearchHandler(engine)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": ""}
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected tool error for empty query")
	}
}

func TestGetMessages_ByUUID(t *testing.T) {
	engine := setupEngine(t)
	handler := makeGetMessagesHandler(engine)

	resp := callHandler[ConversationMessagesResponse](t, handler, map[string]interface{}{
		"conversation_uuid": "22222222-2222-2222-2222-222222222222",
	})

	if resp.Name != "Cooking Notes" {
		t.Errorf("unexpected name: %s", resp.Name)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count=2, got %d", resp.TotalCount)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages returned, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Sender != "human" {
		t.Errorf("expected first message human, got %s", resp.Messages[0].Sender)
	}
}

func TestGetMessages_LastN(t *testing.T) {
	engine := setupEngine(t)
	handler := makeGetMessagesHandler(engine)

	resp := callHandler[ConversationMessagesResponse](t, handler, map[string]interface{}{
		"conversation_uuid": "22222222-2222-2222-2222-222222222222",
		"last_n":            float64(1),
	})

	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message returned, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Sender != "assistant" {
		t.Errorf("expected last message to be assistant, got %s", resp.Messages[0].Sender)
	}
}

func TestGetMessages_UnknownUUID(t *testing.T) {
	engine := setupEngine(t)
	handler := makeGetMessagesHandler(engine)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"conversation_uuid": "00000000-0000-0000-0000-000000000000",
	}
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected tool error for unknown uuid")
	}
}

func TestParseFlexDate(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"", time.Time{}},
		{"2026-01-15", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{"2026-01-15T10:00:00Z", time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got, err := parseFlexDate(tc.in)
		if err != nil {
			t.Errorf("parseFlexDate(%q) error: %v", tc.in, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("parseFlexDate(%q): got %v, want %v", tc.in, got, tc.want)
		}
	}
	if _, err := parseFlexDate("not-a-date"); err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestCoerceStringNumbers(t *testing.T) {
	m := map[string]interface{}{
		"limit":  "5",
		"name":   "kept",
		"flag":   true,
		"weight": "3.14",
	}
	coerceStringNumbers(m)
	if m["limit"] != float64(5) {
		t.Errorf("limit: got %v (%T), want 5.0", m["limit"], m["limit"])
	}
	if m["name"] != "kept" {
		t.Errorf("name should not be coerced, got %v", m["name"])
	}
	if m["flag"] != true {
		t.Errorf("flag should not be coerced, got %v", m["flag"])
	}
	if !strings.Contains(string(mustMarshal(m["weight"])), "3.14") {
		t.Errorf("weight should be coerced to 3.14, got %v", m["weight"])
	}
}

func TestTrimToFit_DropsTail(t *testing.T) {
	// Build a slice that obviously exceeds the byte budget.
	type item struct {
		Name string `json:"name"`
		Pad  string `json:"pad"`
	}
	pad := strings.Repeat("x", 1024)
	items := make([]item, 200)
	for i := range items {
		items[i] = item{Name: "n", Pad: pad}
	}
	out, truncated := trimToFit(items, func(v []item) []byte {
		return mustMarshal(map[string]interface{}{"items": v})
	})
	if !truncated {
		t.Fatal("expected truncation")
	}
	if len(out) >= len(items) {
		t.Errorf("expected length to shrink, got %d", len(out))
	}
}
