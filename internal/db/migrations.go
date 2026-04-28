package db

// migrate runs schema upgrades for existing databases. Each migration is
// idempotent — safe to run on a fresh DB (whose initSchema already includes
// the migrated columns) and on databases at any prior version.
func (db *DB) migrate() error {
	if err := db.migration001AddImportFileTracking(); err != nil {
		return err
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
