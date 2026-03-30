// networks.go fetches Docker network data and builds adjacency info for the graph renderer.
package docker

import (
	"sort"
	"strings"

	"github.com/docker/docker/api/types/network"
)

// ContainerEndpoint holds a container attached to a network with its IPv4 address on that network.
type ContainerEndpoint struct {
	Name string
	IPv4 string // e.g. "172.20.0.2" (empty for host/none drivers)
}

// NetworkInfo represents one Docker network with its IPAM config and attached containers.
type NetworkInfo struct {
	ID         string
	Name       string
	Driver     string
	Subnet     string              // e.g. "172.20.0.0/16" (empty if no IPAM config)
	Containers []ContainerEndpoint // containers attached to this network
}

// ListNetworks returns all Docker networks with their connected containers and IPAM metadata.
func (c *Client) ListNetworks() ([]NetworkInfo, error) {
	networks, err := c.cli.NetworkList(c.ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]NetworkInfo, 0, len(networks))
	for _, n := range networks {
		// Extract subnet from IPAM config.
		subnet := ""
		if len(n.IPAM.Config) > 0 {
			subnet = n.IPAM.Config[0].Subnet
		}

		// Extract containers with their IPv4 addresses.
		// IPv4Address comes as "172.20.0.2/16" — strip the CIDR mask.
		endpoints := make([]ContainerEndpoint, 0, len(n.Containers))
		for _, ep := range n.Containers {
			ipv4 := ep.IPv4Address
			if idx := strings.Index(ipv4, "/"); idx != -1 {
				ipv4 = ipv4[:idx]
			}
			endpoints = append(endpoints, ContainerEndpoint{
				Name: ep.Name,
				IPv4: ipv4,
			})
		}

		result = append(result, NetworkInfo{
			ID:         n.ID[:12],
			Name:       n.Name,
			Driver:     n.Driver,
			Subnet:     subnet,
			Containers: endpoints,
		})
	}
	sysOrder := map[string]int{"bridge": 0, "host": 1, "none": 2}
	sort.SliceStable(result, func(i, j int) bool {
		ri, iSys := sysOrder[result[i].Name]
		rj, jSys := sysOrder[result[j].Name]
		if iSys != jSys {
			return !iSys
		}
		if iSys {
			return ri < rj
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
