// interface.go defines the DockerClient interface.
// Both the real Client and DemoClient implement this, allowing the TUI
// to work identically in live and demo modes.
package docker

// DockerClient is the interface the TUI depends on.
// The real Client and DemoClient both satisfy it.
type DockerClient interface {
	ListContainers() ([]ContainerInfo, error)
	ListNetworks() ([]NetworkInfo, error)
	ListImages() ([]ImageInfo, error)
	FetchStats(id string) (cpu float64, memMB float64, err error)
	StartContainer(id string) error
	StopContainer(id string) error
	RestartContainer(id string) error
	Close()
}
