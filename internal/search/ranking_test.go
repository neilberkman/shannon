package search

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neilberkman/shannon/internal/db"
)

func TestParseQueryAndDates(t *testing.T) {
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	opts := parseQueryAt(`"machine learning" sender:human after:7d before:2026-06-14 sort:date`, now)
	if opts.Query != `"machine learning"` {
		t.Errorf("Query = %q, want %q", opts.Query, `"machine learning"`)
	}
	if opts.Sender != "human" {
		t.Errorf("Sender = %q, want human", opts.Sender)
	}
	if opts.SortBy != "date" {
		t.Errorf("SortBy = %q, want date", opts.SortBy)
	}
	if opts.StartDate == nil || !opts.StartDate.Equal(now.AddDate(0, 0, -7)) {
		t.Errorf("StartDate = %v, want %v", opts.StartDate, now.AddDate(0, 0, -7))
	}
	wantEnd := time.Date(2026, 6, 14, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	if opts.EndDate == nil || !opts.EndDate.Equal(wantEnd) {
		t.Errorf("EndDate = %v, want end-of-day %v", opts.EndDate, wantEnd)
	}

	// Filter-like token inside quotes stays literal query text.
	quoted := parseQueryAt(`"after:yesterday" from:assistant`, now)
	if quoted.Query != `"after:yesterday"` {
		t.Errorf("quoted Query = %q, want %q", quoted.Query, `"after:yesterday"`)
	}
	if quoted.StartDate != nil {
		t.Errorf("expected nil StartDate for quoted filter, got %v", quoted.StartDate)
	}
	if quoted.Sender != "assistant" {
		t.Errorf("Sender = %q, want assistant", quoted.Sender)
	}

	// Hyphenated relative phrases.
	rel := parseQueryAt(`after:3-days-ago`, now)
	if rel.Query != "" {
		t.Errorf("filter-only Query = %q, want empty", rel.Query)
	}
	if rel.StartDate == nil || !rel.StartDate.Equal(now.AddDate(0, 0, -3)) {
		t.Errorf("3-days-ago StartDate = %v, want %v", rel.StartDate, now.AddDate(0, 0, -3))
	}
}

func TestIsCodeQueryWholeWords(t *testing.T) {
	e := &Engine{}
	for _, english := range []string{"science", "capital", "latest recipe", "gardening tips"} {
		if e.isCodeQuery(english) {
			t.Errorf("isCodeQuery(%q) = true, want false (should keep porter stemming)", english)
		}
	}
	for _, code := range []string{"myFunc()", "snake_case", "camelCase", "json", "api design"} {
		if !e.isCodeQuery(code) {
			t.Errorf("isCodeQuery(%q) = false, want true", code)
		}
	}
}

func TestSearchBM25RankingDirectionAndPunctuation(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "rank.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	now := time.Now().UTC()
	seed := []struct {
		uuid    string
		name    string
		updated time.Time
		msg     string
	}{
		{"c-weak", "General Notes", now.Add(-2 * time.Hour), "This long note discusses many unrelated gardening topics, soil, watering, pruning, sunlight, compost, mulch, and briefly mentions kubernetes once at the very end."},
		{"c-strong", "Kubernetes Deployment", now.Add(-2 * time.Hour), "Kubernetes deployment guide: kubernetes pods, kubernetes services, and kubernetes ingress configuration."},
		{"c-title-only", "Astronomical Telescope Guide", now.Add(-1 * time.Hour), "Let's discuss lenses and mirrors for backyard stargazing."},
		{"c-hyphen", "Library Notes", now.Add(-1 * time.Hour), "We used scikit-learn for classification and emailed alice@example.com."},
	}

	for i, s := range seed {
		res, err := database.Exec(
			`INSERT INTO conversations (uuid, name, created_at, updated_at, message_count) VALUES (?, ?, ?, ?, 1)`,
			s.uuid, s.name, s.updated, s.updated,
		)
		if err != nil {
			t.Fatal(err)
		}
		cid, _ := res.LastInsertId()
		bres, err := database.Exec(`INSERT INTO branches (conversation_id, name) VALUES (?, 'main')`, cid)
		if err != nil {
			t.Fatal(err)
		}
		bid, _ := bres.LastInsertId()
		if _, err := database.Exec(
			`INSERT INTO messages (uuid, conversation_id, sender, text, search_text, created_at, branch_id, sequence) VALUES (?, ?, 'human', ?, ?, ?, ?, ?)`,
			s.uuid+"-m1", cid, s.msg, s.msg, s.updated, bid, i+1,
		); err != nil {
			t.Fatal(err)
		}
	}

	engine := NewEngine(database)

	// 1. Best BM25 match must rank FIRST (not last!).
	hits, err := engine.Search(SearchOptions{Query: "kubernetes", SortBy: "relevance", SortOrder: "desc", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ConversationUUID != "c-strong" {
		t.Errorf("top BM25 hit = %s (rank=%v), want c-strong (second=%s rank=%v)",
			hits[0].ConversationUUID, hits[0].Rank, hits[1].ConversationUUID, hits[1].Rank)
	}
	if hits[0].Sequence == 0 {
		t.Errorf("expected Sequence to be populated on SearchResult, got 0")
	}

	// 2. Punctuation queries (hyphenated library, email) must not fail with FTS syntax error.
	for _, q := range []string{"scikit-learn", "alice@example.com"} {
		res, err := engine.Search(SearchOptions{Query: q, Limit: 10})
		if err != nil {
			t.Fatalf("Search(%q) returned error: %v", q, err)
		}
		if len(res) != 1 || res[0].ConversationUUID != "c-hyphen" {
			t.Errorf("Search(%q) = %+v, want c-hyphen", q, res)
		}
	}

	// 3. Conversation-grouped search finds title-only matches too.
	grouped, err := engine.SearchConversationsGrouped(SearchOptions{Query: "Telescope", Limit: 10})
	if err != nil {
		t.Fatalf("SearchConversationsGrouped(title): %v", err)
	}
	if len(grouped) != 1 || grouped[0].Conversation.UUID != "c-title-only" {
		t.Fatalf("expected title-only match c-title-only, got %+v", grouped)
	}

	// 4. Filter-only conversation search works without a keyword.
	since := now.Add(-90 * time.Minute)
	filtered, err := engine.SearchConversationsGrouped(SearchOptions{StartDate: &since, Limit: 10})
	if err != nil {
		t.Fatalf("filter-only SearchConversationsGrouped: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 recent conversations, got %d", len(filtered))
	}
}
