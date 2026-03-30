// graph.go renders Docker network topology as ASCII art.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/0206pdh/dockviz-cli/internal/docker"
)

// RenderNetworkGraph builds a compact ASCII representation of Docker networks.
// Container nodes are coloured based on their ContainerState:
//
//	● green  = running
//	◑ yellow = restarting
//	✗ red    = dead
//	○ gray   = unknown / no event data yet
//
// Pass a nil or empty states map to render without colour decoration.
func RenderNetworkGraph(networks []docker.NetworkInfo, states map[string]docker.ContainerState) string {
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
		nodes := make([]string, len(n.Containers))
		for i, ep := range n.Containers {
			nodes[i] = containerLabel(ep.Name, states)
		}
		sb.WriteString(label + ": " + strings.Join(nodes, " \u2500\u2500\u2500 ") + "\n")
	}
	return sb.String()
}

// containerLabel returns a coloured "icon name" string for a container node.
// The colour and icon reflect the container's last-known state from the event stream.
func containerLabel(name string, states map[string]docker.ContainerState) string {
	cs, ok := states[name]
	if !ok {
		return lipgloss.NewStyle().Foreground(ColorGray).Render("○ " + name)
	}
	switch cs.Status {
	case "dead":
		return lipgloss.NewStyle().Foreground(ColorRed).Render("✗ " + name)
	case "restarting":
		return lipgloss.NewStyle().Foreground(ColorYellow).Render("◑ " + name)
	case "running":
		return lipgloss.NewStyle().Foreground(ColorGreen).Render("● " + name)
	default:
		return lipgloss.NewStyle().Foreground(ColorGray).Render("○ " + name)
	}
}
