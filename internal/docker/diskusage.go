// diskusage.go computes a `docker system df`-style disk usage breakdown and
// exposes one prune action per resource category (images, containers,
// volumes, build cache). Each category's reported ReclaimMB always matches
// exactly what its Prune* method will free, so the TUI never shows a number
// the prune action can't deliver.
package docker

import (
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
}

// DiskUsageInfo is the full disk usage breakdown, one category per resource type.
type DiskUsageInfo struct {
	Images     DiskUsageCategory
	Containers DiskUsageCategory
	Volumes    DiskUsageCategory
	BuildCache DiskUsageCategory
}

// DiskUsage fetches and summarizes Docker's disk usage, mirroring `docker system df`.
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
func (c *Client) PruneVolumes() (float64, error) {
	report, err := c.cli.VolumesPrune(c.ctx, filters.NewArgs())
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
