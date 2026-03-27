// networks.go fetches Docker network data and builds adjacency info for the graph renderer.
package docker

import (
	"github.com/docker/docker/api/types/network"
)

// NetworkInfo represents one Docker network and the containers attached to it.
type NetworkInfo struct {
	ID         string
	Name       string
	Driver     string   // bridge, host, overlay, etc.
	Containers []string // container names on this network
}

// ListNetworks returns all Docker networks with their connected containers.
func (c *Client) ListNetworks() ([]NetworkInfo, error) {
	networks, err := c.cli.NetworkList(c.ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]NetworkInfo, 0, len(networks))
	for _, n := range networks {
		containers := make([]string, 0, len(n.Containers))
		for _, ep := range n.Containers {
			containers = append(containers, ep.Name)
		}
		result = append(result, NetworkInfo{
			ID:         n.ID[:12],
			Name:       n.Name,
			Driver:     n.Driver,
			Containers: containers,
		})
	}
	return result, nil
}
