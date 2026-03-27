// images.go fetches the list of local Docker images.
package docker

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/image"
)

// ImageInfo holds display data for one Docker image.
type ImageInfo struct {
	ID   string
	Tags string  // comma-separated repo:tag
	SizeMB float64
}

// ListImages returns all locally available Docker images.
func (c *Client) ListImages() ([]ImageInfo, error) {
	images, err := c.cli.ImageList(c.ctx, image.ListOptions{All: false})
	if err != nil {
		return nil, err
	}

	result := make([]ImageInfo, 0, len(images))
	for _, img := range images {
		tags := strings.Join(img.RepoTags, ", ")
		if tags == "" {
			tags = "<none>"
		}
		id := img.ID
		if strings.HasPrefix(id, "sha256:") {
			id = id[7:19] // short ID
		}
		result = append(result, ImageInfo{
			ID:     id,
			Tags:   tags,
			SizeMB: float64(img.Size) / 1024 / 1024,
		})
	}
	return result, nil
}

// RemoveImage removes a local image (force, to handle tagged images).
func (c *Client) RemoveImage(id string) error {
	_, err := c.cli.ImageRemove(c.ctx, id, image.RemoveOptions{Force: true})
	return err
}

// FormatSize returns a human-readable size string (MB or GB).
func FormatSize(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", mb/1024)
	}
	return fmt.Sprintf("%.0f MB", mb)
}
