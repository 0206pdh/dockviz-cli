# Testing: Container Logs Disk Usage Category

**Status:** Written 2026-07-22 from static analysis of `internal/docker/diskusage.go` and the vendored `docker/docker` v28.0.4 source. Not yet executed against a real Docker daemon — this session had no native-Linux `dockerd` available (Windows host, empty local Docker Desktop). Run this checklist on a real Linux box before the next release that touches `diskusage.go`.

## Why this needs a manual test plan, not just `go test`

`go test ./...` covers `logFileSizes` (pure function, table-tested against real temp files) but the `Client` methods that actually talk to Docker — `DiskUsage()`, `PruneLogs()` — sit at 0% coverage, same as every other real-daemon method in this package (see `TODOS.md` T-005). That's not a gap this feature can close with more unit tests: the two things that actually determine whether it works are (1) whether dockviz's process shares a filesystem with `dockerd`, and (2) file permissions on `/var/lib/docker/containers`. Neither is expressible as a Go unit test — they're properties of the *environment*, not the code. The only way to validate them is to run the real binary against a real daemon in each environment shape and measure.

## Supported-environment matrix

| Environment | Shares filesystem with `dockerd`? | Expected result |
|---|---|---|
| Native Linux, rootful `dockerd`, dockviz run as unprivileged `docker`-group user | Yes | `Unavailable = "permission denied on N container(s) — try sudo"` |
| Native Linux, rootful `dockerd`, dockviz run via `sudo` | Yes | Full numbers, prune works |
| Native Linux, rootless `dockerd` | Yes (logs are user-owned) | Full numbers, prune works, no `sudo` needed |
| `dockerd` installed directly inside a WSL2 distro (not Docker Desktop integration) | Yes | Same as native Linux rows above |
| Docker Desktop, Windows (WSL2 backend) | No — `dockerd` runs in a separate hidden `docker-desktop` distro | Logs row reads 0, no warning (see `docs/troubleshooting.md#13`) |
| Docker Desktop, macOS (VM backend) | No | Same as above |
| Remote `DOCKER_HOST=tcp://...` | No | Same as above |

This matrix itself is the thing to verify first — confirm each row's "expected result" actually happens before doing the detailed measurement below. This project's stated primary audience is native-Linux server operators (`README.md` → "Why I built this"), so the first three rows matter most.

## Quantitative measurement procedure

Run this on a native-Linux Docker host (rootful, default install — the row most dockviz server users will actually hit).

**1. Generate a container with a deterministic log size**

```bash
docker run -d --name logtest alpine sh -c '
  i=0
  while [ $i -lt 5000 ]; do
    echo "line $i $(head -c 100 </dev/zero | tr "\0" x)"
    i=$((i+1))
  done
  sleep 3600
'
```

**2. Record ground truth directly from the filesystem**

```bash
LOGPATH=$(docker inspect --format='{{.LogPath}}' logtest)
stat -c%s "$LOGPATH"          # ground-truth bytes for the active file
```

**3. Compare against the panel**

Open `dockviz` (or `sudo dockviz`, matching the row under test), switch to the Disk Usage tab, read the Container Logs row's SIZE column.

*Pass:* `ground_truth_bytes / 1024 / 1024` matches the displayed value within one `FormatSize` rounding unit. It should be exact — `SizeMB`/`ReclaimMB` are computed straight from `fi.Size()`, no estimation involved.

**4. Prune and re-verify**

Select the Container Logs row, press `d`, confirm. Re-run `stat -c%s "$LOGPATH"` — expect `0`.

**5. Confirm the container is undisturbed**

```bash
docker logs -f logtest &      # start tailing before/after the prune
# after pruning: write a new line
docker exec logtest sh -c 'echo post-truncate-line'
```

*Pass:* the container is still `running` (`docker ps`), and `post-truncate-line` appears on the existing `docker logs -f` tail — the daemon's open file descriptor survived the truncate (see `docs/troubleshooting.md#12` for the same fd principle applied to volumes, and the `PruneLogs` doc comment in `diskusage.go` for why truncate-in-place rather than remove).

**6. Rotated-file handling**

```bash
docker run -d --name logtest-rotate --log-opt max-size=1k --log-opt max-file=3 \
  alpine sh -c 'i=0; while [ $i -lt 5000 ]; do echo "line $i padding padding padding"; i=$((i+1)); done; sleep 3600'
ls "$(docker inspect --format='{{.LogPath}}' logtest-rotate)"*
```

*Pass:* `.log.1` / `.log.2` siblings exist, dockviz's SizeMB for this container includes them (cross-check against `du -cb "$LOGPATH"*`), and after pruning the rotated files are gone entirely (`ls` no longer lists them) while the base `.log` file still exists at 0 bytes — rotated files are `os.Remove`d, the active file is only `os.Truncate`d.

**7. Permission-denied path**

Repeat steps 1–3 as a `docker`-group (non-root, no `sudo`) user on a box where `/var/lib/docker` is the default root-owned layout.

*Pass:* Total/SizeMB read `0`, and the panel shows `⚠ permission denied on N container(s) — try sudo` under the row — not a silent, indistinguishable-from-empty `0`. Re-run the identical steps with `sudo dockviz` and confirm the row now populates.

**8. Unsupported-environment smoke test**

On Docker Desktop or against a remote `DOCKER_HOST`, confirm the Logs row simply reads `0` with no warning and no crash — `permDenied` is deliberately `false` in this path (see the `logFileSizes` doc comment), since there's nothing actionable to tell the user. This step is really about *not panicking* more than about numbers.

## Pass/fail criteria

| Check | Pass condition |
|---|---|
| SizeMB accuracy | `\|dockviz SizeMB − ground truth MB\|` within one rounding unit |
| ReclaimMB invariant | `ReclaimMB == SizeMB` before pruning (every byte found is reclaimable by design) |
| Prune correctness | ground-truth file size is `0` after pruning |
| Container survives | stays `running`; `docker logs -f` keeps receiving lines written after the truncate |
| Rotated files | counted in SizeMB; fully removed (not left as empty files) after pruning |
| Permission-denied UX | `Unavailable` message shown, not a silent `0`, when the daemon-owned log path isn't readable |
| Unsupported-env safety | no panic, no error dialog — reads as a clean `0` |

## Known gap

This whole procedure is manual. There is no Docker daemon in this repo's GitHub Actions runners today (`TODOS.md` T-005), so none of it can run in CI as written. Until that changes, treat this file as a release checklist for any change to `internal/docker/diskusage.go`, not as a substitute for `go test`.
