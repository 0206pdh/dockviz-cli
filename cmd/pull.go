// pull.go registers the `dockviz pull <image>` subcommand.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourusername/dockviz-cli/internal/docker"
	"github.com/yourusername/dockviz-cli/internal/tui"
)

var pullCmd = &cobra.Command{
	Use:   "pull <image>",
	Short: "Pull a Docker image with a live layer-progress dashboard",
	Example: `  dockviz pull nginx:alpine
  dockviz pull postgres:16
  dockviz pull ubuntu:24.04`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]

		dc, err := docker.NewClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: cannot connect to Docker daemon:", err)
			os.Exit(1)
		}

		return tui.StartPull(dc, ref)
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
