package search

import (
	"os"
	"testing"
	"time"

	"github.com/user/shannon/internal/db"
)

func TestNewEngine(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "shannon-search-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Warning: failed to clean up temp dir: %v", err)
		}
	}()

	dbPath := tmpDir + "/test.db"
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Errorf("Warning: failed to close database: %v", err)
		}
	}()

	engine := NewEngine(database)
	if engine == nil {
		t.Fatal("Expected engine to be non-nil")
	}
	if engine.db != database {
		t.Error("Engine should store reference to database")
	}
}

func TestSearchOptions(t *testing.T) {
	opts := SearchOptions{
		Query:     "test query",
		Limit:     50,
		Offset:    0,
		SortBy:    "relevance",
		SortOrder: "desc",
	}

	if opts.Query != "test query" {
		t.Error("Query not set correctly")
	}
	if opts.Limit != 50 {
		t.Error("Limit not set correctly")
	}
	if opts.SortBy != "relevance" {
		t.Error("SortBy not set correctly")
	}
}

func TestSearchOptionsWithFilters(t *testing.T) {
	convID := int64(123)
	startDate := time.Now().AddDate(0, 0, -7)
	endDate := time.Now()

	opts := SearchOptions{
		Query:          "test",
		ConversationID: &convID,
		Sender:         "human",
		StartDate:      &startDate,
		EndDate:        &endDate,
	}

	if opts.ConversationID == nil || *opts.ConversationID != 123 {
		t.Error("ConversationID not set correctly")
	}
	if opts.Sender != "human" {
		t.Error("Sender not set correctly")
	}
	if opts.StartDate == nil {
		t.Error("StartDate not set")
	}
	if opts.EndDate == nil {
		t.Error("EndDate not set")
	}
}