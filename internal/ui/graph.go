// graph.go renders an ASCII network topology graph.
// Each network is shown as a labeled row with containers joined by dashes.
package ui

import (
	"fmt"
	"strings"

	"github.com/yourusername/dockviz-cli/internal/docker"
)

// RenderNetworkGraph builds a multi-line ASCII representation of Docker networks.
//
// Example output:
//
//	bridge : nginx ─── postgres ─── redis
//	host   : (no containers)
func RenderNetworkGraph(networks []docker.NetworkInfo) string {
	if len(networks) == 0 {
		return "  No networks found."
	}

	var sb strings.Builder
	for _, n := range networks {
		label := fmt.Sprintf("  %-12s", n.Name)
		if len(n.Containers) == 0 {
			sb.WriteString(label + ": (no containers)\n")
			continue
		}
		sb.WriteString(label + ": " + strings.Join(n.Containers, " \u2500\u2500\u2500 ") + "\n")
	}
	return sb.String()
}
