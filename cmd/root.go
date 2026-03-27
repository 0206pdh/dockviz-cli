// Package cmd defines the CLI entry point using Cobra.
// root.go registers the root command and delegates to the TUI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourusername/dockviz-cli/internal/tui"
)

var demoMode bool

var rootCmd = &cobra.Command{
	Use:   "dockviz",
	Short: "Interactive Docker environment visualizer",
	Long: `dockviz-cli is a TUI dashboard for monitoring your Docker environment.

It shows real-time container stats, network topology, and lets you
start/stop containers directly from the terminal.

Run with --demo to preview the dashboard without a running Docker daemon.`,
	Example: `  dockviz           # connect to local Docker daemon
  dockviz --demo    # run with simulated data (no Docker required)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Start(demoMode)
	},
}

func init() {
	rootCmd.Flags().BoolVar(&demoMode, "demo", false, "Run with simulated data (no Docker daemon required)")
}

// Execute is called from main.go. It runs the root Cobra command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
