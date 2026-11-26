// Package docker wraps the Docker SDK client.
// client.go provides a singleton Docker client and basic connectivity check.
package docker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// Timeouts for calls on the periodic refresh path. Without them a single
// unresponsive daemon call blocks the refresh forever, and the TUI skips new
// refreshes while one is still in flight. Long-running user-initiated
// operations (prune, remove) and streams are deliberately not bounded here.
const (
	listTimeout    = 10 * time.Second
	statsTimeout   = 10 * time.Second
	inspectTimeout = 5 * time.Second
)

// Client wraps the Docker SDK client with a shared context.
type Client struct {
	cli                *client.Client
	ctx                context.Context
	daemonHost         string
	resourceLimitsMu   sync.Mutex
	resourceLimitsByID map[string]resourceLimits
}

type resourceLimits struct {
	cpuLimit      float64
	memoryLimitMB float64
	known         bool
}

// NewClient creates and validates a connection to the Docker daemon.
// host overrides the DOCKER_HOST environment variable when non-empty
// (e.g. "tcp://192.168.1.100:2375"). Pass "" to use the env var or default socket.
func NewClient(host string) (*Client, error) {
	effectiveHost := host
	if effectiveHost == "" {
		effectiveHost = os.Getenv("DOCKER_HOST")
	}

	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	ctx := context.Background()

	// Ping the daemon to confirm connectivity.
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cannot reach Docker daemon — is Docker running? %w", err)
	}

	return &Client{
		cli:                cli,
		ctx:                ctx,
		daemonHost:         effectiveHost,
		resourceLimitsByID: make(map[string]resourceLimits),
	}, nil
}

// Close releases the underlying HTTP connection.
func (c *Client) Close() {
	_ = c.cli.Close()
}
