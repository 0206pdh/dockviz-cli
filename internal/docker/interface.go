// interface.go defines the DockerClient interface.
// Both the real Client and DemoClient implement this, allowing the TUI
// to work identically in live and demo modes.
package docker

import "context"

// DockerClient is the interface the TUI depends on.
// The real Client and DemoClient both satisfy it.
type DockerClient interface {
	ListContainers() ([]ContainerInfo, error)
	ListImages() ([]ImageInfo, error)
	FetchStats(id string) (cpu float64, memMB float64, err error)
	// RemoveContainer force-removes a container (running or stopped).
	RemoveContainer(id string) error
	// RemoveImage removes a local image by ID or tag.
	RemoveImage(id string) error
	// DiskUsage returns a docker-system-df-style breakdown of images,
	// containers, volumes, and build cache, plus a Logs category that
	// `docker system df` itself doesn't account for.
	DiskUsage() (DiskUsageInfo, error)
	// PruneImages removes dangling (untagged) images.
	PruneImages() (freedMB float64, err error)
	// PruneContainers removes stopped containers.
	PruneContainers() (freedMB float64, err error)
	// PruneVolumes removes volumes not attached to any container.
	PruneVolumes() (freedMB float64, err error)
	// PruneBuildCache removes unused build cache layers.
	PruneBuildCache() (freedMB float64, err error)
	// PruneLogs truncates container log files. Never removes a container.
	PruneLogs() (freedMB float64, err error)
	// StreamLogs streams the last 50 lines + live log output for a container.
	// Cancel the provided context to stop the stream.
	StreamLogs(ctx context.Context, id string) <-chan LogLine
	// StreamEvents streams container lifecycle events (start, stop, die, etc.).
	// Includes the past hour of events on first call, then live from there.
	// Cancel the provided context to stop the stream.
	StreamEvents(ctx context.Context) <-chan EventInfo
	Close()
}
