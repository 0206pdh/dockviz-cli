// model.go defines the Bubble Tea Model — the single source of truth for all TUI state.
//
// Bubble Tea follows The Elm Architecture (TEA):
//   Model  — what the app knows (state)
//   Update — how state changes in response to messages
//   View   — how state is rendered to the terminal
package tui

import (
	"context"
	"sync"
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
	PanelEvents
)

// View represents the current screen displayed.
type View int

const (
	ViewDashboard View = iota
	ViewDetail
	ViewLogs
	ViewChart
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

	// Remote host override (empty = local daemon). Used when spawning `docker exec`.
	host string
	// demo is true when running with the DemoClient (no real daemon).
	demo bool

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
	confirmDelete   bool   // true when the "confirm delete" dialog is visible
	pendingDeleteID string // ID of the item to delete (captured at dialog open time)

	// refreshing is true between pressing 'r' and the data arriving back
	refreshing bool

	// CPU sparkline / chart history: containerID → last 60 CPU% readings
	history map[string][]float64
	// Memory chart history: containerID → last 60 MEM MB readings
	memHistory map[string][]float64

	// Log streaming state
	logs      []string             // accumulated log lines for the current container
	logScroll int                  // scroll offset (0 = top, len(logs) = bottom)
	logCh     <-chan docker.LogLine // channel receiving live log lines
	logCancel context.CancelFunc   // call to stop the streaming goroutine

	// Event timeline state
	events             []docker.EventInfo    // container lifecycle events, newest first, capped at 100
	eventCh            <-chan docker.EventInfo // channel receiving live events
	eventCancel        context.CancelFunc    // call to stop the event streaming goroutine
	eventDisconnected  bool                  // true when the daemon dropped the event stream

	// ContainerStates tracks the last-known health of each container derived from
	// the event stream. Keyed by container name. Used to colorise topology nodes.
	ContainerStates map[string]docker.ContainerState
}

// Init implements tea.Model. Starts the spinner, first data fetch, and event streaming.
// Event streaming begins immediately so the timeline is populated from app launch —
// not deferred to the first Events tab visit.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchDataCmd(m.docker), tickCmd(), waitForEventCmd(m.eventCh))
}

// newModel creates the initial Model. Accepts any DockerClient (real or demo).
// Event streaming is started here so Init() can register the first waitForEventCmd.
func newModel(dc docker.DockerClient, version, host string, demo bool) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	eventCtx, eventCancel := context.WithCancel(context.Background())
	eventCh := dc.StreamEvents(eventCtx)

	return Model{
		version:         version,
		host:            host,
		demo:            demo,
		docker:          dc,
		activePanel:     PanelContainers,
		activeView:      ViewDashboard,
		keys:            DefaultKeyMap(),
		spinner:         sp,
		loading:         true,
		history:         make(map[string][]float64),
		memHistory:      make(map[string][]float64),
		eventCh:         eventCh,
		eventCancel:     eventCancel,
		ContainerStates: make(map[string]docker.ContainerState),
	}
}

// tickCmd returns a Cmd that fires a tickMsg after the refresh interval.
func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchDataCmd fetches all Docker data concurrently and returns a dataMsg.
// Containers, networks, and images are fetched in parallel goroutines.
// For running containers, CPU/memory stats are fetched in a second parallel pass.
func fetchDataCmd(dc docker.DockerClient) tea.Cmd {
	return func() tea.Msg {
		var (
			containers []docker.ContainerInfo
			networks   []docker.NetworkInfo
			images     []docker.ImageInfo
			cErr, nErr, iErr error
		)

		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); containers, cErr = dc.ListContainers() }()
		go func() { defer wg.Done(); networks, nErr = dc.ListNetworks() }()
		go func() { defer wg.Done(); images, iErr = dc.ListImages() }()
		wg.Wait()

		if cErr != nil {
			return dataMsg{err: cErr}
		}
		if nErr != nil {
			return dataMsg{err: nErr}
		}
		if iErr != nil {
			return dataMsg{err: iErr}
		}

		// Fetch CPU/memory stats for running containers in parallel.
		// Errors are silently ignored — the list remains visible with zero stats
		// rather than blocking the whole refresh.
		var statsMu sync.Mutex
		var statsWg sync.WaitGroup
		for i, c := range containers {
			if c.Status != "running" {
				continue
			}
			statsWg.Add(1)
			i, c := i, c
			go func() {
				defer statsWg.Done()
				cpu, mem, err := dc.FetchStats(c.ID)
				if err != nil {
					return
				}
				statsMu.Lock()
				containers[i].CPUPerc = cpu
				containers[i].MemMB = mem
				statsMu.Unlock()
			}()
		}
		statsWg.Wait()

		return dataMsg{
			containers: containers,
			networks:   networks,
			images:     images,
		}
	}
}
