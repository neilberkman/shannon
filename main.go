package main

import (
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
)

func main() {
	// Add subcommands
	root.RootCmd.AddCommand(imports.ImportCmd)
	root.RootCmd.AddCommand(list.ListCmd)
	root.RootCmd.AddCommand(recent.RecentCmd)
	root.RootCmd.AddCommand(search.SearchCmd)
	root.RootCmd.AddCommand(view.ViewCmd)
	root.RootCmd.AddCommand(edit.EditCmd)
	root.RootCmd.AddCommand(export.ExportCmd)
	root.RootCmd.AddCommand(stats.StatsCmd)
	root.RootCmd.AddCommand(tui.TuiCmd)

	// Execute
	root.Execute()
}
