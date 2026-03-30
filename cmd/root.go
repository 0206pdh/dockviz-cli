// Package cmd defines the CLI entry point using Cobra.
// root.go registers the root command and delegates to the TUI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/0206pdh/dockviz-cli/internal/tui"
)

var demoMode bool
var dockerHost string

// rootCmd is the base command. Its Version field is set by Execute() before
// the command tree is evaluated, so --version always reflects the build tag.
var rootCmd = &cobra.Command{
	Use:   "dockviz",
	Short: "Interactive Docker environment visualizer",
	Long: `dockviz-cli is a TUI dashboard for monitoring your Docker environment.

It shows real-time container stats, network topology, and lets you
start/stop containers directly from the terminal.

Run with --demo to preview the dashboard without a running Docker daemon.`,
	Example: `  dockviz                                  # connect to local Docker daemon
  dockviz --demo                            # run with simulated data (no Docker required)
  dockviz --host tcp://192.168.1.100:2375   # connect to a remote Docker daemon
  dockviz --version                         # print version and exit`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Start(demoMode, dockerHost, cmd.Version)
	},
}

func init() {
	rootCmd.Flags().BoolVar(&demoMode, "demo", false, "Run with simulated data (no Docker daemon required)")
	rootCmd.Flags().StringVar(&dockerHost, "host", "", "Docker daemon socket or host (e.g. tcp://192.168.1.100:2375). Overrides DOCKER_HOST env var.")
}

// Execute is called from main.go. It injects the build-time version string
// (set via -ldflags="-X main.version=<tag>") and runs the root Cobra command.
func Execute(version string) {
	// Cobra exposes --version automatically when Version is non-empty.
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
