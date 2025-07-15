package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/neilberkman/shannon/internal/db"
	"github.com/neilberkman/shannon/internal/models"
	"github.com/neilberkman/shannon/internal/search"
)

// assertViewMatchesSnapshot compares the model's view with a golden file.
// If the UPDATE_SNAPSHOTS environment variable is set, it updates the golden file.
func assertViewMatchesSnapshot(t *testing.T, view string, snapshotName string) {
	t.Helper()

	snapshotPath := filepath.Join("testdata", snapshotName+".golden")

	if os.Getenv("UPDATE_SNAPSHOTS") != "" {
		err := os.MkdirAll(filepath.Dir(snapshotPath), 0755)
		if err != nil {
			t.Fatalf("failed to create snapshot directory: %v", err)
		}
		err = os.WriteFile(snapshotPath, []byte(view), 0644)
		if err != nil {
			t.Fatalf("failed to update snapshot: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to read snapshot: %v", err)
	}

	// Normalize line endings for comparison
	expectedStr := strings.ReplaceAll(string(expected), "\r\n", "\n")
	actualStr := strings.ReplaceAll(view, "\r\n", "\n")

	if expectedStr != actualStr {
		t.Errorf("view does not match snapshot %s", snapshotName)
		t.Logf("EXPECTED:\n%s", expectedStr)
		t.Logf("ACTUAL:\n%s", actualStr)
	}
}

// setupTestDB creates a temporary in-memory database with synthetic data for testing.
func setupTestDB(t *testing.T) *search.Engine {
	t.Helper()

	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory db: %v", err)
	}

	engine := search.NewEngine(database)

	// Insert synthetic data with fixed dates for consistent testing
	fixedTime := time.Date(2025, 6, 25, 10, 0, 0, 0, time.UTC)
	convs := []*models.Conversation{
		{ID: 1, UUID: "uuid-1", Name: "Test Conversation 1", CreatedAt: fixedTime.Add(-2 * time.Hour), UpdatedAt: fixedTime.Add(-1 * time.Hour), MessageCount: 5},
		{ID: 2, UUID: "uuid-2", Name: "Another Test Convo", CreatedAt: fixedTime.Add(-3 * time.Hour), UpdatedAt: fixedTime.Add(-2 * time.Hour), MessageCount: 10},
		{ID: 3, UUID: "uuid-3", Name: "Final Test", CreatedAt: fixedTime.Add(-4 * time.Hour), UpdatedAt: fixedTime.Add(-3 * time.Hour), MessageCount: 2},
	}

	for _, c := range convs {
		_, err := engine.DB().Exec("INSERT INTO conversations (id, uuid, name, created_at, updated_at, message_count) VALUES (?, ?, ?, ?, ?, ?)", c.ID, c.UUID, c.Name, c.CreatedAt, c.UpdatedAt, c.MessageCount)
		if err != nil {
			t.Fatalf("failed to insert conversation: %v", err)
		}
	}

	return engine
}

func TestBrowseView_Initial(t *testing.T) {
	engine := setupTestDB(t)
	model := newBrowseModel(engine)
	model.list.SetSize(80, 24) // Set a fixed size for consistent test output

	view := model.View()
	assertViewMatchesSnapshot(t, view, "browse_initial")
}

func TestBrowseView_NavigateDown(t *testing.T) {
	engine := setupTestDB(t)
	model := newBrowseModel(engine)
	model.list.SetSize(80, 24)

	// Send a 'down' key press
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	updatedModel, _ := model.Update(msg)
	model = updatedModel.(browseModel)

	view := model.View()
	assertViewMatchesSnapshot(t, view, "browse_navigate_down")
}

func TestBrowseView_NavigateUp(t *testing.T) {
	engine := setupTestDB(t)
	model := newBrowseModel(engine)
	model.list.SetSize(80, 24)

	// Go down, then up
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updatedModel.(browseModel)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updatedModel.(browseModel)

	view := model.View()
	// Should be the same as the initial view
	assertViewMatchesSnapshot(t, view, "browse_initial")
}
