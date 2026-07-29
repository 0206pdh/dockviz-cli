// diskusage.go computes a `docker system df`-style disk usage breakdown and
// exposes one prune action per resource category (images, containers,
// volumes, build cache, container logs). Each category's reported ReclaimMB
// always matches exactly what its Prune* method will free, so the TUI never
// shows a number the prune action can't deliver.
package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// DiskUsageCategory holds the disk usage breakdown for one resource type.
type DiskUsageCategory struct {
	Label     string  // display name, e.g. "Images"
	Total     int     // total object count
	Active    int     // objects currently in use
	SizeMB    float64 // total size on disk
	ReclaimMB float64 // size Prune would free for this category
	// Unknown is the number of objects whose size or reclaimability could not
	// be measured. It is deliberately separate from zero: zero means the
	// daemon measured no bytes, while Unknown means the answer is unavailable.
	Unknown int
	// OutsidePruneMB is known unused space that the selected safe prune action
	// intentionally does not remove. It is currently used for tagged images.
	OutsidePruneMB float64
	// Unavailable, when non-empty, explains why this category couldn't be
	// fully measured (e.g. filesystem permission denied) so the TUI can
	// show that instead of a misleadingly clean 0.
	Unavailable string
}

// DiskUsageInfo is the full disk usage breakdown, one category per resource type.
type DiskUsageInfo struct {
	Images     DiskUsageCategory
	Containers DiskUsageCategory
	Volumes    DiskUsageCategory
	BuildCache DiskUsageCategory
	// Logs is container stdout/stderr log files on disk. `docker system df`
	// never counts these — its container size only covers the RW layer —
	// so an unrotated json-file log is a common source of disk usage that
	// stays invisible until the disk fills up.
	Logs DiskUsageCategory
}

// DiskUsage fetches and summarizes Docker's disk usage, mirroring `docker system df`
// plus a Logs category that `docker system df` doesn't account for at all.
func (c *Client) DiskUsage() (DiskUsageInfo, error) {
	du, err := c.cli.DiskUsage(c.ctx, types.DiskUsageOptions{})
	if err != nil {
		return DiskUsageInfo{}, err
	}

	var info DiskUsageInfo

	info.Images.Label = "Images"
	for _, img := range du.Images {
		info.Images.Total++
		info.Images.SizeMB += bytesToMB(img.Size)
		if img.Containers > 0 {
			info.Images.Active++
		}
		if isDangling(img) {
			info.Images.ReclaimMB += bytesToMB(img.Size)
		} else if img.Containers == 0 {
			// The selected prune action is intentionally dangling-only. Keep
			// tagged images outside that action, but tell the operator that the
			// space is unused and can be removed from the Images panel.
			info.Images.OutsidePruneMB += bytesToMB(img.Size)
		}
	}

	info.Containers.Label = "Containers"
	for _, ctr := range du.Containers {
		info.Containers.Total++
		info.Containers.SizeMB += bytesToMB(ctr.SizeRw)
		// "running" and "paused" containers are left alone by ContainersPrune;
		// only "created"/"exited"/"dead" containers are actually reclaimable.
		if ctr.State == "running" || ctr.State == "paused" {
			info.Containers.Active++
		} else {
			info.Containers.ReclaimMB += bytesToMB(ctr.SizeRw)
		}
	}

	info.Volumes.Label = "Local Volumes"
	unknownVolumeCount := 0
	for _, v := range du.Volumes {
		info.Volumes.Total++
		var size, refCount int64
		if v.UsageData == nil {
			unknownVolumeCount++
			continue
		}
		size = v.UsageData.Size
		refCount = v.UsageData.RefCount
		info.Volumes.SizeMB += bytesToMB(size)
		if refCount > 0 {
			info.Volumes.Active++
		} else {
			info.Volumes.ReclaimMB += bytesToMB(size)
		}
	}
	if unknownVolumeCount > 0 {
		info.Volumes.Unknown = unknownVolumeCount
		info.Volumes.Unavailable = fmt.Sprintf("size unavailable for %d volume(s) — daemon returned no UsageData", unknownVolumeCount)
	}

	info.BuildCache.Label = "Build Cache"
	for _, bc := range du.BuildCache {
		info.BuildCache.Total++
		info.BuildCache.SizeMB += bytesToMB(bc.Size)
		if bc.InUse {
			info.BuildCache.Active++
		} else {
			info.BuildCache.ReclaimMB += bytesToMB(bc.Size)
		}
	}

	info.Logs.Label = "Container Logs"
	unknownLogCount := 0
	for _, ctr := range du.Containers {
		inspect, err := c.cli.ContainerInspect(c.ctx, ctr.ID)
		if err != nil {
			unknownLogCount++
			continue
		}
		activeMB, rotated, unavailable := logFileSizes(inspect.LogPath)
		if unavailable {
			unknownLogCount++
			continue
		}
		size := activeMB
		for _, s := range rotated {
			size += s
		}
		if size == 0 {
			continue
		}
		info.Logs.Total++
		info.Logs.SizeMB += size
		// Truncating a log file never touches the container, so every byte
		// found is reclaimable regardless of running state.
		info.Logs.ReclaimMB += size
		if ctr.State == "running" || ctr.State == "paused" {
			info.Logs.Active++
		}
	}
	// /var/lib/docker/containers is root-owned on a typical native-Linux
	// install; a `docker`-group user can talk to the daemon but can't read
	// those files directly. Say so explicitly instead of showing a clean 0,
	// which would look identical to "this container just has no logs".
	if unknownLogCount > 0 {
		info.Logs.Unknown = unknownLogCount
		info.Logs.Unavailable = fmt.Sprintf("log path unavailable for %d container(s) — Docker Desktop/remote daemon paths are not local", unknownLogCount)
	}

	return info, nil
}

// PruneImages removes dangling (untagged) images — the leftovers of repeated
// rebuilds. Tagged images still referenced by a name are never touched.
func (c *Client) PruneImages() (float64, error) {
	report, err := c.cli.ImagesPrune(c.ctx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return 0, err
	}
	return bytesToMB(int64(report.SpaceReclaimed)), nil
}

// PruneContainers removes stopped ("created"/"exited"/"dead") containers.
func (c *Client) PruneContainers() (float64, error) {
	report, err := c.cli.ContainersPrune(c.ctx, filters.NewArgs())
	if err != nil {
		return 0, err
	}
	return bytesToMB(int64(report.SpaceReclaimed)), nil
}

// PruneVolumes removes local volumes not attached to any container.
// The "all" filter is required because on API >=1.42 the daemon otherwise
// restricts pruning to anonymous volumes only, which would silently leave
// named-but-unattached volumes on disk despite DiskUsage reporting them
// as reclaimable.
func (c *Client) PruneVolumes() (float64, error) {
	report, err := c.cli.VolumesPrune(c.ctx, filters.NewArgs(filters.Arg("all", "true")))
	if err != nil {
		return 0, err
	}
	return bytesToMB(int64(report.SpaceReclaimed)), nil
}

// PruneBuildCache removes build cache layers not in use by a running build.
func (c *Client) PruneBuildCache() (float64, error) {
	report, err := c.cli.BuildCachePrune(c.ctx, types.BuildCachePruneOptions{All: true})
	if err != nil {
		return 0, err
	}
	if report == nil {
		return 0, nil
	}
	return bytesToMB(int64(report.SpaceReclaimed)), nil
}

// PruneLogs truncates every container's active log file to zero bytes and
// removes any rotated siblings (<path>.1, <path>.2, ...). The active file is
// truncated in place rather than removed: the daemon holds it open for the
// life of the container, and unlinking it would leave the space allocated
// until the daemon next reopens it. This never touches the container
// itself — a running container keeps writing to the same (now-empty) file.
func (c *Client) PruneLogs() (float64, error) {
	du, err := c.cli.DiskUsage(c.ctx, types.DiskUsageOptions{})
	if err != nil {
		return 0, err
	}

	var freedMB float64
	unavailable := 0
	failed := 0
	for _, ctr := range du.Containers {
		inspect, err := c.cli.ContainerInspect(c.ctx, ctr.ID)
		if err != nil {
			unavailable++
			continue
		}
		activeMB, rotated, pathUnavailable := logFileSizes(inspect.LogPath)
		if pathUnavailable {
			unavailable++
			continue
		}
		if inspect.LogPath != "" && activeMB > 0 {
			if err := os.Truncate(inspect.LogPath, 0); err == nil {
				freedMB += activeMB
			} else {
				failed++
			}
		}
		for path, size := range rotated {
			if err := os.Remove(path); err == nil {
				freedMB += size
			} else {
				failed++
			}
		}
	}
	if unavailable > 0 {
		return freedMB, fmt.Errorf("container log paths unavailable for %d container(s); Docker Desktop or remote daemon logs must be cleaned on the daemon host", unavailable)
	}
	if failed > 0 {
		return freedMB, fmt.Errorf("failed to remove or truncate %d container log file(s)", failed)
	}
	return freedMB, nil
}

// isDangling reports whether an image is untagged — the classic leftover
// from a rebuild that reused the same tag.
func isDangling(img *image.Summary) bool {
	if len(img.RepoTags) == 0 {
		return true
	}
	return len(img.RepoTags) == 1 && img.RepoTags[0] == "<none>:<none>"
}

func bytesToMB(b int64) float64 {
	return float64(b) / 1024 / 1024
}

// fileSizeMB returns a file's size in MB, or 0 if it can't be stat'd (e.g.
// it doesn't exist, or LogPath points at a path this process can't reach —
// which is always the case against a remote DOCKER_HOST).
func fileSizeMB(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return bytesToMB(fi.Size())
}

// logFileSizes stat's a container's active log file plus any rotated
// siblings matching "<logPath>*" (Docker's json-file driver rotates as
// <logPath>.1, <logPath>.2, ..., optionally gzip-compressed). It returns the
// active file's size separately from the rotated ones because the two must
// be reclaimed differently: the active file is truncated in place while
// rotated files are removed outright (see PruneLogs).
//
// unavailable reports whether the active log path is not reachable from this
// process. Docker Desktop and remote daemons commonly return a path that only
// exists inside the daemon's VM/host, so treating an os.Stat failure as 0B
// would hide real log usage.
func logFileSizes(logPath string) (activeMB float64, rotatedMB map[string]float64, permDenied bool) {
	rotatedMB = map[string]float64{}
	if logPath == "" {
		return 0, rotatedMB, false
	}

	fi, err := os.Stat(logPath)
	if err != nil {
		// A non-empty LogPath that cannot be stat'ed is not evidence of a
		// zero-byte log. It is usually a Docker Desktop VM or remote-daemon
		// path that this process cannot reach.
		return 0, rotatedMB, true
	}
	activeMB = bytesToMB(fi.Size())

	matches, err := filepath.Glob(logPath + "*")
	if err != nil {
		return activeMB, rotatedMB, false
	}
	for _, m := range matches {
		if m == logPath {
			continue
		}
		rotatedMB[m] = fileSizeMB(m)
	}
	return activeMB, rotatedMB, false
}
