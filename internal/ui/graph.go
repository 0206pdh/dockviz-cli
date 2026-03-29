// graph.go renders Docker network topology as ASCII art.
package ui

import (
	"fmt"
	"strings"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

// RenderNetworkGraph builds a compact ASCII representation of Docker networks.
// Used as a fallback; the TUI renders a richer view directly.
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
		names := make([]string, len(n.Containers))
		for i, ep := range n.Containers {
			names[i] = ep.Name
		}
		sb.WriteString(label + ": " + strings.Join(names, " \u2500\u2500\u2500 ") + "\n")
	}
	return sb.String()
}
