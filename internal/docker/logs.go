// logs.go provides real-time log streaming for a container.
// It wraps the Docker ContainerLogs API with a simple channel-based interface
// so the TUI can receive log lines asynchronously via Bubble Tea commands.
package docker

import (
	"bufio"
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// LogLine carries a single line of log output from a container.
type LogLine struct {
	Text string
}

// StreamLogs streams the last 50 lines of existing logs followed by live output
// for the given container ID. Log lines are sent on the returned channel.
// Cancel the provided context to stop the goroutine and close the channel.
//
// Docker log streams use an 8-byte multiplexed framing (stdcopy format) for
// non-TTY containers. stdcopy.StdCopy demultiplexes the stream correctly,
// avoiding the data corruption that naive byte-slicing causes.
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

		// Pipe stdout and stderr through stdcopy to strip the 8-byte framing
		// headers, then scan the demultiplexed output line by line.
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			stdcopy.StdCopy(pw, pw, rc) //nolint:errcheck
		}()

		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			select {
			case ch <- LogLine{Text: scanner.Text()}:
			case <-ctx.Done():
				pr.Close()
				return
			}
		}
	}()
	return ch
}
