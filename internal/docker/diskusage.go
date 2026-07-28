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
	for _, v := range du.Volumes {
		info.Volumes.Total++
		var size, refCount int64
		if v.UsageData != nil {
			size = v.UsageData.Size
			refCount = v.UsageData.RefCount
		}
		info.Volumes.SizeMB += bytesToMB(size)
		if refCount > 0 {
			info.Volumes.Active++
		} else {
			info.Volumes.ReclaimMB += bytesToMB(size)
		}
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
	deniedCount := 0
	for _, ctr := range du.Containers {
		inspect, err := c.cli.ContainerInspect(c.ctx, ctr.ID)
		if err != nil {
			continue
		}
		activeMB, rotated, denied := logFileSizes(inspect.LogPath)
		if denied {
			deniedCount++
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
	if deniedCount > 0 {
		info.Logs.Unavailable = fmt.Sprintf("permission denied on %d container(s) — try sudo", deniedCount)
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
	for _, ctr := range du.Containers {
		inspect, err := c.cli.ContainerInspect(c.ctx, ctr.ID)
		if err != nil {
			continue
		}
		activeMB, rotated, _ := logFileSizes(inspect.LogPath)
		if inspect.LogPath != "" && activeMB > 0 {
			if err := os.Truncate(inspect.LogPath, 0); err == nil {
				freedMB += activeMB
			}
		}
		for path, size := range rotated {
			if err := os.Remove(path); err == nil {
				freedMB += size
			}
		}
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
// permDenied reports whether the active log path exists but this process
// lacks permission to read it. It's checked with a direct Stat on logPath
// rather than inferred from the Glob results below: Glob silently returns
// no matches (not an error) when it can't list the containing directory, so
// relying on it would make "permission denied" indistinguishable from
// "no log file". permDenied is left false for any other reason the log is
// unreachable (no logPath, or it doesn't exist) since there's nothing
// actionable to tell the user in those cases.
func logFileSizes(logPath string) (activeMB float64, rotatedMB map[string]float64, permDenied bool) {
	rotatedMB = map[string]float64{}
	if logPath == "" {
		return 0, rotatedMB, false
	}

	fi, err := os.Stat(logPath)
	if os.IsPermission(err) {
		return 0, rotatedMB, true
	}
	if err != nil {
		return 0, rotatedMB, false
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
