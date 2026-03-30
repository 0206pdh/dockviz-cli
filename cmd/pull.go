// pull.go registers the `dockviz pull <image>` subcommand.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/0206pdh/dockviz-cli/internal/docker"
	"github.com/0206pdh/dockviz-cli/internal/tui"
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
			return fmt.Errorf("cannot connect to Docker daemon: %w", err)
		}

		return tui.StartPull(dc, ref)
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
