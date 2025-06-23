package search

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/neilberkman/shannon/internal/db"
)

func setupTestDB(t *testing.T) (*Engine, func()) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "shannon-search-test")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := tmpDir + "/test.db"
	database, err := db.New(dbPath)
	if err != nil {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			t.Errorf("failed to remove temp dir: %v", removeErr)
		}
		t.Fatal(err)
	}

	// Insert test data
	insertTestData(t, database)

	engine := NewEngine(database)

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("failed to remove temp dir: %v", err)
		}
	}

	return engine, cleanup
}

func insertTestData(t *testing.T, database *db.DB) {
	// Insert test conversations and messages
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			t.Errorf("failed to rollback transaction: %v", err)
		}
	}()

	// Insert conversations
	conv1, err := tx.Exec(`
		INSERT INTO conversations (uuid, name, created_at, updated_at, message_count)
		VALUES (?, ?, ?, ?, ?)
	`, "conv-1", "Python Development", time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 0, -10), 3)
	if err != nil {
		t.Fatal(err)
	}
	conv1ID, _ := conv1.LastInsertId()

	conv2, err := tx.Exec(`
		INSERT INTO conversations (uuid, name, created_at, updated_at, message_count)
		VALUES (?, ?, ?, ?, ?)
	`, "conv-2", "Test Project Alpha", time.Now().AddDate(0, 0, -5), time.Now().AddDate(0, 0, -2), 2)
	if err != nil {
		t.Fatal(err)
	}
	conv2ID, _ := conv2.LastInsertId()

	// Insert branches
	branch1, err := tx.Exec(`INSERT INTO branches (conversation_id, name) VALUES (?, ?)`, conv1ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	branch1ID, _ := branch1.LastInsertId()

	branch2, err := tx.Exec(`INSERT INTO branches (conversation_id, name) VALUES (?, ?)`, conv2ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	branch2ID, _ := branch2.LastInsertId()

	// Insert messages for conversation 1
	messages := []struct {
		convID   int64
		branchID int64
		uuid     string
		sender   string
		text     string
		created  time.Time
	}{
		{conv1ID, branch1ID, "msg-1", "human", "How do I use Python for machine learning?", time.Now().AddDate(0, -1, 0)},
		{conv1ID, branch1ID, "msg-2", "assistant", "Python is great for machine learning with libraries like scikit-learn", time.Now().AddDate(0, -1, 0).Add(time.Minute)},
		{conv1ID, branch1ID, "msg-3", "human", "What about Python Django for web development?", time.Now().AddDate(0, 0, -10)},
		{conv2ID, branch2ID, "msg-4", "human", "Tell me about the test project with Alice", time.Now().AddDate(0, 0, -5)},
		{conv2ID, branch2ID, "msg-5", "assistant", "The test project is working with Alice on software architecture", time.Now().AddDate(0, 0, -5).Add(time.Minute)},
	}

	for i, msg := range messages {
		_, err := tx.Exec(`
			INSERT INTO messages (uuid, conversation_id, sender, text, created_at, parent_id, branch_id, sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.uuid, msg.convID, msg.sender, msg.text, msg.created.Format("2006-01-02 15:04:05"), nil, msg.branchID, i)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchWithDateFilters(t *testing.T) {
	engine, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name          string
		query         string
		startDate     *time.Time
		endDate       *time.Time
		expectedCount int
	}{
		{
			name:          "no date filter",
			query:         "python",
			expectedCount: 3, // msg-1, msg-2, and msg-3
		},
		{
			name:          "with start date filter",
			query:         "python",
			startDate:     timePtr(time.Now().AddDate(0, 0, -15)),
			expectedCount: 1, // only msg-3 is within last 15 days
		},
		{
			name:          "with end date filter", 
			query:         "python",
			endDate:       timePtr(time.Now().AddDate(0, 0, -20)),
			expectedCount: 2, // msg-1 and msg-2 are older than 20 days
		},
		{
			name:          "with both date filters",
			query:         "alice",
			startDate:     timePtr(time.Now().AddDate(0, 0, -7)),
			endDate:       timePtr(time.Now().AddDate(0, 0, -3)),
			expectedCount: 2, // msg-4 and msg-5 are within range
		},
		{
			name:          "no results in date range",
			query:         "python",
			startDate:     timePtr(time.Now().AddDate(0, 0, -3)),
			endDate:       timePtr(time.Now().AddDate(0, 0, -1)),
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SearchOptions{
				Query:     tt.query,
				StartDate: tt.startDate,
				EndDate:   tt.endDate,
				Limit:     100,
			}

			results, err := engine.Search(opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(results) != tt.expectedCount {
				t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

func TestSearchWithSenderFilter(t *testing.T) {
	engine, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name          string
		query         string
		sender        string
		expectedCount int
	}{
		{
			name:          "human messages only",
			query:         "python",
			sender:        "human",
			expectedCount: 2, // msg-1 and msg-3
		},
		{
			name:          "assistant messages only",
			query:         "python",
			sender:        "assistant",
			expectedCount: 1, // only msg-2
		},
		{
			name:          "no sender filter",
			query:         "python",
			sender:        "",
			expectedCount: 3, // msg-1, msg-2, and msg-3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SearchOptions{
				Query:  tt.query,
				Sender: tt.sender,
				Limit:  100,
			}

			results, err := engine.Search(opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(results) != tt.expectedCount {
				t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
			}

			// Verify sender matches
			for _, result := range results {
				if tt.sender != "" && result.Sender != tt.sender {
					t.Errorf("expected sender %s, got %s", tt.sender, result.Sender)
				}
			}
		})
	}
}

func TestSearchWithConversationFilter(t *testing.T) {
	engine, cleanup := setupTestDB(t)
	defer cleanup()

	// Get conversation IDs
	var conv1ID, conv2ID int64
	if err := engine.db.QueryRow("SELECT id FROM conversations WHERE name = ?", "Python Development").Scan(&conv1ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.db.QueryRow("SELECT id FROM conversations WHERE name = ?", "Test Project Alpha").Scan(&conv2ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		query          string
		conversationID *int64
		expectedCount  int
	}{
		{
			name:           "search in specific conversation",
			query:          "project",
			conversationID: &conv2ID,
			expectedCount:  2, // both messages in conv2 contain "project"
		},
		{
			name:           "search across all conversations",
			query:          "alice",
			conversationID: nil,
			expectedCount:  2, // both messages in conv2 contain "alice"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SearchOptions{
				Query:          tt.query,
				ConversationID: tt.conversationID,
				Limit:          100,
			}

			results, err := engine.Search(opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(results) != tt.expectedCount {
				t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

func TestSearchSortingAndPagination(t *testing.T) {
	engine, cleanup := setupTestDB(t)
	defer cleanup()

	// Test sorting by date
	opts := SearchOptions{
		Query:     "python OR alice",
		SortBy:    "date",
		SortOrder: "asc",
		Limit:     10,
	}

	results, err := engine.Search(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Verify results are sorted by date ascending
	for i := 1; i < len(results); i++ {
		prev := results[i-1].CreatedAt
		curr := results[i].CreatedAt
		if prev.After(curr) {
			t.Error("results not sorted by date ascending")
		}
	}

	// Test pagination
	opts.Limit = 2
	opts.Offset = 0
	page1, err := engine.Search(opts)
	if err != nil {
		t.Fatal(err)
	}

	opts.Offset = 2
	page2, err := engine.Search(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Verify no overlap between pages
	for _, r1 := range page1 {
		for _, r2 := range page2 {
			if r1.MessageID == r2.MessageID {
				t.Error("pagination overlap detected")
			}
		}
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}