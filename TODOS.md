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
