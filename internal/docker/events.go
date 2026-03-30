// events.go streams Docker container lifecycle events from the daemon.
package docker

import (
	"context"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// EventInfo represents a single Docker container lifecycle event.
// When Disconnected is true, the event stream has been interrupted (daemon
// restart, network drop, etc.) and no further events will arrive until the
// stream is restarted. All other fields are zero in this case.
type EventInfo struct {
	Time          time.Time
	Action        string // start, stop, die, create, destroy, kill, restart, pause, unpause
	ContainerName string
	ContainerID   string // short 12-char ID
	Disconnected  bool   // sentinel: stream was interrupted by an error
}

// StreamEvents streams container lifecycle events from the Docker daemon.
// It includes events from the past hour so the initial view is populated.
// The returned channel is closed when ctx is cancelled.
func (c *Client) StreamEvents(ctx context.Context) <-chan EventInfo {
	ch := make(chan EventInfo, 64)
	go func() {
		defer close(ch)
		f := filters.NewArgs()
		f.Add("type", "container")
		since := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
		msgCh, errCh := c.cli.Events(ctx, events.ListOptions{
			Filters: f,
			Since:   since,
		})
		for {
			select {
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				name := msg.Actor.Attributes["name"]
				id := msg.Actor.ID
				if len(id) > 12 {
					id = id[:12]
				}
				select {
				case ch <- EventInfo{
					Time:          time.Unix(msg.Time, 0),
					Action:        string(msg.Action),
					ContainerName: name,
					ContainerID:   id,
				}:
				case <-ctx.Done():
					return
				}
			case err, ok := <-errCh:
				if ok && err != nil {
					// Daemon dropped the stream. Send a sentinel so the TUI
					// can display a "disconnected" indicator, then close.
					select {
					case ch <- EventInfo{Disconnected: true}:
					default:
					}
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
