package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

type Dirs struct {
	Config string
	Data   string
}

func GetAppDirs(appName string) (*Dirs, error) {
	var dirs Dirs

	switch runtime.GOOS {
	case "linux":
		dirs.Config = getLinuxConfigDir(appName)
		dirs.Data = getLinuxDataDir(appName)
	case "darwin":
		dirs.Config = getMacConfigDir(appName)
		dirs.Data = getMacDataDir(appName)
	case "windows":
		dirs.Config = getWindowsConfigDir(appName)
		dirs.Data = getWindowsDataDir(appName)
	default:
		// Fallback to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dirs.Config = filepath.Join(home, "."+appName)
		dirs.Data = dirs.Config
	}

	// Ensure directories exist
	if err := os.MkdirAll(dirs.Config, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dirs.Data, 0755); err != nil {
		return nil, err
	}

	return &dirs, nil
}

func getLinuxConfigDir(appName string) string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", appName)
}

func getLinuxDataDir(appName string) string {
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", appName)
}

func getMacConfigDir(appName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", appName)
}

func getMacDataDir(appName string) string {
	return getMacConfigDir(appName)
}

func getWindowsConfigDir(appName string) string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming", appName)
}

func getWindowsDataDir(appName string) string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", appName)
}

// GetDownloadsDir returns the platform-specific Downloads directory
func GetDownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		// Try USERPROFILE/Downloads first, fallback to home/Downloads
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return filepath.Join(userProfile, "Downloads"), nil
		}
		return filepath.Join(home, "Downloads"), nil
	case "darwin", "linux":
		return filepath.Join(home, "Downloads"), nil
	default:
		return filepath.Join(home, "Downloads"), nil
	}
}
