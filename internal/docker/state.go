// state.go defines ContainerState, which tracks the health of a container
// as derived from the live event stream.
//
// Placing this type in the docker package (not tui or ui) avoids the import
// cycle that would occur if ui/graph.go needed to reference a tui type.
// Both internal/ui and internal/tui already import internal/docker, so this
// is safe for both consumers.
package docker

import "time"

// ContainerState tracks the last-known health of a container derived from
// the event stream. It is keyed by ContainerName in Model.ContainerStates.
//
// State transition table:
//
//	container/start   → Status="running",    RestartCount reset to 0
//	container/restart → Status="restarting", RestartCount incremented
//	container/die     → Status="dead",       ExitCode/OOMKilled populated
//	container/destroy → entry deleted from map
type ContainerState struct {
	Status       string    // "running" | "dead" | "restarting"
	ExitCode     int       // 0=clean, 137=SIGKILL, 1=app error
	OOMKilled    bool      // true when the die event had oomKilled="true"
	RestartCount int       // session-scoped; resets when the app restarts
	UpdatedAt    time.Time
}
