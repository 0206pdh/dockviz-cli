package docker

import (
	"testing"

	"github.com/docker/docker/api/types/image"
)

func TestIsDangling(t *testing.T) {
	tests := []struct {
		name string
		img  *image.Summary
		want bool
	}{
		{"no tags", &image.Summary{RepoTags: nil}, true},
		{"single none tag", &image.Summary{RepoTags: []string{"<none>:<none>"}}, true},
		{"tagged image", &image.Summary{RepoTags: []string{"nginx:latest"}}, false},
		{"multiple tags", &image.Summary{RepoTags: []string{"nginx:latest", "nginx:1.25"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDangling(tt.img); got != tt.want {
				t.Errorf("isDangling() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		name string
		b    int64
		want float64
	}{
		{"zero", 0, 0},
		{"one MB", 1024 * 1024, 1},
		{"half MB", 512 * 1024, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesToMB(tt.b); got != tt.want {
				t.Errorf("bytesToMB(%d) = %v, want %v", tt.b, got, tt.want)
			}
		})
	}
}

func TestDemoClientPruneShrinksDiskUsage(t *testing.T) {
	d := NewDemoClient()

	before, err := d.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage() error = %v", err)
	}
	if before.Images.ReclaimMB == 0 {
		t.Fatal("expected non-zero reclaimable images before pruning")
	}

	if _, err := d.PruneImages(); err != nil {
		t.Fatalf("PruneImages() error = %v", err)
	}

	after, err := d.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage() error = %v", err)
	}
	if after.Images.ReclaimMB != 0 {
		t.Errorf("Images.ReclaimMB after prune = %v, want 0", after.Images.ReclaimMB)
	}
	// Pruning images must not affect other categories.
	if after.BuildCache.ReclaimMB != before.BuildCache.ReclaimMB {
		t.Errorf("BuildCache.ReclaimMB changed after pruning images: before=%v after=%v",
			before.BuildCache.ReclaimMB, after.BuildCache.ReclaimMB)
	}
}
