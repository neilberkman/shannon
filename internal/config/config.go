package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/neilberkman/shannon/pkg/platform"
)

type Config struct {
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`

	Search struct {
		MaxResults    int  `mapstructure:"max_results"`
		ShowSnippets  bool `mapstructure:"show_snippets"`
		SnippetLength int  `mapstructure:"snippet_length"`
	} `mapstructure:"search"`

	UI struct {
		Theme          string `mapstructure:"theme"`
		PageSize       int    `mapstructure:"page_size"`
		HighlightColor string `mapstructure:"highlight_color"`
	} `mapstructure:"ui"`

	Import struct {
		BatchSize int  `mapstructure:"batch_size"`
		Verbose   bool `mapstructure:"verbose"`
	} `mapstructure:"import"`
}

var (
	cfg  *Config
	dirs *platform.Dirs
)

func Init() error {
	// Get platform-specific directories
	appDirs, err := platform.GetAppDirs("shannon")
	if err != nil {
		return fmt.Errorf("failed to get app directories: %w", err)
	}
	dirs = appDirs

	// Set up Viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(dirs.Config)

	// Set defaults
	setDefaults()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err != nil {
		// It's OK if the config file doesn't exist
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config: %w", err)
		}
	}

	// Unmarshal config
	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Ensure database path is set
	if cfg.Database.Path == "" {
		cfg.Database.Path = filepath.Join(dirs.Data, "claude-search.db")
	}

	return nil
}

func setDefaults() {
	// Database defaults
	viper.SetDefault("database.path", "")

	// Search defaults
	viper.SetDefault("search.max_results", 50)
	viper.SetDefault("search.show_snippets", true)
	viper.SetDefault("search.snippet_length", 200)

	// UI defaults
	viper.SetDefault("ui.theme", "dark")
	viper.SetDefault("ui.page_size", 20)
	viper.SetDefault("ui.highlight_color", "yellow")

	// Import defaults
	viper.SetDefault("import.batch_size", 1000)
	viper.SetDefault("import.verbose", false)
}

func Get() *Config {
	if cfg == nil {
		panic("config not initialized")
	}
	return cfg
}

func GetDirs() *platform.Dirs {
	if dirs == nil {
		panic("config not initialized")
	}
	return dirs
}

func SaveDefaults() error {
	configPath := filepath.Join(dirs.Config, "config.yaml")
	return viper.WriteConfigAs(configPath)
}
