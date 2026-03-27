// Package cmd defines the CLI entry point using Cobra.
// root.go registers the root command and delegates to the TUI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourusername/dockviz-cli/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "dockviz",
	Short: "Interactive Docker environment visualizer",
	Long: `dockviz-cli is a TUI dashboard that shows real-time Docker
container stats, network topology, and lets you control containers
directly from your terminal.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Start()
	},
}

// Execute is called from main.go. It runs the root Cobra command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
