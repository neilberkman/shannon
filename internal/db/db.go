package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	// Open database with pragmas for performance and FTS5
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(1) // SQLite only supports one writer
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	db := &DB{conn: conn}

	// Initialize schema
	if err := db.initSchema(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (also failed to close connection: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	schema := `
	-- Conversations table
	CREATE TABLE IF NOT EXISTS conversations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		message_count INTEGER DEFAULT 0,
		imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_conversations_uuid ON conversations(uuid);
	CREATE INDEX IF NOT EXISTS idx_conversations_created_at ON conversations(created_at);
	CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at);
	
	-- Branches table for conversation threading
	CREATE TABLE IF NOT EXISTS branches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL,
		name TEXT,
		parent_branch_id INTEGER,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
		FOREIGN KEY (parent_branch_id) REFERENCES branches(id)
	);
	CREATE INDEX IF NOT EXISTS idx_branches_conversation_id ON branches(conversation_id);
	
	-- Messages table
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT UNIQUE NOT NULL,
		conversation_id INTEGER NOT NULL,
		sender TEXT NOT NULL CHECK(sender IN ('human', 'assistant')),
		text TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		parent_id INTEGER,
		branch_id INTEGER NOT NULL,
		sequence INTEGER NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
		FOREIGN KEY (parent_id) REFERENCES messages(id),
		FOREIGN KEY (branch_id) REFERENCES branches(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_messages_uuid ON messages(uuid);
	CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_messages_branch_id ON messages(branch_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_parent_id ON messages(parent_id);
	
	-- Enhanced full-text search with multiple tokenizers for different content types
	-- Main FTS table with porter stemming for natural language
	CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		text,
		content=messages,
		content_rowid=id,
		tokenize='porter unicode61'
	);
	
	-- Code-specific FTS table that preserves symbols and camelCase
	CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts_code USING fts5(
		text,
		content=messages,
		content_rowid=id,
		tokenize='unicode61'
	);
	
	-- Triggers to keep FTS indices in sync
	CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
		INSERT INTO messages_fts_code(rowid, text) VALUES (new.id, new.text);
	END;
	
	CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
		DELETE FROM messages_fts WHERE rowid = old.id;
		DELETE FROM messages_fts_code WHERE rowid = old.id;
	END;
	
	CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
		UPDATE messages_fts SET text = new.text WHERE rowid = new.id;
		UPDATE messages_fts_code SET text = new.text WHERE rowid = new.id;
	END;
	
	-- Import tracking table
	CREATE TABLE IF NOT EXISTS import_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		file_hash TEXT NOT NULL,
		imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		conversations_count INTEGER,
		messages_count INTEGER,
		status TEXT NOT NULL CHECK(status IN ('success', 'partial', 'failed')),
		error_message TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_import_history_file_hash ON import_history(file_hash);
	
	-- Metadata table for database versioning
	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	
	-- Insert version if not exists
	INSERT OR IGNORE INTO metadata (key, value) VALUES ('schema_version', '1');
	INSERT OR IGNORE INTO metadata (key, value) VALUES ('app_version', '0.1.0');
	`

	_, err := db.conn.Exec(schema)
	return err
}

// Begin starts a new transaction
func (db *DB) Begin() (*sql.Tx, error) {
	return db.conn.Begin()
}

// Exec executes a query without returning rows
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}

// Query executes a query that returns rows
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// QueryRow executes a query that returns a single row
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}
