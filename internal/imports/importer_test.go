package imports

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/discovery"
	"github.com/neilberkman/shannon/internal/models"
)

func setupImporter(t *testing.T) (*Importer, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shannon-test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	return NewImporter(database, 100, false), dir
}

func writeSyntheticExport(t *testing.T, path string, convUUID string) {
	t.Helper()
	body := `[{
		"uuid": "` + convUUID + `",
		"name": "Test Project Alpha",
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
		"chat_messages": [
			{
				"uuid": "11111111-1111-1111-1111-111111111111",
				"sender": "human",
				"text": "How do I use Python for machine learning?",
				"created_at": "2026-01-01T00:00:00Z"
			},
			{
				"uuid": "22222222-2222-2222-2222-222222222222",
				"sender": "assistant",
				"text": "Python is great for data science with pandas and numpy.",
				"created_at": "2026-01-01T00:00:01Z"
			}
		]
	}]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write export: %v", err)
	}
}

func TestImport_FastPathRejectsUnchangedFile(t *testing.T) {
	imp, dir := setupImporter(t)
	exportPath := filepath.Join(dir, "conversations.json")
	writeSyntheticExport(t, exportPath, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	if _, err := imp.Import(exportPath); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Re-import without touching the file: must short-circuit on the fast path
	// before hashing.
	_, err := imp.Import(exportPath)
	if err == nil {
		t.Fatal("re-import unexpectedly succeeded; expected already-imported error")
	}
	if !strings.Contains(err.Error(), "already imported") {
		t.Fatalf("expected already-imported error, got: %v", err)
	}
	// Fast path uses the unchanged sentinel, not the hash sentinel.
	if !strings.Contains(err.Error(), "unchanged") {
		t.Fatalf("expected fast-path unchanged sentinel, got: %v", err)
	}
}

func TestImport_HashPathRejectsUnchangedContentAtNewPath(t *testing.T) {
	imp, dir := setupImporter(t)
	originalPath := filepath.Join(dir, "conversations.json")
	writeSyntheticExport(t, originalPath, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	if _, err := imp.Import(originalPath); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Copy to a new path. Same bytes, different filename. Fast path should
	// miss (different file_path), but hash path should reject.
	copyPath := filepath.Join(dir, "conversations-copy.json")
	data, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	if err := os.WriteFile(copyPath, data, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	_, err = imp.Import(copyPath)
	if err == nil {
		t.Fatal("re-import via different path unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "already imported (hash:") {
		t.Fatalf("expected hash-path already-imported error, got: %v", err)
	}
}

func TestImport_FastPathSurvivesMtimeWithinTolerance(t *testing.T) {
	imp, dir := setupImporter(t)
	exportPath := filepath.Join(dir, "conversations.json")
	writeSyntheticExport(t, exportPath, "cccccccc-cccc-cccc-cccc-cccccccccccc")

	if _, err := imp.Import(exportPath); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Nudge the mtime forward by less than a second — still considered the same
	// file by the fast path. (Filesystems and cloud sync clients sometimes
	// rewrite subsecond components.)
	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(exportPath, time.Now(), info.ModTime().Add(500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	_, err = imp.Import(exportPath)
	if err == nil || !strings.Contains(err.Error(), "unchanged") {
		t.Fatalf("expected fast-path rejection within mtime tolerance, got: %v", err)
	}
}

func TestImport_DetectsModifiedContent(t *testing.T) {
	imp, dir := setupImporter(t)
	exportPath := filepath.Join(dir, "conversations.json")
	writeSyntheticExport(t, exportPath, "dddddddd-dddd-dddd-dddd-dddddddddddd")

	if _, err := imp.Import(exportPath); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Replace with a different conversation. Both fast path (size+mtime
	// changed) and hash path (different content) must miss, so this should
	// import successfully.
	writeSyntheticExport(t, exportPath, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	// Force mtime past the 1-second tolerance window.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(exportPath, future, future); err != nil {
		t.Fatal(err)
	}

	stats, err := imp.Import(exportPath)
	if err != nil {
		t.Fatalf("re-import after content change failed: %v", err)
	}
	if stats.ConversationsImported == 0 {
		t.Fatal("expected new conversation to be imported")
	}
}

// TestReimportRepairsLossyMessages covers the self-heal path: a message
// stored by an older importer kept only its first text block, and
// re-importing the same export must recover the rest without duplicating the
// message or losing the display prose.
func TestReimportRepairsLossyMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repair.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer func() { _ = database.Close() }()

	seed := []string{
		`INSERT INTO conversations (uuid, name, created_at, updated_at)
			VALUES ('c1', 'legacy', '2026-01-01', '2026-01-01')`,
		`INSERT INTO branches (id, conversation_id, name) VALUES (1, 1, 'main')`,
		// As an older importer would have stored it: first text block only.
		`INSERT INTO messages (uuid, conversation_id, sender, text, search_text, created_at, branch_id, sequence)
			VALUES ('m1', 1, 'assistant', 'visible answer', 'visible answer', '2026-01-01', 1, 0)`,
	}
	for _, stmt := range seed {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed failed (%s): %v", stmt, err)
		}
	}

	msg := models.ClaudeChatMessage{
		UUID:      "m1",
		Sender:    "assistant",
		Text:      "visible answer",
		CreatedAt: "2026-01-01T00:00:00Z",
		Content: []models.ClaudeMessageContent{
			{Type: "text", Text: "visible answer"},
			{Type: "thinking", Thinking: "the Orion Nebula deadline is September 11"},
		},
	}

	importer := NewImporter(database, 100, false)
	tx, err := database.Begin()
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	existing := &existingConvState{
		byUUID:           make(map[string]existingMessage),
		idByUUID:         make(map[string]int64),
		mainParentHasKid: make(map[int64]bool),
	}
	if err := importer.loadExistingConvState(tx, 1, 1, existing); err != nil {
		t.Fatalf("failed to read existing messages: %v", err)
	}
	stats := &models.ImportStats{}
	newCount, _, err := importer.importNewMessages(tx, 1, 1, []models.ClaudeChatMessage{msg}, existing, stats)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	if newCount != 0 {
		t.Errorf("existing message must not be re-inserted; got %d new", newCount)
	}
	if stats.MessagesUpdated != 1 {
		t.Errorf("expected 1 repaired message, got %d", stats.MessagesUpdated)
	}

	var text, searchText string
	if err := database.QueryRow(
		`SELECT text, search_text FROM messages WHERE uuid = 'm1'`).Scan(&text, &searchText); err != nil {
		t.Fatalf("failed to read repaired row: %v", err)
	}
	if text != "visible answer" {
		t.Errorf("display text should be unchanged; got %q", text)
	}
	if !strings.Contains(searchText, "Orion Nebula") {
		t.Errorf("thinking content should be recovered; got %q", searchText)
	}

	var hits int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'Orion'`).Scan(&hits); err != nil {
		t.Fatalf("fts query failed: %v", err)
	}
	if hits != 1 {
		t.Errorf("repaired content should be searchable; got %d hits", hits)
	}
}

func writeSyntheticZipExport(t *testing.T, zipPath, entryName, convUUID string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(entryName)
	if err != nil {
		t.Fatalf("zip create entry: %v", err)
	}
	body := `[{
		"uuid": "` + convUUID + `",
		"name": "Zipped Synthetic Chat",
		"created_at": "2026-03-01T00:00:00Z",
		"updated_at": "2026-03-01T00:05:00Z",
		"chat_messages": [
			{
				"uuid": "` + convUUID + `-m1",
				"sender": "human",
				"text": "Explain Go zip streaming",
				"created_at": "2026-03-01T00:00:00Z"
			},
			{
				"uuid": "` + convUUID + `-m2",
				"sender": "assistant",
				"text": "Use archive/zip to stream entries directly without extracting.",
				"created_at": "2026-03-01T00:01:00Z"
			}
		]
	}]`
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file close: %v", err)
	}
}

func TestImport_ZipDirectAndBangEntryAndFastSkip(t *testing.T) {
	imp, dir := setupImporter(t)
	zipPath := filepath.Join(dir, "data-2026-03-01-export.zip")
	writeSyntheticZipExport(t, zipPath, "data-2026-03-01/conversations.json", "zip-conv-1")

	stats, err := imp.Import(zipPath)
	if err != nil {
		t.Fatalf("zip import failed: %v", err)
	}
	if stats.ConversationsImported != 1 || stats.MessagesImported != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Re-importing via the discovery zip!entry syntax should hit the fast path.
	_, err = imp.Import(zipPath + "!data-2026-03-01/conversations.json")
	if err == nil || !strings.Contains(err.Error(), "already imported") {
		t.Fatalf("expected already-imported skip for zip!entry, got: %v", err)
	}
}

func TestSyncFromScanner_ImportsNewZipAndSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sync.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer func() { _ = database.Close() }()

	exportsDir := filepath.Join(dir, "downloads")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(exportsDir, "data-2026-03-02-claude.zip")
	writeSyntheticZipExport(t, zipPath, "conversations.json", "sync-conv-1")

	scanner := &discovery.Scanner{}
	scanner.AddSearchPath(exportsDir)

	s1, err := SyncFromScanner(context.Background(), database, scanner)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if s1.FilesImported != 1 || s1.ConversationsImported != 1 {
		t.Fatalf("unexpected first sync stats: %+v", s1)
	}

	s2, err := SyncFromScanner(context.Background(), database, scanner)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if s2.FilesImported != 0 || s2.FilesSkipped != 1 {
		t.Fatalf("expected unchanged zip to be skipped on second sync, got: %+v", s2)
	}
}
