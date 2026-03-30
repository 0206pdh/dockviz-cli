// images.go fetches the list of local Docker images.
package docker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/image"
)

// ImageInfo holds display data for one Docker image row (one tag per row).
type ImageInfo struct {
	ID      string
	Tag     string   // 이 행의 단일 태그, 또는 "<none>"
	AllTags []string // 이 image ID의 모든 태그 (삭제 경고 표시용)
	SizeMB  float64
}

// ListImages returns all locally available Docker images, one row per tag.
func (c *Client) ListImages() ([]ImageInfo, error) {
	images, err := c.cli.ImageList(c.ctx, image.ListOptions{All: false})
	if err != nil {
		return nil, err
	}

	var result []ImageInfo
	for _, img := range images {
		id := img.ID
		if strings.HasPrefix(id, "sha256:") {
			id = id[7:19]
		}
		size := float64(img.Size) / 1024 / 1024
		tags := img.RepoTags
		if len(tags) == 0 {
			result = append(result, ImageInfo{ID: id, Tag: "<none>", SizeMB: size})
			continue
		}
		for _, tag := range tags {
			result = append(result, ImageInfo{ID: id, Tag: tag, AllTags: tags, SizeMB: size})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Tag < result[j].Tag
	})
	return result, nil
}

// RemoveImage removes a local image by tag name (safe default: only removes that tag).
func (c *Client) RemoveImage(id string) error {
	_, err := c.cli.ImageRemove(c.ctx, id, image.RemoveOptions{Force: false})
	return err
}

// FormatSize returns a human-readable size string (MB or GB).
func FormatSize(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", mb/1024)
	}
	return fmt.Sprintf("%.0f MB", mb)
}
