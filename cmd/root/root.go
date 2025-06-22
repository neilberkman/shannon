package root

import (
	"fmt"
	"os"

	"github.com/neilberkman/shannon/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
)

var (
	// Version information - will be set by goreleaser
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// RootCmd represents the base command
var RootCmd = &cobra.Command{
	Use:   "shannon",
	Short: "Search your AI conversation history",
	Long: `Shannon is a powerful CLI tool for searching through your exported AI conversation history.
	
Named after Claude Shannon, the father of information theory, this tool provides full-text 
search capabilities with advanced query features, preserves conversation threading, and 
offers both CLI and TUI interfaces for different use cases.

Quick start:
  shannon discover                    # Find Claude exports
  shannon import conversations.json   # Import an export
  shannon search "python"             # Search conversations  
  shannon recent                      # Show recent activity
  shannon tui                         # Interactive interface`,
	Version: Version,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize configuration
		if err := config.Init(); err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/shannon/config.yaml)")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Bind flags to viper
	if err := viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		panic(fmt.Sprintf("failed to bind flag: %v", err))
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	}

	viper.AutomaticEnv() // read in environment variables that match
}
