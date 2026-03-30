# Container States, Network Drivers & Event Actions

A reference for every status, driver, and event action shown in dockviz.

---

## Container States

| State | Icon | Colour | Meaning |
|-------|------|--------|---------|
| `running` | ● | green | The container process is alive and executing. |
| `exited` | ○ | red | The container process has stopped. The filesystem is preserved until the container is removed. |
| `paused` | ◑ | yellow | All processes in the container are frozen with `SIGSTOP`. CPU usage drops to zero; memory is still held. Resume with `docker unpause`. |
| `restarting` | ↻ | blue | The container crashed or was stopped and the restart policy is bringing it back up. Seen between a `die` event and the next `start`. |
| `dead` | ✗ | red | A force-remove was attempted but failed (e.g. device busy). The container is partially destroyed. Usually requires manual cleanup. |
| `created` | ○ | gray | `docker create` has run but `docker start` has not. The container exists but no process has ever run. |
| `removing` | ○ | gray | `docker rm` is in progress. Transient; disappears from the list once removal is complete. |

### Exit Codes (shown on `die` events)

| Exit code | Meaning |
|-----------|---------|
| `0` | Clean exit — the process finished normally. |
| `1` | Generic application error — the process itself returned a non-zero status. |
| `130` | `SIGINT` — the container received Ctrl+C. |
| `137` | `SIGKILL` — killed by the kernel (OOM) or by `docker kill`. Check `OOM` flag. |
| `143` | `SIGTERM` — graceful shutdown signal sent by `docker stop`. |
| Other | Application-defined exit code. Check the container logs for context. |

---

## Network Drivers

| Driver | Description |
|--------|-------------|
| `bridge` | Default driver. Creates an isolated virtual network on the host. Containers on the same bridge can reach each other by IP; external traffic is NATted. The built-in `bridge` network is the fallback when no `--network` flag is given. |
| `host` | Removes network isolation entirely — the container shares the host's network stack. No virtual interface, no NAT. Useful for performance-critical workloads but exposes all host ports. |
| `none` | Disables all networking. The container gets only a loopback (`lo`) interface. Use when network access must be explicitly blocked. |
| `overlay` | Spans multiple Docker hosts (Swarm mode). Containers on different machines appear on the same virtual network. Not visible in single-host setups. |
| `macvlan` | Assigns a MAC address from the physical network to the container so it appears as a real device on the LAN. Used when containers must be directly addressable on the corporate network. |
| `ipvlan` | Like macvlan but containers share the host MAC address. L2 or L3 mode. Useful where the upstream switch limits the number of MACs per port. |

### System Networks (always shown at the bottom)

Docker creates three networks automatically and they cannot be removed:

- **bridge** — default bridge for unspecified containers
- **host** — direct host networking
- **none** — no networking

---

## Event Actions

Events appear in the Events tab and in the Networks tab timeline.

| Action | Icon | Colour | Meaning |
|--------|------|--------|---------|
| `create` | ✦ | gray | Container object created (`docker create` or first step of `docker run`). No process running yet. |
| `start` | ● | green | Container process started. |
| `restart` | ↻ | blue | Container was restarted (manually via `docker restart` or by a restart policy). |
| `stop` | ○ | red | `docker stop` sent `SIGTERM`; container exited cleanly within the grace period. |
| `kill` | ○ | red | `docker kill` sent a signal (default `SIGKILL`). Immediate termination; no cleanup. |
| `die` | ○ | red | The container process exited for any reason (clean exit, crash, or kill). Always paired with an exit code. `OOM` annotation means the kernel killed it for exceeding memory limits. |
| `pause` | ◑ | yellow | All processes frozen with `SIGSTOP`. Container is still "running" from Docker's perspective. |
| `unpause` | ● | green | Processes resumed after a pause. |
| `destroy` | ✕ | red | Container object permanently removed (`docker rm`). The entry is deleted from dockviz. |
| `rename` | • | gray | Container was renamed. |
| `exec_create` / `exec_start` | • | gray | A command was executed inside the container (`docker exec`). |
| `health_status` | • | gray | Health check result (healthy / unhealthy / starting). Only emitted when a `HEALTHCHECK` is defined. |

---

## Image States

Docker images don't have a runtime state, but their tag status matters.

| State | Meaning |
|-------|---------|
| `name:tag` | A normal tagged image (e.g. `nginx:1.25-alpine`). |
| `<none>` | **Dangling image** — an untagged layer, usually left behind after `docker pull` or `docker build` rebuilt an existing tag. Takes up disk space but is not actively used. Safe to remove with `docker image prune`. |

### Multiple Tags on One Image

A single image ID can carry multiple tags (e.g. `nginx:latest` and `nginx:1.25`). In dockviz each tag is shown as a separate row. Deleting one tag only removes that alias — the image data and other tags remain until the last tag is deleted.

---

## OOM Killed

When a container's memory usage hits the limit set by `--memory`, the Linux kernel's Out-Of-Memory (OOM) killer terminates it. This appears as:

```
○ die   api-server   exit=137  OOM
```

- Exit code is always `137` (SIGKILL from the kernel).
- The `OOM` badge is shown when the Docker `oomKilled` attribute is `true`.
- **Fix:** increase `--memory` limit, reduce application memory usage, or add a swap limit.
