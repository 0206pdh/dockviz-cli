// leftovers.go classifies Docker resources that tend to be left behind and
// keep using disk without anyone noticing — the Docker equivalent of an S3
// incomplete multipart upload. See docs/leftovers-plan.ko.md §2 for the rules
// and docs/validation.ko.md §10 for the real-daemon evidence behind them.
//
// Detection is read-only. DetectLeftovers is a pure function over plain
// inputs; the *FromSDK helpers convert Docker API responses into those inputs
// and are shared by the live client and the fixture tests.
package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
)

// LeftoverKind names one detection rule.
type LeftoverKind string

const (
	LeftoverComposeOrphan    LeftoverKind = "compose-orphan"
	LeftoverComposeOneOff    LeftoverKind = "compose-oneoff"
	LeftoverDroppedVolume    LeftoverKind = "compose-dropped-volume"
	LeftoverOwnerlessVolume  LeftoverKind = "compose-ownerless-volume"
	LeftoverAnonymousVolume  LeftoverKind = "anonymous-volume"
	LeftoverStoppedContainer LeftoverKind = "stopped-container"
	LeftoverBuilderCache     LeftoverKind = "buildx-builder-cache"
)

// LeftoverConfidence says how much judgement a finding still needs.
type LeftoverConfidence string

const (
	// ConfidenceCertain: the labels prove the resource is no longer wanted
	// by its owner (e.g. a service removed from the compose file).
	ConfidenceCertain LeftoverConfidence = "certain"
	// ConfidenceLikely: probably unwanted, but a normal workflow can produce
	// the same state (e.g. `compose down` that intentionally keeps data).
	ConfidenceLikely LeftoverConfidence = "likely"
	// ConfidenceReport: shown for awareness only; never removed by dockviz.
	ConfidenceReport LeftoverConfidence = "report"
)

// DefaultStoppedFor is how long a container must have been stopped before
// it is reported as a stopped-container leftover.
const DefaultStoppedFor = 14 * 24 * time.Hour

const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
	labelComposeOneOff  = "com.docker.compose.oneoff"
	labelComposeVolume  = "com.docker.compose.volume"
	labelAnonymousVol   = "com.docker.volume.anonymous"
)

// Leftover is one finding.
type Leftover struct {
	Kind       LeftoverKind
	Confidence LeftoverConfidence
	// ID is the container ID or the volume name — what a removal call takes.
	ID      string
	Name    string
	Project string
	Service string
	// SizeMB is the measured size (container RW layer or volume data).
	// SizeKnown is false when the daemon returned no size.
	SizeMB    float64
	SizeKnown bool
	Reason    string
	// Action is the suggested next step in plain words.
	Action string
	// Removable is true only when removing the resource is safe to offer
	// now: nothing is using it and the rule allows removal at all.
	Removable bool
	// HeldBy lists containers that still use the resource. They must go
	// first before its space can be reclaimed.
	HeldBy []string
}

// ComposeScope is the loaded compose project, if any. Rules that compare
// against the compose file (orphans, dropped volumes) only run with it.
type ComposeScope struct {
	Project  string
	Services map[string]bool
	Volumes  map[string]bool
}

// LeftoverContainer is the subset of container data the rules need.
type LeftoverContainer struct {
	ID     string
	Name   string
	State  string
	Labels map[string]string
	// FinishedAt is zero when unknown or the container never stopped.
	FinishedAt time.Time
	SizeRwMB   float64
	SizeKnown  bool
	// Volumes are the names of volumes mounted by the container.
	Volumes []string
}

// LeftoverVolume is the subset of volume data the rules need.
type LeftoverVolume struct {
	Name      string
	Labels    map[string]string
	SizeMB    float64
	SizeKnown bool
	// RefCount is the daemon's count of containers using the volume, or -1
	// when the daemon did not report usage data.
	RefCount int64
}

// LeftoverInput is everything DetectLeftovers looks at.
type LeftoverInput struct {
	Containers []LeftoverContainer
	Volumes    []LeftoverVolume
	Compose    *ComposeScope
	Now        time.Time
	// StoppedFor overrides DefaultStoppedFor when positive.
	StoppedFor time.Duration
}

// DetectLeftovers applies every rule and returns findings ordered by
// confidence (certain, likely, report), then size, then name.
func DetectLeftovers(in LeftoverInput) []Leftover {
	stoppedFor := in.StoppedFor
	if stoppedFor <= 0 {
		stoppedFor = DefaultStoppedFor
	}

	mountedBy := map[string][]string{}
	projectHasContainer := map[string]bool{}
	for _, c := range in.Containers {
		for _, v := range c.Volumes {
			mountedBy[v] = append(mountedBy[v], c.Name)
		}
		if p := c.Labels[labelComposeProject]; p != "" {
			projectHasContainer[p] = true
		}
	}

	var out []Leftover
	for _, c := range in.Containers {
		if l, ok := classifyContainer(c, in.Compose, in.Now, stoppedFor); ok {
			out = append(out, l)
		}
	}
	for _, v := range in.Volumes {
		if l, ok := classifyVolume(v, in.Compose, mountedBy[v.Name], projectHasContainer); ok {
			out = append(out, l)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := confidenceRank(out[i].Confidence), confidenceRank(out[j].Confidence)
		if ri != rj {
			return ri < rj
		}
		if out[i].SizeMB != out[j].SizeMB {
			return out[i].SizeMB > out[j].SizeMB
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func confidenceRank(c LeftoverConfidence) int {
	switch c {
	case ConfidenceCertain:
		return 0
	case ConfidenceLikely:
		return 1
	}
	return 2
}

func isRunningState(state string) bool {
	switch state {
	case "running", "paused", "restarting":
		return true
	}
	return false
}

func classifyContainer(c LeftoverContainer, scope *ComposeScope, now time.Time, stoppedFor time.Duration) (Leftover, bool) {
	project := c.Labels[labelComposeProject]
	service := c.Labels[labelComposeService]
	running := isRunningState(c.State)
	base := Leftover{
		ID:        c.ID,
		Name:      c.Name,
		Project:   project,
		Service:   service,
		SizeMB:    c.SizeRwMB,
		SizeKnown: c.SizeKnown,
	}

	// One-off containers from `compose run` without --rm. A running one is
	// someone's interactive session, not a leftover.
	if strings.EqualFold(c.Labels[labelComposeOneOff], "true") {
		if running {
			return Leftover{}, false
		}
		base.Kind = LeftoverComposeOneOff
		base.Confidence = ConfidenceCertain
		base.Reason = "`compose run` container left behind (run without --rm)"
		base.Action = "remove it; use `docker compose run --rm` next time"
		base.Removable = true
		return base, true
	}

	// Orphans: the service is no longer in the loaded compose file. In the
	// real-daemon run they kept running, so most are guidance, not removal.
	if scope != nil && project != "" && project == scope.Project && service != "" && !scope.Services[service] {
		base.Kind = LeftoverComposeOrphan
		base.Confidence = ConfidenceCertain
		base.Reason = fmt.Sprintf("service %q is no longer in the compose file", service)
		if running {
			base.Action = "still running; stop it or run `docker compose up --remove-orphans`"
		} else {
			base.Action = "remove it or run `docker compose up --remove-orphans`"
			base.Removable = true
		}
		return base, true
	}

	// Long-stopped containers. The stop time only comes from inspect, so a
	// zero FinishedAt (unknown, or created but never started) is skipped.
	if (c.State == "exited" || c.State == "dead") && !c.FinishedAt.IsZero() {
		if age := now.Sub(c.FinishedAt); age >= stoppedFor {
			base.Kind = LeftoverStoppedContainer
			base.Confidence = ConfidenceLikely
			base.Reason = fmt.Sprintf("stopped for %d days", int(age.Hours()/24))
			base.Action = "remove it if it is not restarted anymore; use `docker run --rm` for one-shot runs"
			base.Removable = true
			return base, true
		}
	}
	return Leftover{}, false
}

func classifyVolume(v LeftoverVolume, scope *ComposeScope, heldBy []string, projectHasContainer map[string]bool) (Leftover, bool) {
	attached := v.RefCount > 0 || len(heldBy) > 0
	project := v.Labels[labelComposeProject]
	base := Leftover{
		ID:        v.Name,
		Name:      v.Name,
		Project:   project,
		SizeMB:    v.SizeMB,
		SizeKnown: v.SizeKnown,
		HeldBy:    heldBy,
	}

	// A docker-container buildx builder keeps its cache in its own volume.
	// Its buildkit container holds it, so it never looks reclaimable as a
	// volume, and `docker builder prune` does not touch it.
	if node, ok := buildxStateNode(v.Name); ok {
		builder := buildxBuilderName(node)
		base.Kind = LeftoverBuilderCache
		base.Confidence = ConfidenceReport
		base.Name = builder
		base.Reason = fmt.Sprintf("build cache of buildx builder %q, outside Build Cache and `docker builder prune`", builder)
		base.Action = fmt.Sprintf("`docker buildx prune --builder %s`, or `docker buildx rm %s` if the builder is unused", builder, builder)
		return base, true
	}

	if _, ok := v.Labels[labelAnonymousVol]; ok {
		if attached {
			return Leftover{}, false
		}
		base.Kind = LeftoverAnonymousVolume
		base.Confidence = ConfidenceCertain
		base.Reason = "anonymous volume no container uses (container removed without -v)"
		base.Action = "remove it; use `docker rm -v` or `docker run --rm` next time"
		base.Removable = true
		return base, true
	}

	// Plain named volumes without compose labels were created on purpose;
	// the existing Local Volumes category covers them.
	if project == "" {
		return Leftover{}, false
	}
	key := v.Labels[labelComposeVolume]

	if scope != nil && project == scope.Project {
		if key == "" || scope.Volumes[key] {
			// Still defined by the current project: data kept on purpose.
			return Leftover{}, false
		}
		base.Kind = LeftoverDroppedVolume
		base.Confidence = ConfidenceCertain
		base.Reason = fmt.Sprintf("volume %q is no longer in the compose file", key)
		if attached {
			base.Action = "still used; remove the container(s) holding it first (often a compose orphan)"
		} else {
			base.Action = "remove it if its data is no longer needed"
			base.Removable = true
		}
		return base, true
	}

	if !projectHasContainer[project] && !attached {
		base.Kind = LeftoverOwnerlessVolume
		base.Confidence = ConfidenceLikely
		base.Reason = fmt.Sprintf("no container of compose project %q exists (removed, or `compose down` without -v)", project)
		base.Action = "remove it if the project is gone; keep it if you bring the project back up"
		base.Removable = true
		return base, true
	}
	return Leftover{}, false
}

// buildxStateNode extracts the node name from a buildx state volume name,
// "buildx_buildkit_<node>_state".
func buildxStateNode(name string) (string, bool) {
	const prefix, suffix = "buildx_buildkit_", "_state"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) || len(name) <= len(prefix)+len(suffix) {
		return "", false
	}
	return name[len(prefix) : len(name)-len(suffix)], true
}

// buildxBuilderName strips the node index buildx appends to the builder
// name ("mybuilder0" -> "mybuilder"). Builders with ten or more nodes would
// keep one stray digit; that is rare enough to accept for a hint.
func buildxBuilderName(node string) string {
	if n := len(node); n > 1 && node[n-1] >= '0' && node[n-1] <= '9' {
		return node[:n-1]
	}
	return node
}

// ---------------------------------------------------------------------------
// SDK conversion

// leftoverContainersFromSDK converts /system/df containers. finishedAt maps
// container ID to inspect's State.FinishedAt for stopped containers.
func leftoverContainersFromSDK(cs []*container.Summary, finishedAt map[string]time.Time) []LeftoverContainer {
	out := make([]LeftoverContainer, 0, len(cs))
	for _, c := range cs {
		if c == nil {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			name = shortID(c.ID)
		}
		var vols []string
		for _, m := range c.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" {
				vols = append(vols, m.Name)
			}
		}
		out = append(out, LeftoverContainer{
			ID:         c.ID,
			Name:       name,
			State:      c.State,
			Labels:     c.Labels,
			FinishedAt: finishedAt[c.ID],
			SizeRwMB:   bytesToMB(c.SizeRw),
			SizeKnown:  c.SizeRw >= 0,
			Volumes:    vols,
		})
	}
	return out
}

// leftoverVolumesFromSDK converts /system/df volumes.
func leftoverVolumesFromSDK(vs []*volume.Volume) []LeftoverVolume {
	out := make([]LeftoverVolume, 0, len(vs))
	for _, v := range vs {
		if v == nil {
			continue
		}
		lv := LeftoverVolume{Name: v.Name, Labels: v.Labels, RefCount: -1}
		if v.UsageData != nil {
			lv.RefCount = v.UsageData.RefCount
			if v.UsageData.Size >= 0 {
				lv.SizeMB = bytesToMB(v.UsageData.Size)
				lv.SizeKnown = true
			}
		}
		out = append(out, lv)
	}
	return out
}

// parseFinishedAt parses inspect's State.FinishedAt; the zero value Docker
// reports for never-stopped containers ("0001-01-01T00:00:00Z") stays zero.
func parseFinishedAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.Year() <= 1 {
		return time.Time{}
	}
	return t
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ---------------------------------------------------------------------------
// live client

// ListLeftovers detects leftover resources on the daemon. scope is the
// loaded compose project, or nil when no compose file is loaded.
func (c *Client) ListLeftovers(scope *ComposeScope) ([]Leftover, error) {
	du, err := c.cli.DiskUsage(c.ctx, types.DiskUsageOptions{
		Types: []types.DiskUsageObject{types.ContainerObject, types.VolumeObject},
	})
	if err != nil {
		return nil, err
	}

	// The stop time is only in inspect; inspect stopped containers only.
	finished := map[string]time.Time{}
	for _, ctr := range du.Containers {
		if ctr == nil || (ctr.State != "exited" && ctr.State != "dead") {
			continue
		}
		if t, ok := c.containerFinishedAt(ctr.ID); ok {
			finished[ctr.ID] = t
		}
	}

	return DetectLeftovers(LeftoverInput{
		Containers: leftoverContainersFromSDK(du.Containers, finished),
		Volumes:    leftoverVolumesFromSDK(du.Volumes),
		Compose:    scope,
		Now:        time.Now(),
	}), nil
}

func (c *Client) containerFinishedAt(id string) (time.Time, bool) {
	ctx, cancel := context.WithTimeout(c.ctx, inspectTimeout)
	defer cancel()
	insp, err := c.cli.ContainerInspect(ctx, id)
	if err != nil || insp.State == nil {
		return time.Time{}, false
	}
	t := parseFinishedAt(insp.State.FinishedAt)
	return t, !t.IsZero()
}
