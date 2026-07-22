# TODOS

Items deferred from engineering review (2026-03-30) and design doc.

---

## T-001: ContainerStats streaming (stream=true)

**What:** Switch from N+1 poll (FetchStats per container every 2s) to Docker daemon push via `ContainerStats(stream=true)`.

**Why:** Real-time stats with zero polling latency and reduced network overhead.

**Pros:** CPU/MEM updates immediately as they change, not on next 2s tick.

**Cons:** Goroutine complexity increases; requires Bubble Tea message integration for per-container stat updates.

**Context:** Parallel goroutine approach (N+1 solved with concurrency) was implemented in v0.2.0. Streaming would be a further improvement. Evaluate at v0.4.0.

**Status:** Deferred → v0.4.0

---

## T-002: Event filtering UI

**What:** Filter Events tab by action type (die, restart, start, etc.) or container name.

**Why:** With many containers the Events tab becomes noisy. Engineers debugging a crash only care about `die` and `restart` events.

**Pros:** Improves DevOps usability; on-call engineers can focus on failure-relevant events only.

**Cons:** Requires UX design (filter input overlay or toggle keys).

**Context:** The per-network timeline in v0.3.0 already provides implicit filtering. A global filter UI remains useful for the Events tab.

**Status:** Deferred → v0.4.0

---

## T-003: `--demo` crash scenario simulation

**What:** Animate a die → restart cycle in demo mode on a timer so the topology node colour change is visible in a recording without a live Docker environment.

**Why:** Portfolio demo viewers need to see real-time topology + event correlation in a GIF without Docker running.

**Context:** v0.3.0 now emits realistic ExitCode/OOMKilled on random die events. A scripted die → restart cycle with fixed timing would make it fully GIF-recordable.

**Status:** Partially addressed in v0.3.0 (realistic exit codes). Scripted cycle → v0.4.0

---

## T-004: Remote-capable Container Logs sizing

**What:** The Disk Usage panel's Container Logs category stats `ContainerInspect(id).LogPath` directly on the local filesystem. This only works when dockviz and `dockerd` share a filesystem (native Linux, or a WSL2 distro running `dockerd` directly) — it reads as a silent 0 on Docker Desktop (WSL2/Hyper-V backend) and on any remote `DOCKER_HOST`. See `docs/troubleshooting.md#13` and `docs/testing-container-log-disk-usage.md`.

**Why:** Docker Desktop is the default install path for most non-server users (Mac, Windows), so the category is currently invisible to a large share of dockviz's audience.

**Pros:** Streaming via `ContainerLogs` and counting bytes works over any transport, including remote `DOCKER_HOST` — no local filesystem access needed at all.

**Cons:** Streaming the full log to count its size is far more expensive than a `stat()` call, especially for containers with large log histories. Would need its own opt-in trigger (e.g. only on demand per-container, not as part of the panel's regular auto-refresh) rather than reusing the existing `fetchDiskUsageCmd` polling path.

**Context:** Landed in the same change as the Container Logs category itself. Deliberately scoped out of v1 — local-filesystem stat is the only zero-cost way to get this number, and native-Linux server operators (this project's primary stated audience) already work today.

**Status:** Deferred

---

## T-005: No automated test/lint workflow

**What:** `.github/workflows/` only contains `release.yml` (build + publish on push to main/tag). Nothing runs `go build`, `go vet`, or `go test ./...` on a PR before it can be merged.

**Why:** The volume-prune reclaim mismatch (`docs/troubleshooting.md#12`) shipped in the original Disk Usage panel PR and went unnoticed for a release cycle. `go test ./...` would not have caught it either — it's a live-daemon integration bug, and the `Client` methods that talk to the real Docker API sit at 0% coverage (`DemoClient` is what's actually under test) — but at minimum `go vet` and the existing unit tests running automatically on every PR would catch regressions in the parts that are covered, instead of relying on someone remembering to run them locally.

**Pros:** Catches build breaks and unit-test regressions before merge, not after `release.yml` has already tagged and published a version from a broken commit.

**Cons:** Needs a `services: docker` (or similar) GitHub Actions runner setup to exercise anything beyond the pure-function tests currently in the suite, if real-daemon integration coverage is ever added for `Client` (see T-004's cost note and the 0%-coverage gap noted in `docs/testing-container-log-disk-usage.md`).

**Context:** Every merged PR so far (#6–#11) has relied on the author running `go build`/`go test` locally before pushing. Fine at the current single-maintainer scale, but each auto-release (`release.yml`, merged in #10) ships whatever is on `main` immediately — there's no gate between "merged" and "published to PyPI/APT/GitHub Releases".

**Status:** Deferred
