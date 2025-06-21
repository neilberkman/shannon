package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseInit(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "claudesearch-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Warning: failed to clean up temp dir: %v", err)
		}
	}()

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Warning: failed to close database: %v", err)
		}
	}()

	// Verify tables exist
	tables := []string{
		"conversations",
		"branches",
		"messages",
		"messages_fts",
		"import_history",
		"metadata",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}

	// Verify FTS5 virtual table
	var sql string
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&sql)
	if err != nil {
		t.Errorf("messages_fts table not found: %v", err)
	}
	if sql == "" {
		t.Error("messages_fts should be a virtual table")
	}
}

func TestTransaction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "claudesearch-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Warning: failed to clean up temp dir: %v", err)
		}
	}()

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Warning: failed to close database: %v", err)
		}
	}()

	// Test transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	_, err = tx.Exec("INSERT INTO metadata (key, value) VALUES (?, ?)", "test_key", "test_value")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Verify insert
	var value string
	err = db.QueryRow("SELECT value FROM metadata WHERE key = ?", "test_key").Scan(&value)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if value != "test_value" {
		t.Errorf("expected test_value, got %s", value)
	}
}
