// Package docker wraps the Docker SDK client.
// client.go provides a singleton Docker client and basic connectivity check.
package docker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
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
	endpoint           Endpoint
	resourceLimitsMu   sync.Mutex
	resourceLimitsByID map[string]resourceLimits
}

type resourceLimits struct {
	cpuLimit      float64
	memoryLimitMB float64
	known         bool
}

// NewClient creates and validates a connection to the Docker daemon.
// host is the --host flag and dockerContext the --context flag; either may be
// empty. The endpoint is resolved with the docker CLI's precedence (see
// ResolveEndpoint), so Docker contexts such as Docker Desktop's
// "desktop-linux" or Colima's "colima" work without DOCKER_HOST.
func NewClient(host, dockerContext string) (*Client, error) {
	ep, err := ResolveEndpoint(EndpointOptions{Host: host, Context: dockerContext})
	if err != nil {
		return nil, err
	}

	opts, err := clientOpts(ep)
	if err != nil {
		return nil, err
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client (%s): %w", ep.Source, err)
	}

	ctx := context.Background()

	// Ping the daemon to confirm connectivity.
	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("cannot reach Docker daemon at %s (from %s) — is Docker running? %w", cli.DaemonHost(), ep.Source, err)
	}

	return &Client{
		cli:                cli,
		ctx:                ctx,
		daemonHost:         cli.DaemonHost(),
		endpoint:           ep,
		resourceLimitsByID: make(map[string]resourceLimits),
	}, nil
}

// clientOpts builds SDK options for a resolved endpoint. Context endpoints
// deliberately skip client.FromEnv: like the docker CLI, a context's own
// host and TLS files win over DOCKER_HOST/DOCKER_CERT_PATH/DOCKER_TLS_VERIFY.
func clientOpts(ep Endpoint) ([]client.Opt, error) {
	if !ep.FromContext() {
		opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
		if ep.Host != "" {
			opts = append(opts, client.WithHost(ep.Host))
		}
		return opts, nil
	}

	// A fresh transport: WithHost configures its socket dialer, and the SDK
	// switches to https when the transport carries a TLS config.
	transport := &http.Transport{}
	if ep.TLS != nil {
		cfg, err := tlsconfig.Client(tlsconfig.Options{
			CAFile:             ep.TLS.CAFile,
			CertFile:           ep.TLS.CertFile,
			KeyFile:            ep.TLS.KeyFile,
			InsecureSkipVerify: ep.TLS.SkipTLSVerify,
			ExclusiveRootPools: true,
		})
		if err != nil {
			return nil, fmt.Errorf("docker context %q TLS config: %w", ep.Context, err)
		}
		transport.TLSClientConfig = cfg
	}
	return []client.Opt{
		client.WithHTTPClient(&http.Client{Transport: transport}),
		client.WithHost(ep.Host),
		client.WithVersionFromEnv(),
		client.WithAPIVersionNegotiation(),
	}, nil
}

// NewSDKClient returns a raw Docker SDK client for the same endpoint
// NewClient would pick, without pinging. Validation scenarios and
// integration tests use it to inspect daemon state that the DockerClient
// interface does not expose.
func NewSDKClient(host, dockerContext string) (*client.Client, Endpoint, error) {
	ep, err := ResolveEndpoint(EndpointOptions{Host: host, Context: dockerContext})
	if err != nil {
		return nil, Endpoint{}, err
	}
	opts, err := clientOpts(ep)
	if err != nil {
		return nil, Endpoint{}, err
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, Endpoint{}, fmt.Errorf("failed to create Docker client (%s): %w", ep.Source, err)
	}
	return cli, ep, nil
}

// Endpoint returns how this client's daemon address was chosen.
func (c *Client) Endpoint() Endpoint { return c.endpoint }

// Close releases the underlying HTTP connection.
func (c *Client) Close() {
	_ = c.cli.Close()
}
