// logs.go provides real-time log streaming for a container.
// It wraps the Docker ContainerLogs API with a simple channel-based interface
// so the TUI can receive log lines asynchronously via Bubble Tea commands.
package docker

import (
	"bufio"
	"context"

	"github.com/docker/docker/api/types/container"
)

// LogLine carries a single line of log output from a container.
type LogLine struct {
	Text string
}

// StreamLogs streams the last 50 lines of existing logs followed by live output
// for the given container ID. Log lines are sent on the returned channel.
// Cancel the provided context to stop the goroutine and close the channel.
func (c *Client) StreamLogs(ctx context.Context, id string) <-chan LogLine {
	ch := make(chan LogLine, 100)
	go func() {
		defer close(ch)

		rc, err := c.cli.ContainerLogs(ctx, id, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Tail:       "50",
			Timestamps: false,
		})
		if err != nil {
			ch <- LogLine{Text: "Error: " + err.Error()}
			return
		}
		defer rc.Close()

		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			line := scanner.Text()
			// Docker multiplexed log streams prepend an 8-byte header
			// (stream type + 3 padding bytes + 4-byte length). Strip it.
			if len(line) > 8 {
				line = line[8:]
			}
			select {
			case ch <- LogLine{Text: line}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
