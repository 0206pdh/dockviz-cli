<div align="center">

# dockviz-cli

**A real-time Docker environment dashboard for your terminal.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-4DA6FF?style=flat-square)](LICENSE)
[![Built with Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-FF75B7?style=flat-square)](https://github.com/charmbracelet/bubbletea)

</div>

---

`dockviz-cli` is an interactive terminal UI (TUI) that gives you a live view of your Docker environment — containers, resource usage, networks, and images — without leaving the terminal.

Run `dockviz --demo` to try it right now without Docker.

## Features

- **Real-time stats** — CPU and memory usage refreshed every 2 seconds
- **Network topology** — ASCII graph of container-to-network relationships  
- **Image browser** — local images with tags and sizes
- **Container control** — start / stop containers with a single key press
- **Detail view** — per-container info (ID, image, ports, status)
- **Demo mode** — `--demo` flag runs with simulated data, no Docker required

## Quick Start

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- Docker Desktop or Docker Engine *(not required for `--demo`)*

### Install from source

```bash
git clone https://github.com/0206pdh/dockviz-cli.git
cd dockviz-cli
go build -o dockviz .
```

### Run

```bash
# Connect to your local Docker daemon
./dockviz

# Preview with simulated data — no Docker required
./dockviz --demo
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `Tab` | Switch panel (Containers → Networks → Images) |
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Open container detail |
| `Esc` | Go back |
| `s` | Start / Stop selected container |
| `l` | View container logs *(coming soon)* |
| `r` | Force refresh |

## Architecture

This project follows **The Elm Architecture (TEA)** as implemented by [Bubble Tea](https://github.com/charmbracelet/bubbletea):

```
main.go
  └── cmd.Execute()          ← Cobra CLI (--demo flag)
        └── tui.Start()
              ├── docker.NewClient()      ← Docker SDK / DemoClient
              └── tea.NewProgram(model)   ← Bubble Tea event loop
                    ├── Init()   → first data fetch + ticker
                    ├── Update() → key events, ticks, Docker responses
                    └── View()   → Lip Gloss styled terminal output
```

### Package layout

```
dockviz-cli/
├── main.go                        # entry point
├── cmd/
│   └── root.go                    # Cobra CLI, --demo flag
└── internal/
    ├── docker/
    │   ├── interface.go           # DockerClient interface
    │   ├── client.go              # live Docker SDK wrapper
    │   ├── demo.go                # animated demo data (no daemon needed)
    │   ├── containers.go          # list, stats, start/stop/restart
    │   ├── networks.go            # network topology
    │   └── images.go              # image list
    ├── tui/
    │   ├── model.go               # state (TEA Model)
    │   ├── update.go              # event handling (TEA Update)
    │   ├── view.go                # rendering (TEA View)
    │   ├── keymap.go              # keyboard bindings
    │   └── start.go               # wires docker client → TUI
    └── ui/
        ├── styles.go              # Lip Gloss color palette and styles
        └── graph.go               # ASCII network graph renderer
```

## Tech Stack

| Layer | Library | Purpose |
|-------|---------|----------|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Event loop, TEA pattern |
| TUI styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | Colors, borders, layout |
| TUI components | [Bubbles](https://github.com/charmbracelet/bubbles) | Spinner, key bindings |
| Docker API | [Docker SDK for Go](https://pkg.go.dev/github.com/docker/docker/client) | Container / network / image data |
| CLI | [Cobra](https://github.com/spf13/cobra) | Flags and subcommands |

## Roadmap

- [x] Project scaffold and build pipeline
- [x] Docker client wrapper with interface (live + demo mode)
- [x] Container list panel with live stats
- [x] Network topology ASCII graph
- [x] Image browser panel
- [x] Container detail view
- [x] `--demo` mode (no Docker required)
- [ ] Real-time log streaming (`l` key)
- [ ] Container stats history sparkline
- [ ] Remote Docker host support (`DOCKER_HOST`)
- [ ] GitHub Actions release pipeline (cross-platform binaries)

## License

MIT © 2026 [0206pdh](https://github.com/0206pdh)
