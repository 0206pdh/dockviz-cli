package tui

import (
	"testing"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		v    int
		lo   int
		hi   int
		want int
	}{
		{"within range", 5, 0, 10, 5},
		{"below lo returns lo", -1, 0, 10, 0},
		{"above hi returns hi", 15, 0, 10, 10},
		{"equal lo", 0, 0, 10, 0},
		{"equal hi", 10, 0, 10, 10},
		{"hi less than lo returns lo", 5, 5, 3, 5},
		{"zero range", 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp(tt.v, tt.lo, tt.hi)
			if got != tt.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}

func TestDiskUsageRows(t *testing.T) {
	du := docker.DiskUsageInfo{
		Images:     docker.DiskUsageCategory{Total: 9, ReclaimMB: 410},
		Containers: docker.DiskUsageCategory{Total: 6, ReclaimMB: 18},
		Volumes:    docker.DiskUsageCategory{Total: 4, ReclaimMB: 96},
		BuildCache: docker.DiskUsageCategory{Total: 37, ReclaimMB: 2560},
	}

	rows := diskUsageRows(du)
	if len(rows) != 4 {
		t.Fatalf("diskUsageRows() returned %d rows, want 4", len(rows))
	}

	wantKeys := []string{"images", "containers", "volumes", "buildcache"}
	for i, want := range wantKeys {
		if rows[i].key != want {
			t.Errorf("rows[%d].key = %q, want %q", i, rows[i].key, want)
		}
		if rows[i].warning == "" {
			t.Errorf("rows[%d] (%s) has empty warning text", i, rows[i].key)
		}
	}

	if rows[0].cat.Total != 9 || rows[0].cat.ReclaimMB != 410 {
		t.Errorf("rows[0].cat = %+v, want the Images category from du", rows[0].cat)
	}
}

func TestCategoryLabel(t *testing.T) {
	if got := categoryLabel("buildcache"); got != "Build Cache" {
		t.Errorf("categoryLabel(%q) = %q, want %q", "buildcache", got, "Build Cache")
	}
	if got := categoryLabel("unknown"); got != "unknown" {
		t.Errorf("categoryLabel(%q) = %q, want the key itself as fallback", "unknown", got)
	}
}
