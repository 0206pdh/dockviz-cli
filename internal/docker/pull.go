// pull.go streams Docker image pull events and parses per-layer progress.
package docker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
)

// LayerStatus represents the download state of a single image layer.
type LayerStatus struct {
	ID      string
	Status  string // "Waiting", "Downloading", "Pull complete", "Already exists", etc.
	Current int64  // bytes downloaded
	Total   int64  // total bytes
}

// PullEvent is sent to the caller on every progress update.
type PullEvent struct {
	Layers  []LayerStatus // ordered snapshot of all known layers
	Overall string        // e.g. "Status: Downloaded newer image for nginx:alpine"
	Done    bool
	Err     error
}

// PullImage pulls an image and streams PullEvents into the returned channel.
// The channel is closed when the pull finishes or errors.
func (c *Client) PullImage(ref string) <-chan PullEvent {
	ch := make(chan PullEvent)

	go func() {
		defer close(ch)

		rc, err := c.cli.ImagePull(c.ctx, ref, image.PullOptions{})
		if err != nil {
			ch <- PullEvent{Err: fmt.Errorf("pull failed: %w", err)}
			return
		}
		defer rc.Close()

		// layerOrder keeps insertion order so the display is stable.
		layerOrder := []string{}
		layers := map[string]*LayerStatus{}

		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			var evt struct {
				Status         string `json:"status"`
				ID             string `json:"id"`
				ProgressDetail struct {
					Current int64 `json:"current"`
					Total   int64 `json:"total"`
				} `json:"progressDetail"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
				continue
			}

			if evt.Error != "" {
				ch <- PullEvent{Err: fmt.Errorf("%s", evt.Error)}
				return
			}

			// Events without an ID are top-level status messages (digest, final status).
			if evt.ID == "" {
				ch <- PullEvent{
					Layers:  snapshot(layerOrder, layers),
					Overall: evt.Status,
					Done:    isFinishedStatus(evt.Status),
				}
				if isFinishedStatus(evt.Status) {
					return
				}
				continue
			}

			// Per-layer event
			if _, ok := layers[evt.ID]; !ok {
				layerOrder = append(layerOrder, evt.ID)
				layers[evt.ID] = &LayerStatus{ID: evt.ID}
			}
			l := layers[evt.ID]
			l.Status = evt.Status
			l.Current = evt.ProgressDetail.Current
			l.Total = evt.ProgressDetail.Total

			ch <- PullEvent{Layers: snapshot(layerOrder, layers)}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			ch <- PullEvent{Err: err}
		}
	}()

	return ch
}

func snapshot(order []string, m map[string]*LayerStatus) []LayerStatus {
	out := make([]LayerStatus, 0, len(order))
	for _, id := range order {
		out = append(out, *m[id])
	}
	return out
}

func isFinishedStatus(s string) bool {
	return len(s) > 7 && s[:7] == "Status:"
}
