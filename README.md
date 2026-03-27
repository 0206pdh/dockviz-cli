<div align="center">

# dockviz-cli

**A real-time Docker environment dashboard for your terminal.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-4DA6FF?style=flat-square)](LICENSE)
[![Built with Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-FF75B7?style=flat-square)](https://github.com/charmbracelet/bubbletea)

**[한국어 문서 (Korean)](README.ko.md)**

</div>

---

`dockviz-cli` is an interactive terminal UI (TUI) that gives you a live view of your Docker environment — containers, resource usage, networks, and images — without leaving the terminal.

Run `dockviz --demo` to try it right now without Docker.

## Features

- **Real-time stats** — CPU and memory usage refreshed every 2 seconds
- **Network topology** — ASCII graph of container-to-network relationships  
- **Image browser** — local images with tags and sizes
- **Container control** — start / stop / delete containers with a single key press
- **CPU sparkline** — 10-point unicode bar graph (▁▂▃▄▅▆▇█) per container row
- **Real-time log streaming** — `l` key opens a scrollable live log view
- **Image pull progress** — `dockviz pull <image>` shows per-layer download bars
- **Detail view** — per-container info (ID, image, ports, status)
- **Demo mode** — `--demo` flag runs with simulated data, no Docker required

## Installation

<img width="1297" height="61" alt="image" src="https://github.com/user-attachments/assets/653aa3ee-fdec-4a86-bb3d-e282601678b2" />
<img width="681" height="242" alt="image" src="https://github.com/user-attachments/assets/d08a5c2b-3019-47ee-a721-3c6e1e3c816f" />
<img width="673" height="282" alt="image" src="https://github.com/user-attachments/assets/bd08af97-00af-4ae2-8984-3b1f26540f5c" />
<img width="630" height="397" alt="image" src="https://github.com/user-attachments/assets/fa896233-26af-4747-829a-83d0905e3e1b" />


### Linux / VM — one-liner (no Go required)

```bash
# amd64 (most servers/VMs)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# arm64 (Raspberry Pi, AWS Graviton, etc.)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

### macOS — one-liner

```bash
# Intel Mac (amd64)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# Apple Silicon (M1/M2/M3)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

### Windows

Download from the [Releases page](https://github.com/0206pdh/dockviz-cli/releases/latest):
- `dockviz-windows-amd64.exe` — Intel/AMD
- `dockviz-windows-arm64.exe` — ARM (Surface Pro X, etc.)

### Build from source

```bash
git clone https://github.com/0206pdh/dockviz-cli.git
cd dockviz-cli
go build -o dockviz .
```

### go install

```bash
go install github.com/0206pdh/dockviz-cli@latest
```

## Quick Start

### Prerequisites

- Docker Engine running *(not required for `--demo`)*

### Run

```bash
# Connect to your local Docker daemon
dockviz

# Preview with simulated data — no Docker required
dockviz --demo

# Pull an image with live layer progress
dockviz pull nginx:alpine
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
| `d` | Delete selected container *(with confirmation)* |
| `l` | Open live log stream |
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
│   ├── root.go                    # Cobra CLI, --demo flag
│   └── pull.go                    # `dockviz pull <image>` subcommand
└── internal/
    ├── docker/
    │   ├── interface.go           # DockerClient interface
    │   ├── client.go              # live Docker SDK wrapper
    │   ├── demo.go                # animated demo data (no daemon needed)
    │   ├── containers.go          # list, stats, start/stop/restart/delete
    │   ├── networks.go            # network topology
    │   ├── images.go              # image list
    │   ├── pull.go                # image pull with per-layer progress stream
    │   └── logs.go                # container log streaming
    ├── tui/
    │   ├── model.go               # state (TEA Model)
    │   ├── update.go              # event handling (TEA Update)
    │   ├── view.go                # rendering (TEA View)
    │   ├── keymap.go              # keyboard bindings
    │   ├── pull.go                # pull progress TUI program
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
- [x] `dockviz pull <image>` — real-time per-layer download progress
- [x] Container delete with confirmation overlay (`d` key)
- [x] CPU sparkline — 10-point unicode bar graph per container row
- [x] Real-time log streaming with color coding (`l` key)
- [x] GitHub Actions release pipeline (Linux / Windows / macOS binaries on tag push)
- [ ] Remote Docker host support (`DOCKER_HOST`)
- [ ] Container stats history chart (full-screen)

## License

MIT © 2026 [0206pdh](https://github.com/0206pdh)
