package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "shannon-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Warning: failed to clean up temp dir: %v", err)
		}
	}()

	// Initialize config first
	if err := Init(); err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Test basic config loading
	cfg := Get()
	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}

	// Verify default database path is set
	if cfg.Database.Path == "" {
		t.Error("Expected database path to be set")
	}

	// Verify database path exists or can be created
	dbDir := filepath.Dir(cfg.Database.Path)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			t.Errorf("Failed to create database directory: %v", err)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	// Initialize config first
	if err := Init(); err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}
	
	cfg := Get()
	
	// Test that config has reasonable defaults
	if cfg.Database.Path == "" {
		t.Error("Database path should have a default value")
	}
	
	// The path should end with "conversations.db"
	if !filepath.IsAbs(cfg.Database.Path) {
		t.Error("Database path should be absolute")
	}
}