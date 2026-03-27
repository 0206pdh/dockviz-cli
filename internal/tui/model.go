// model.go defines the Bubble Tea Model — the single source of truth for all TUI state.
//
// Bubble Tea follows The Elm Architecture (TEA):
//   Model  — what the app knows (state)
//   Update — how state changes in response to messages
//   View   — how state is rendered to the terminal
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/0206pdh/dockviz-cli/internal/docker"
)

// Panel represents which section of the dashboard is active.
type Panel int

const (
	PanelContainers Panel = iota
	PanelNetworks
	PanelImages
)

// View represents the current screen displayed.
type View int

const (
	ViewDashboard View = iota
	ViewDetail
	ViewLogs
)

// tickMsg is sent on each refresh interval.
type tickMsg time.Time

// dataMsg carries freshly fetched Docker data back to Update.
type dataMsg struct {
	containers []docker.ContainerInfo
	networks   []docker.NetworkInfo
	images     []docker.ImageInfo
	err        error
}

// Model is the entire application state.
type Model struct {
	// Build-time version string (e.g. "v0.2.3")
	version string

	// Docker connection (real or demo)
	docker docker.DockerClient

	// Current data
	containers []docker.ContainerInfo
	networks   []docker.NetworkInfo
	images     []docker.ImageInfo

	// Navigation
	activePanel  Panel
	activeView   View
	cursor       int // selected row index in active list
	selectedID   string

	// UI helpers
	keys    KeyMap
	spinner spinner.Model
	loading bool
	err     error

	// Terminal dimensions
	width  int
	height int

	// Delete confirmation overlay
	confirmDelete bool // true when the "confirm delete" dialog is visible

	// refreshing is true between pressing 'r' and the data arriving back
	refreshing bool

	// CPU sparkline history: containerID → last 10 CPU% readings
	history map[string][]float64

	// Log streaming state
	logs      []string            // accumulated log lines for the current container
	logScroll int                 // scroll offset (0 = top, len(logs) = bottom)
	logCh     <-chan docker.LogLine // channel receiving live log lines
	logCancel context.CancelFunc  // call to stop the streaming goroutine
}

// Init implements tea.Model. It kicks off the spinner and the first data fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchDataCmd(m.docker), tickCmd())
}

// newModel creates the initial Model. Accepts any DockerClient (real or demo).
func newModel(dc docker.DockerClient, version string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		version:     version,
		docker:      dc,
		activePanel: PanelContainers,
		activeView:  ViewDashboard,
		keys:        DefaultKeyMap(),
		spinner:     sp,
		loading:     true,
		history:     make(map[string][]float64),
	}
}

// tickCmd returns a Cmd that fires a tickMsg after the refresh interval.
func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchDataCmd fetches all Docker data off the main goroutine and returns a dataMsg.
func fetchDataCmd(dc docker.DockerClient) tea.Cmd {
	return func() tea.Msg {
		containers, err := dc.ListContainers()
		if err != nil {
			return dataMsg{err: err}
		}
		networks, err := dc.ListNetworks()
		if err != nil {
			return dataMsg{err: err}
		}
		images, err := dc.ListImages()
		if err != nil {
			return dataMsg{err: err}
		}
		return dataMsg{
			containers: containers,
			networks:   networks,
			images:     images,
		}
	}
}
