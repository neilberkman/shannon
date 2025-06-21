package main

import (
	"github.com/user/shannon/cmd/discover"
	"github.com/user/shannon/cmd/edit"
	"github.com/user/shannon/cmd/export"
	imports "github.com/user/shannon/cmd/import"
	"github.com/user/shannon/cmd/list"
	"github.com/user/shannon/cmd/recent"
	"github.com/user/shannon/cmd/root"
	"github.com/user/shannon/cmd/search"
	"github.com/user/shannon/cmd/stats"
	"github.com/user/shannon/cmd/tui"
	"github.com/user/shannon/cmd/view"
	"github.com/user/shannon/cmd/xargs"
)

// Version information, set during build
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Set version information
	root.Version = version
	root.Commit = commit
	root.Date = date
	root.RootCmd.Version = version

	// Add subcommands
	root.RootCmd.AddCommand(imports.ImportCmd)
	root.RootCmd.AddCommand(discover.DiscoverCmd)
	root.RootCmd.AddCommand(list.ListCmd)
	root.RootCmd.AddCommand(recent.RecentCmd)
	root.RootCmd.AddCommand(search.SearchCmd)
	root.RootCmd.AddCommand(view.ViewCmd)
	root.RootCmd.AddCommand(edit.EditCmd)
	root.RootCmd.AddCommand(export.ExportCmd)
	root.RootCmd.AddCommand(stats.StatsCmd)
	root.RootCmd.AddCommand(tui.TuiCmd)
	root.RootCmd.AddCommand(xargs.XargsCmd)

	// Execute
	root.Execute()
}
