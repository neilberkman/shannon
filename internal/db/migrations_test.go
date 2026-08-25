package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration002RebuildsFTSOverSearchText simulates a pre-migration
// database — messages without search_text and FTS defined over text — and
// verifies the migration backfills content and reindexes without data loss.
func TestMigration002RebuildsFTSOverSearchText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	db, err := New(path)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	// Rewind to the pre-migration shape.
	legacy := []string{
		`DROP TRIGGER messages_ai`,
		`DROP TRIGGER messages_ad`,
		`DROP TRIGGER messages_au`,
		`DROP TABLE messages_fts`,
		`DROP TABLE messages_fts_code`,
		`CREATE VIRTUAL TABLE messages_fts USING fts5(
			text, content=messages, content_rowid=id, tokenize='porter unicode61')`,
		`CREATE VIRTUAL TABLE messages_fts_code USING fts5(
			text, content=messages, content_rowid=id, tokenize='unicode61')`,
		`CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
			INSERT INTO messages_fts_code(rowid, text) VALUES (new.id, new.text);
		END`,
		`CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
			DELETE FROM messages_fts WHERE rowid = old.id;
			DELETE FROM messages_fts_code WHERE rowid = old.id;
		END`,
		`CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
			UPDATE messages_fts SET text = new.text WHERE rowid = new.id;
			UPDATE messages_fts_code SET text = new.text WHERE rowid = new.id;
		END`,
		`INSERT INTO conversations (uuid, name, created_at, updated_at)
			VALUES ('c1', 'legacy', '2026-01-01', '2026-01-01')`,
		`INSERT INTO branches (id, conversation_id, name) VALUES (1, 1, 'main')`,
		`INSERT INTO messages (uuid, conversation_id, sender, text, created_at, branch_id, sequence)
			VALUES ('m1', 1, 'human', 'legacy message about voltage', '2026-01-01', 1, 0)`,
	}
	for _, stmt := range legacy {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("failed to build legacy schema (%s): %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Reopening runs migrate().
	db, err = New(path)
	if err != nil {
		t.Fatalf("failed to reopen db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var ddl string
	if err := db.conn.QueryRow(
		`SELECT sql FROM sqlite_master WHERE name = 'messages_fts'`).Scan(&ddl); err != nil {
		t.Fatalf("failed to read fts ddl: %v", err)
	}
	if !strings.Contains(ddl, "search_text") {
		t.Errorf("messages_fts should index search_text after migration; got %s", ddl)
	}

	var backfilled string
	if err := db.conn.QueryRow(
		`SELECT search_text FROM messages WHERE uuid = 'm1'`).Scan(&backfilled); err != nil {
		t.Fatalf("failed to read search_text: %v", err)
	}
	if backfilled != "legacy message about voltage" {
		t.Errorf("search_text should be backfilled from text; got %q", backfilled)
	}

	var hits int
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'voltage'`).Scan(&hits); err != nil {
		t.Fatalf("failed to query fts: %v", err)
	}
	if hits != 1 {
		t.Errorf("rebuilt index should find the legacy message; got %d hits", hits)
	}

	// Running again must be a no-op rather than a second rebuild.
	if err := db.migrate(); err != nil {
		t.Errorf("migration should be idempotent: %v", err)
	}
}
