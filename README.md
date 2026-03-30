<div align="center">

# dockviz-cli

**A real-time Docker environment dashboard for your terminal.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-4DA6FF?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/0206pdh/dockviz-cli?style=flat-square)](https://github.com/0206pdh/dockviz-cli/releases/latest)
[![Built with Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-FF75B7?style=flat-square)](https://github.com/charmbracelet/bubbletea)

**[한국어 문서 (Korean)](README.ko.md)**

</div>

---

## Why I built this

Running Docker day-to-day means repeating the same commands constantly:

```bash
docker ps
docker stats
docker logs -f nginx
docker network ls
docker images
```

Each command is simple, but when you're running several containers at once you end up juggling multiple terminal windows. A few specific pain points:

- With 5+ containers running, it's hard to tell at a glance which one is spiking CPU
- `docker logs -f` locks the terminal — you can't do anything else while tailing
- Figuring out which containers share a network requires multiple commands
- `docker pull` just dumps layer progress as raw text with no visual structure

`dockviz-cli` puts all of that information on **one screen, updating live**.

---

## Why use this over raw Docker commands

| Without dockviz | With dockviz |
|-----------------|--------------|
| `docker ps` + `docker stats` alternating | Container list + CPU/MEM live on one screen |
| Multiple terminal windows open | Tab key switches between Containers / Networks / Images / Events |
| `docker logs -f` in a separate window | `l` key opens a live log stream, `Esc` to dismiss |
| Manually type `docker rm -f` | `d` key with a confirmation overlay; multi-tag images are protected |
| `docker pull` raw text output | Per-layer progress bars |
| Guessing which container failed and why | Event timeline + topology node colours show failure propagation instantly |

A single static binary. No runtime dependencies. Drop it on any server and run.

---

## Screenshots

<img width="1297" height="61" alt="image" src="https://github.com/user-attachments/assets/653aa3ee-fdec-4a86-bb3d-e282601678b2" />
<img width="681" height="242" alt="image" src="https://github.com/user-attachments/assets/d08a5c2b-3019-47ee-a721-3c6e1e3c816f" />
<img width="673" height="282" alt="image" src="https://github.com/user-attachments/assets/bd08af97-00af-4ae2-8984-3b1f26540f5c" />
<img width="630" height="397" alt="image" src="https://github.com/user-attachments/assets/fa896233-26af-4747-829a-83d0905e3e1b" />

---

## Installation

### Linux / macOS — one-liner (auto-detects OS and architecture)

```bash
rm -f /usr/local/bin/dockviz; curl -sL "https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

Works on Linux (amd64 / arm64) and macOS (Intel / Apple Silicon) without modification.
Run the same command again to update to the latest version.

> **Note:** The `rm -f` at the start ensures the old binary is removed before writing the new one. Without it, curl overwrites the file in place and the running shell may cache the old inode — `--version` then still reports the previous version.

<details>
<summary>Manual download (pick your platform)</summary>

```bash
# Linux amd64 (most servers/VMs)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# Linux arm64 (Raspberry Pi, AWS Graviton, etc.)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# macOS Intel
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# macOS Apple Silicon (M1/M2/M3)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

</details>

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

---

## Quick Start

```bash
# Connect to your local Docker daemon
dockviz

# Preview with simulated data — no Docker required
dockviz --demo

# Connect to a remote Docker daemon
dockviz --host tcp://192.168.1.100:2375

# Or use the standard Docker environment variable
DOCKER_HOST=tcp://192.168.1.100:2375 dockviz

# Pull an image with live per-layer progress bars
dockviz pull nginx:alpine

# Print version
dockviz --version
```

---

## Features in detail

### 1. Real-time container dashboard

Refreshes every 2 seconds. Shows each container's CPU %, memory in MB, status, and ports in a single list.

### 2. CPU sparkline (▁▂▃▄▅▆▇█)

Each container row displays the last 10 CPU readings as unicode block characters. The trend — a spike, a gradual climb, idle flatline — is immediately readable without parsing numbers.

### 3. Stats history chart — `g` key

Press `g` on any running container to open a full-screen CPU and memory bar chart covering the last 60 readings (2 minutes of history).

**CPU % can exceed 100%.** Docker's CPU percentage formula is:

```
(container CPU time delta) / (system CPU time delta) × num_cores × 100
```

A container with no CPU limit running on a 4-core machine can reach 400%. The Y-axis auto-scales in 100% increments to match the actual data — so you always see bar height variation regardless of load level.

Threshold lines are drawn at **80%** (red) and **50%** (yellow) as absolute markers. These represent single-core saturation thresholds and stay at the same position regardless of how many cores the container is using.

```
  CPU   187.3%   0 – 200%
  200% ┤ ████████████████████████████████████
       ┤ ████████████████████████████████████
       ┤ ██████████████████████·············
  100% ┤ ████████████████████████████████████
       ┤ ████████████████████████████████████
  80%  ┤·················████████████████████  ← red threshold
       ┤                 ████████████████████
  50%  ┤·················████████████████████  ← yellow threshold
    0  └──────────────────────────────── now
       ← 2m ago
```

Empty cells in threshold rows render a coloured dot (`·`) so the boundary line is visible even when bars haven't reached it yet.

### 4. Failure propagation visualizer — Networks tab

The Networks tab uses a split layout:

- **Left — topology**: containers shown as colour-coded nodes connected by lines
  - `●` green — running
  - `◑` yellow — restarting
  - `✗` red — dead / exited with non-zero code
  - `○` grey — unknown / exited cleanly
- **Right — event timeline**: per-network log of lifecycle events with exit codes and OOM kill annotations

```
  app-network  : ● nginx-proxy ─── ● api-server ─── ✗ worker
  db-network   : ● api-server  ─── ● postgres-db ─── ● redis-cache
```

When a container dies, its node turns red instantly. You can see at a glance whether a failure is isolated or spreading.

### 5. Event timeline — Events tab

Live stream of Docker container lifecycle events: `create`, `start`, `die`, `restart`, `destroy`. Each `die` event includes the exit code and an OOM kill flag where applicable. The event stream reconnects automatically; press `r` if you need to force a reconnect.

### 6. Image browser

One row per tag, sorted alphabetically. Pressing `d` removes only the selected tag — not the whole image. If the image has multiple tags, a warning overlay lists them before you confirm. The underlying image stays until all its tags are removed.

### 7. Real-time log streaming

`l` opens a scrollable live log view for the selected container. Lines containing `ERROR` are red, `WARN` is yellow. Press `Esc` to dismiss.

### 8. Image pull progress — `dockviz pull`

```
  Pulling nginx:alpine

  abc1234abc12  ████████████░░░░░░░░  61%   4.2 MB / 6.9 MB   Downloading
  b2c3456b2c34  ████████████████████ 100%                      Pull complete ✓
  c3d4567c3d45  ────────────────────                           Already exists
```

Each layer gets its own progress bar. The overall pull completes only when all layers finish.

### 9. Demo mode

`dockviz --demo` runs entirely without a Docker daemon. CPU and memory values animate in sine-wave patterns so you can explore every tab and key binding before connecting to a real environment.

### 10. Exec into container — `e` key

Press `e` on any **running** container to open an interactive shell inside it. dockviz suspends, hands the terminal over to the shell session, and resumes when you exit.

```
  # In dockviz: select a running container, press e
  # Terminal becomes:
  / # ls /app
  / # ps aux
  / # exit
  # dockviz resumes
```

Tries `/bin/bash` first, falls back to `/bin/sh` automatically — works on Alpine, Debian, Ubuntu, and any image that has a POSIX shell. If `--host` is set, the exec is forwarded to the same remote daemon.

Not available in `--demo` mode (no real containers to exec into).

### 11. Volume mount display

The container detail view (`Enter`) shows all volume mounts:

```
  ID       a1b2c3d4e5f6
  Image    postgres:16
  Status   ● running
  Ports    5432
  Volumes  postgres_data → /var/lib/postgresql/data
           /backup → /backup (ro)
```

Named volumes show the volume name; bind mounts show the host path. Read-only mounts are marked `(ro)`.

### 12. Remote host support

```bash
dockviz --host tcp://192.168.1.100:2375
# or
DOCKER_HOST=tcp://192.168.1.100:2375 dockviz
```

`--host` takes precedence over `DOCKER_HOST`. Both work with the `pull` subcommand as well.

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `Tab` | Switch panel (Containers → Networks → Images → Events) |
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Open container detail |
| `Esc` | Go back / close overlay |
| `s` | Start / Stop selected container |
| `d` | Delete selected container or image tag *(with confirmation)* |
| `l` | Open live log stream |
| `r` | Force refresh / reconnect event stream |
| `g` | Open full-screen CPU/MEM history chart for selected container |
| `e` | Open interactive shell inside selected container *(running only)* |

---

## Architecture

### The Elm Architecture (TEA)

This project follows the TEA pattern as implemented by [Bubble Tea](https://github.com/charmbracelet/bubbletea):

```
main.go
  └── cmd.Execute()               ← Cobra CLI (--demo, --host flags)
        └── tui.Start()
              ├── docker.NewClient()      ← live Docker SDK wrapper
              │   or docker.NewDemoClient()  ← animated demo data
              └── tea.NewProgram(model)   ← Bubble Tea event loop
                    ├── Init()    → first data fetch + 2s ticker + event stream
                    ├── Update()  → key events, ticks, Docker responses, state transitions
                    └── View()    → Lip Gloss styled terminal output
```

**Why TEA?**
- State (Model), logic (Update), and rendering (View) are completely separated — easy to reason about and test
- All state changes flow in one direction, so bugs are easy to trace
- Async work (Docker API calls, log streaming, event streaming) is isolated in `Cmd` — the UI never blocks

### Package layout

```
dockviz-cli/
├── main.go                        # entry point, injects build-time version via ldflags
├── cmd/
│   ├── root.go                    # Cobra root command — --demo, --host flags
│   └── pull.go                    # `dockviz pull <image>` subcommand
└── internal/
    ├── docker/
    │   ├── interface.go           # DockerClient interface (live + demo share this)
    │   ├── client.go              # live Docker SDK wrapper (FromEnv + optional WithHost)
    │   ├── demo.go                # animated sine-wave demo data, no daemon needed
    │   ├── containers.go          # list, stats (parallel fetch), start/stop/remove
    │   ├── networks.go            # topology: NetworkList then NetworkInspect per network
    │   ├── images.go              # image list — one row per tag, alphabetically sorted
    │   ├── state.go               # ContainerState — health derived from event stream
    │   ├── events.go              # lifecycle event streaming (ExitCode, OOMKilled)
    │   ├── pull.go                # image pull with per-layer progress stream
    │   └── logs.go                # container log streaming (stdcopy demux)
    ├── tui/
    │   ├── model.go               # TEA Model — all UI state including history/memHistory maps
    │   ├── update.go              # TEA Update — key handling, tick, Docker msg routing
    │   ├── view.go                # TEA View — renders all panels, chart, overlays
    │   ├── keymap.go              # key bindings (bubbles/key)
    │   ├── pull.go                # standalone pull-progress TUI program
    │   └── start.go               # wires docker client into tea.NewProgram
    └── ui/
        ├── styles.go              # Lip Gloss colour palette, shared styles, sparkline
        └── graph.go               # topology graph renderer with health-coloured nodes
```

### DockerClient interface

The live client and the demo client both implement the same interface. The TUI layer never knows which one it's talking to.

```go
type DockerClient interface {
    ListContainers() ([]ContainerInfo, error)
    ListNetworks()   ([]NetworkInfo, error)
    ListImages()     ([]ImageInfo, error)
    FetchStats(id string) (cpu float64, memMB float64, err error)
    StartContainer(id string)   error
    StopContainer(id string)    error
    RestartContainer(id string) error
    RemoveContainer(id string)  error
    RemoveImage(id string)      error
    StreamLogs(ctx context.Context, id string) <-chan LogLine
    StreamEvents(ctx context.Context)          <-chan EventInfo
    Close()
}
```

### Exec shell — how it works

`e` does not go through the `DockerClient` interface. It spawns a system `docker exec -it <name> sh` process directly via `tea.ExecProcess`, which suspends the Bubble Tea event loop and hands the terminal to the subprocess. When the shell exits, the loop resumes and a fresh data fetch runs.

```go
cmd := exec.Command("docker", "exec", "-it", containerName, "sh", "-c", "bash 2>/dev/null || sh")
return tea.ExecProcess(cmd, func(err error) tea.Msg {
    return execDoneMsg{err: err}
})
```

### Version injection

The binary version is injected at build time via ldflags:

```bash
go build -ldflags="-X main.version=v1.2.3" -o dockviz .
```

`dockviz --version` always reflects the exact tag that was built. A local `go build` without ldflags reports `dev`.

---

## CI/CD

Pushing a version tag triggers GitHub Actions to cross-compile and publish binaries for all six targets automatically:

```bash
git tag v1.2.3 && git push origin v1.2.3
```

Actions:
1. Cross-compiles for Linux amd64/arm64, macOS amd64/arm64, Windows amd64/arm64
2. Injects the tag as the version string via `-ldflags="-X main.version=${{ github.ref_name }}"`
3. Uploads all binaries to GitHub Releases

The `curl` one-liner on the install section always resolves to the latest release via GitHub's `/releases/latest/download/` redirect.

---

## Tech Stack

| Layer | Library | Purpose |
|-------|---------|---------|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Event loop, TEA pattern |
| TUI styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | Colours, borders, layout |
| TUI components | [Bubbles](https://github.com/charmbracelet/bubbles) | Spinner, key bindings |
| Docker API | [Docker SDK for Go](https://pkg.go.dev/github.com/docker/docker/client) | Container / network / image / event data |
| CLI | [Cobra](https://github.com/spf13/cobra) | Flags and subcommands |

---

## Roadmap

- [x] Project scaffold and build pipeline
- [x] Docker client wrapper with interface (live + demo mode)
- [x] Container list panel with live CPU/MEM stats (parallel fetch)
- [x] CPU sparkline — 10-point unicode bar per container
- [x] Network topology graph with health-coloured nodes (● ◑ ✗ ○)
- [x] Networks tab split layout — topology left, per-network event timeline right
- [x] Container lifecycle event streaming with exit code and OOM kill detection
- [x] ContainerState tracking — failure propagation across network topology
- [x] Image browser — one row per tag, alphabetical order, tag-by-tag safe delete
- [x] Container detail view
- [x] `--demo` mode (no Docker required)
- [x] `dockviz pull <image>` — real-time per-layer download progress
- [x] Container / image delete with confirmation overlay (`d` key)
- [x] Real-time log streaming with colour coding (`l` key)
- [x] Event stream disconnect detection + `r` to reconnect
- [x] GitHub Actions release pipeline (Linux / Windows / macOS binaries on tag push)
- [x] Remote Docker host support (`--host` flag + `DOCKER_HOST` env var)
- [x] Container stats history chart — full-screen CPU/MEM bar chart (`g` key)
- [x] Dynamic CPU Y-axis — auto-scales beyond 100% for multi-core containers
- [x] Volume mount display in container detail view
- [x] Interactive exec shell — `e` key suspends TUI and opens shell in container

---

## License

MIT © 2026 [0206pdh](https://github.com/0206pdh)
