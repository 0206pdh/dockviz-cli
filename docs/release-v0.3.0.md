# v0.3.0 Release — Failure Propagation Visualizer

## Overview

v0.3.0 ships a real-time failure propagation visualizer. The Networks tab is rebuilt as a split-pane layout: a coloured topology graph on the left shows container health at a glance, and a per-network event timeline on the right shows exactly what happened and when.

---

## Implementation Walkthrough

### Phase 1 — Engineering Review (`/plan-eng-review`)

Before writing code, a full codebase audit was run. Key findings acted on:

| Finding | Fix |
|---------|-----|
| CPU/MEM stats always 0 | `fetchDataCmd` now runs `FetchStats` in a second parallel goroutine pass |
| Silent remove errors | `removeContainerCmd` / `removeImageCmd` return errors via `dataMsg{err}` |
| Event stream starts late | `StreamEvents` moved to `newModel()` + `Init()` batch |
| Log corruption on TTY containers | Replaced manual 8-byte strip with `stdcopy.StdCopy` via `io.Pipe` |
| Sparkline scale misleading | Fixed 0–100 % scale; was relative-to-local-max |
| `os.Exit` inside `RunE` | Replaced with `return fmt.Errorf(...)` |

All fixes landed in **PR #3** (merged before v0.3.0).

### Phase 2 — v0.3.0 Feature Implementation

Six files changed, in dependency order:

#### `internal/docker/state.go` (new file)

Defines `ContainerState` — the canonical representation of a container's last-known health, derived from the event stream rather than polled from the Docker API.

```go
type ContainerState struct {
    Status       string    // "running" | "dead" | "restarting"
    ExitCode     int       // 0=clean, 137=SIGKILL, 1=app error
    OOMKilled    bool
    RestartCount int       // session-scoped; resets on app restart
    UpdatedAt    time.Time
}
```

**Why here:** both `internal/ui` and `internal/tui` already import `internal/docker`. Placing the type here prevents the import cycle that would occur if `ui/graph.go` referenced a `tui` type.

#### `internal/docker/events.go`

`EventInfo` gets two new fields:

```go
ExitCode  int   // populated on container/die
OOMKilled bool  // populated on container/die when oomKilled="true"
```

Parsed from `msg.Actor.Attributes` in the streaming goroutine.

#### `internal/tui/model.go`

Added to `Model`:

```go
ContainerStates map[string]docker.ContainerState
```

Initialised to an empty map in `newModel()` so the topology renderer never sees a nil map.

#### `internal/tui/update.go`

The `eventMsg` handler now drives state transitions after appending the event:

| Event action | State transition |
|-------------|-----------------|
| `start` | `Status="running"`, RestartCount reset to 0 |
| `restart` | `Status="restarting"`, RestartCount incremented |
| `die` | `Status="dead"`, ExitCode/OOMKilled populated |
| `destroy` | Entry deleted from map |

#### `internal/ui/graph.go`

`RenderNetworkGraph` signature updated:

```go
func RenderNetworkGraph(networks []docker.NetworkInfo, states map[string]docker.ContainerState) string
```

New `containerLabel()` helper applies a colour and icon per health state:

```
● green    running
◑ yellow   restarting
✗ red      dead
○ gray     unknown / no event data yet
```

#### `internal/tui/view.go`

`renderNetworks()` rebuilt as a split layout using lipgloss side-by-side panels:

- **Left** — `ui.RenderNetworkGraph` with `m.ContainerStates`
- **Right** — recent events filtered to the selected network's containers, with `exit=N` and `OOM` annotations on `die` events

Width guard: `m.width < 80` falls back to `renderNetworksFallback()` (plain list) to avoid corruption on narrow SSH terminals or before the first `WindowSizeMsg`.

#### `internal/docker/demo.go`

Die events in `StreamEvents` now emit realistic exit codes (0, 1, 137) and `OOMKilled=true` on a random subset of SIGKILL exits, making the demo timeline meaningful.

---

## Troubleshooting Log

### CRLF line endings blocking Edit tool

**Problem:** The repository was cloned on Windows with `autocrlf=true`. Files modified in a previous session had CRLF (`\r\n`) line endings. The `Edit` tool matches byte-for-byte, so attempts to replace strings using LF-only patterns silently failed with "String to replace not found."

**Fix:** Used a small Go script that reads the file as raw bytes, matches the CRLF pattern explicitly, and writes the replacement back:

```go
old := "\t\t\t\tselect {\r\n\t\t\t\tcase ch <- EventInfo{..."
content = strings.Replace(content, old, new, 1)
os.WriteFile("internal/docker/demo.go", []byte(content), 0644)
```

**Prevention:** When targeting files with CRLF endings, include `\r\n` in match strings, or convert the file to LF first with `git config core.autocrlf false`.

### `go.mod` version directive rollback

**Problem:** An attempt to downgrade `go 1.25.0` to `go 1.23` was reverted by `go mod tidy` because the local toolchain is Go 1.26.1, which supports the `go 1.25.0` directive.

**Fix:** Left the directive as-is. The `go 1.25.0` line is valid and correct for the toolchain in use.

### Import cycle risk with `ContainerState`

**Problem:** `internal/ui/graph.go` needed to accept `ContainerState` to colour topology nodes. Defining the struct in `internal/tui` would have created a cycle since `internal/tui` already imports `internal/ui`.

**Fix:** Defined `ContainerState` in `internal/docker` — the only package both `tui` and `ui` already import. Zero new import edges required.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/docker/state.go` | New — `ContainerState` struct |
| `internal/docker/events.go` | `EventInfo` + `ExitCode`, `OOMKilled` fields |
| `internal/docker/demo.go` | Realistic exit codes on die events |
| `internal/tui/model.go` | `ContainerStates` map field |
| `internal/tui/update.go` | State transitions in `eventMsg` handler |
| `internal/ui/graph.go` | `containerLabel()` + updated `RenderNetworkGraph` signature |
| `internal/tui/view.go` | Networks tab split layout |

## Release

- **Tag:** `v0.3.0`
- **PR:** [#4](https://github.com/0206pdh/dockviz-cli/pull/4)
- **GitHub Release:** https://github.com/0206pdh/dockviz-cli/releases/tag/v0.3.0
