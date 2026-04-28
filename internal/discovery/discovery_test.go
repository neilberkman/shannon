package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

// minimal but plausibly-valid Claude export payload — must be > 1KB to pass
// the size gate in isLikelyClaudeExport.
func writeFakeExport(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `[{"uuid":"00000000-0000-0000-0000-000000000000","name":"Test","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","chat_messages":[{"uuid":"11111111-1111-1111-1111-111111111111","text":"hi","sender":"human","created_at":"2026-01-01T00:00:00Z"}]}]`
	// pad to exceed the 1KB threshold without breaking JSON validity (whitespace
	// after the array is allowed by encoding/json)
	pad := make([]byte, 2048)
	for i := range pad {
		pad[i] = ' '
	}
	if err := os.WriteFile(path, append([]byte(body), pad...), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanDirectory_DataFolderPrefixVariants(t *testing.T) {
	tmp := t.TempDir()

	// Old format Anthropic used through early 2026
	oldDir := "data-2026-03-25-22-41-40-batch-0000"
	// New UUID-style format observed 2026-04-28
	newDir := "data-1a7fb994-2c78-4290-9ffe-ac4af4b22452-1777408945-628929f7-batch-0000"
	// Should be ignored — starts with "data-" but has no conversations.json
	emptyDir := "data-empty-batch-0000"
	// Should be ignored — wrong prefix
	unrelatedDir := "random-folder"

	writeFakeExport(t, filepath.Join(tmp, oldDir, "conversations.json"))
	writeFakeExport(t, filepath.Join(tmp, newDir, "conversations.json"))
	if err := os.MkdirAll(filepath.Join(tmp, emptyDir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeExport(t, filepath.Join(tmp, unrelatedDir, "conversations.json"))

	s := &Scanner{searchPaths: []string{tmp}}
	exports, err := s.ScanForExports()
	if err != nil {
		t.Fatalf("ScanForExports: %v", err)
	}

	got := map[string]bool{}
	for _, e := range exports {
		rel, _ := filepath.Rel(tmp, e.Path)
		got[rel] = true
	}

	for _, want := range []string{
		filepath.Join(oldDir, "conversations.json"),
		filepath.Join(newDir, "conversations.json"),
	} {
		if !got[want] {
			t.Errorf("expected to discover %q, got %v", want, got)
		}
	}

	for _, reject := range []string{
		filepath.Join(emptyDir, "conversations.json"),
		filepath.Join(unrelatedDir, "conversations.json"),
	} {
		if got[reject] {
			t.Errorf("did not expect to discover %q", reject)
		}
	}
}
