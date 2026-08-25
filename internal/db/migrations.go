package db

import "strings"

// migrate runs schema upgrades for existing databases. Each migration is
// idempotent — safe to run on a fresh DB (whose initSchema already includes
// the migrated columns) and on databases at any prior version.
func (db *DB) migrate() error {
	if err := db.migration001AddImportFileTracking(); err != nil {
		return err
	}
	if err := db.migration002AddMessageSearchText(); err != nil {
		return err
	}
	if err := db.migration003FixFTSSyncTriggers(); err != nil {
		return err
	}
	return nil
}

// FTS5 external-content tables cannot be kept in sync with plain UPDATE and
// DELETE statements: removing a row's tokens requires replaying its old
// values through the table's 'delete' command. Writing to the index any other
// way leaves it inconsistent with its content table, which SQLite later
// reports as "database disk image is malformed".
const (
	ddlTriggerDelete = `CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, search_text) VALUES('delete', old.id, old.search_text);
		INSERT INTO messages_fts_code(messages_fts_code, rowid, search_text) VALUES('delete', old.id, old.search_text);
	END`

	ddlTriggerUpdate = `CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, search_text) VALUES('delete', old.id, old.search_text);
		INSERT INTO messages_fts(rowid, search_text) VALUES (new.id, new.search_text);
		INSERT INTO messages_fts_code(messages_fts_code, rowid, search_text) VALUES('delete', old.id, old.search_text);
		INSERT INTO messages_fts_code(rowid, search_text) VALUES (new.id, new.search_text);
	END`
)

// migration003FixFTSSyncTriggers replaces the update and delete triggers that
// wrote to the FTS indexes directly. Databases created before this fix carry
// triggers that corrupt the index the first time a message is edited or a
// conversation is deleted; the indexes are rebuilt afterwards in case either
// already fired.
func (db *DB) migration003FixFTSSyncTriggers() error {
	var broken int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type = 'trigger' AND name IN ('messages_ad', 'messages_au')
		   AND (sql LIKE '%UPDATE messages_fts%' OR sql LIKE '%DELETE FROM messages_fts%')`,
	).Scan(&broken)
	if err != nil {
		return err
	}
	if broken == 0 {
		return nil
	}

	repair := []string{
		`DROP TRIGGER IF EXISTS messages_ad`,
		`DROP TRIGGER IF EXISTS messages_au`,
		ddlTriggerDelete,
		ddlTriggerUpdate,
		`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`,
		`INSERT INTO messages_fts_code(messages_fts_code) VALUES('rebuild')`,
	}
	for _, stmt := range repair {
		if _, err := db.conn.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// migration001AddImportFileTracking adds file_size, file_mtime, file_inode,
// and file_device columns to import_history. These let the importer detect
// unchanged files via os.Stat alone, skipping the SHA-256 read.
func (db *DB) migration001AddImportFileTracking() error {
	columns := []struct{ name, ddl string }{
		{"file_size", `ALTER TABLE import_history ADD COLUMN file_size INTEGER`},
		{"file_mtime", `ALTER TABLE import_history ADD COLUMN file_mtime DATETIME`},
		{"file_inode", `ALTER TABLE import_history ADD COLUMN file_inode INTEGER`},
		{"file_device", `ALTER TABLE import_history ADD COLUMN file_device INTEGER`},
	}

	for _, col := range columns {
		var count int
		err := db.conn.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('import_history') WHERE name = ?`,
			col.name,
		).Scan(&count)
		if err != nil {
			return err
		}
		if count == 0 {
			if _, err := db.conn.Exec(col.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// migration002AddMessageSearchText separates what is displayed from what is
// indexed. messages.text holds the prose a reader sees; messages.search_text
// holds everything worth finding, including extended thinking and tool
// results that never appear in the displayed prose.
//
// Existing rows are backfilled from text, which preserves current search
// behaviour exactly. Recovering the content blocks that earlier imports
// discarded requires re-importing the export with --force.
func (db *DB) migration002AddMessageSearchText() error {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'search_text'`,
	).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		if _, err := db.conn.Exec(
			`ALTER TABLE messages ADD COLUMN search_text TEXT NOT NULL DEFAULT ''`,
		); err != nil {
			return err
		}
	}

	// The FTS tables and their triggers are created with IF NOT EXISTS, so an
	// existing database keeps indexing the old column until they are dropped
	// and rebuilt against search_text.
	stale, err := db.ftsIndexesTargetOldColumn()
	if err != nil {
		return err
	}

	if !stale {
		// Already migrated. Backfill anyway: a run interrupted between adding
		// the column and filling it would otherwise leave rows unindexed. The
		// predicate makes the common case a no-op.
		return db.backfillSearchText()
	}

	// Drop the stale triggers and indexes before backfilling, so the backfill
	// does not fire writes into indexes that are about to be discarded.
	teardown := []string{
		`DROP TRIGGER IF EXISTS messages_ai`,
		`DROP TRIGGER IF EXISTS messages_ad`,
		`DROP TRIGGER IF EXISTS messages_au`,
		`DROP TABLE IF EXISTS messages_fts`,
		`DROP TABLE IF EXISTS messages_fts_code`,
	}
	for _, stmt := range teardown {
		if _, err := db.conn.Exec(stmt); err != nil {
			return err
		}
	}

	if err := db.backfillSearchText(); err != nil {
		return err
	}

	rebuild := []string{
		`CREATE VIRTUAL TABLE messages_fts USING fts5(
			search_text,
			content=messages,
			content_rowid=id,
			tokenize='porter unicode61'
		)`,
		`CREATE VIRTUAL TABLE messages_fts_code USING fts5(
			search_text,
			content=messages,
			content_rowid=id,
			tokenize='unicode61'
		)`,
		`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`,
		`INSERT INTO messages_fts_code(messages_fts_code) VALUES('rebuild')`,
		`CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, search_text) VALUES (new.id, new.search_text);
			INSERT INTO messages_fts_code(rowid, search_text) VALUES (new.id, new.search_text);
		END`,
		ddlTriggerDelete,
		ddlTriggerUpdate,
	}

	for _, stmt := range rebuild {
		if _, err := db.conn.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ftsIndexesTargetOldColumn reports whether the message FTS tables are still
// defined over the display column rather than search_text.
func (db *DB) ftsIndexesTargetOldColumn() (bool, error) {
	rows, err := db.conn.Query(
		`SELECT sql FROM sqlite_master
		 WHERE type = 'table' AND name IN ('messages_fts', 'messages_fts_code')`,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ddl string
		if err := rows.Scan(&ddl); err != nil {
			return false, err
		}
		if !strings.Contains(ddl, "search_text") {
			return true, nil
		}
	}
	return false, rows.Err()
}

// backfillSearchText seeds search_text from the display column for rows that
// predate the split. Rows imported after the split already carry the richer
// extracted text and are left alone.
func (db *DB) backfillSearchText() error {
	_, err := db.conn.Exec(
		`UPDATE messages SET search_text = text WHERE search_text = '' AND text <> ''`,
	)
	return err
}
