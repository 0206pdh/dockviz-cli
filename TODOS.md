# TODOS

Items deferred from engineering review (2026-03-30) and design doc.

---

## T-001: ContainerStats streaming (stream=true)

**What:** Switch from N+1 poll (FetchStats per container every 2s) to Docker daemon push via `ContainerStats(stream=true)`.

**Why:** Real-time stats with zero polling latency and reduced network overhead.

**Pros:** CPU/MEM updates immediately as they change, not on next 2s tick.

**Cons:** Goroutine complexity increases; requires Bubble Tea message integration for per-container stat updates.

**Context:** Currently the plan is to call FetchStats in parallel goroutines (N+1 solved with concurrency). Streaming would be a further improvement where Docker pushes rather than we poll. Evaluate at v0.3.0 — the parallel goroutine approach may be sufficient for the portfolio use case.

**Depends on:** FetchStats parallel goroutine implementation (v0.2.0 bug fix) completed first.

---

## T-002: Event filtering UI

**What:** Filter Events tab by action type (die, restart, start, etc.) or container name.

**Why:** With many containers the Events tab becomes noisy. Engineers debugging a crash only care about `die` and `restart` events.

**Pros:** Improves DevOps usability; on-call engineers can focus on failure-relevant events only.

**Cons:** Requires UX design (filter input overlay or toggle keys). Outside v0.2.0 scope.

**Context:** Explicitly deferred to v0.3.0 in design doc (`docs/design-v0.2.0-failure-propagation.md`).

**Depends on:** v0.2.0 Events streaming at Init complete.

---

## T-003: `--demo` crash scenario simulation

**What:** Animate a container crash (die → restart cycle) in demo mode with realistic EventInfo and ContainerState updates.

**Why:** Portfolio demo viewers need to see the real-time topology + event correlation in a single screenshot or GIF without needing a live Docker environment.

**Pros:** Makes the crash-visualization feature demonstrable without a live Docker environment.

**Cons:** demo.go grows more complex; needs to emit die/restart events on a timer.

**Context:** Deferred to v0.3.0 in design doc. Only meaningful after v0.2.0 ContainerState + topology split view is implemented — without those, the demo would show events but no topology node color change.

**Depends on:** v0.2.0 ContainerState + graph split view complete.
