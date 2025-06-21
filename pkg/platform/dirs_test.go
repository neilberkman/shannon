package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAppDirs(t *testing.T) {
	dirs, err := GetAppDirs("shannon-test")
	if err != nil {
		t.Fatalf("GetAppDirs failed: %v", err)
	}

	if dirs.Config == "" {
		t.Error("Config dir should return a non-empty path")
	}

	if dirs.Data == "" {
		t.Error("Data dir should return a non-empty path")
	}

	// Should be absolute paths
	if !filepath.IsAbs(dirs.Config) {
		t.Error("Config dir should return an absolute path")
	}

	if !filepath.IsAbs(dirs.Data) {
		t.Error("Data dir should return an absolute path")
	}

	// Should contain "shannon-test" in the path
	if !contains(dirs.Config, "shannon-test") {
		t.Error("Config dir should contain 'shannon-test' in the path")
	}

	if !contains(dirs.Data, "shannon-test") {
		t.Error("Data dir should contain 'shannon-test' in the path")
	}
}

func TestConfigAndDataDirsDifferent(t *testing.T) {
	dirs, err := GetAppDirs("shannon-test2")
	if err != nil {
		t.Fatalf("GetAppDirs failed: %v", err)
	}

	// On some systems they might be the same, on others different
	// This test just ensures they both work
	if dirs.Config == "" || dirs.Data == "" {
		t.Error("Both Config and Data dirs should return non-empty paths")
	}
}

func TestDirectoryCreation(t *testing.T) {
	// Test that we can actually create directories in the returned paths
	dirs, err := GetAppDirs("shannon-test3")
	if err != nil {
		t.Fatalf("GetAppDirs failed: %v", err)
	}

	// GetAppDirs should have already created the directories
	// Verify they exist
	if _, err := os.Stat(dirs.Config); os.IsNotExist(err) {
		t.Errorf("Config directory was not created: %s", dirs.Config)
	}

	if _, err := os.Stat(dirs.Data); os.IsNotExist(err) {
		t.Errorf("Data directory was not created: %s", dirs.Data)
	}
}

// Helper function
func contains(s, substr string) bool {
	return filepath.Base(s) == substr || filepath.Base(filepath.Dir(s)) == substr
}