package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestRenderDiskUsageDistinguishesZeroUnknownAndOutsidePrune(t *testing.T) {
	m := Model{
		activePanel:     PanelDiskUsage,
		diskUsageLoaded: true,
		diskUsage: docker.DiskUsageInfo{
			Images: docker.DiskUsageCategory{
				Total:          1,
				SizeMB:         128,
				OutsidePruneMB: 128,
			},
			BuildCache: docker.DiskUsageCategory{Total: 0},
			Logs: docker.DiskUsageCategory{
				Total:       0,
				Unknown:     1,
				Unavailable: "log path unavailable",
			},
		},
	}

	view := m.renderDiskUsage()
	for _, want := range []string{"0B", "N/A", "unused tagged space outside selected prune", "log path unavailable"} {
		if !strings.Contains(view, want) {
			t.Errorf("renderDiskUsage() missing %q in:\n%s", want, view)
		}
	}
}

func TestDiskUsageRefreshErrorStaysInPanel(t *testing.T) {
	m := Model{activePanel: PanelDiskUsage, diskUsageLoaded: true}
	updated, _ := m.Update(diskUsageMsg{err: errors.New("daemon unavailable")})
	got := updated.(Model)

	if got.err != nil {
		t.Fatalf("disk usage error leaked into global dashboard error: %v", got.err)
	}
	if got.diskUsageErr == nil || got.diskUsageErr.Error() != "daemon unavailable" {
		t.Fatalf("diskUsageErr = %v, want daemon unavailable", got.diskUsageErr)
	}
	if !strings.Contains(got.renderDiskUsage(), "daemon unavailable") {
		t.Fatal("renderDiskUsage() did not show the refresh error")
	}
}

func TestRenderDiskUsageShowsHostStorageSeparately(t *testing.T) {
	m := Model{
		activePanel:     PanelDiskUsage,
		diskUsageLoaded: true,
		diskUsage: docker.DiskUsageInfo{
			Images: docker.DiskUsageCategory{SizeMB: 460},
			HostStorage: docker.HostStorageInfo{
				Label:       "Docker Desktop VHDX",
				Path:        `C:\Users\me\AppData\Local\Docker\wsl\disk\docker_data.vhdx`,
				AllocatedMB: 19920,
				HostFreeMB:  28030,
				Available:   true,
			},
		},
	}

	view := m.renderDiskUsage()
	for _, want := range []string{"HOST STORAGE", "Docker Desktop VHDX", "19.5 GB", "host free 27.4 GB", "outside Docker df: 19.0 GB", "VHDX compaction"} {
		if !strings.Contains(view, want) {
			t.Errorf("renderDiskUsage() missing %q in:\n%s", want, view)
		}
	}

	if got := m.activeListLen(); got != 5 {
		t.Fatalf("activeListLen() = %d, want 5 prunable rows only", got)
	}
}

func TestHostStorageOutsideDockerDF(t *testing.T) {
	du := docker.DiskUsageInfo{
		Images:     docker.DiskUsageCategory{SizeMB: 100},
		Containers: docker.DiskUsageCategory{SizeMB: 50},
		HostStorage: docker.HostStorageInfo{
			AllocatedMB: 200,
			Available:   true,
		},
	}
	if got := hostStorageOutsideDockerDF(du); got != 50 {
		t.Fatalf("hostStorageOutsideDockerDF() = %v, want 50", got)
	}

	du.HostStorage.AllocatedMB = 100
	if got := hostStorageOutsideDockerDF(du); got != 0 {
		t.Fatalf("hostStorageOutsideDockerDF() = %v, want 0 when Docker df accounts for more than host allocation", got)
	}
}

func TestRenderContainersShowsResourceSummaries(t *testing.T) {
	m := Model{
		containers: []docker.ContainerInfo{
			{
				ID:             "abc123",
				Name:           "api",
				Status:         "running",
				CPUPerc:        40,
				MemMB:          512,
				CPULimit:       1.5,
				MemoryLimitMB:  2048,
				LimitsKnown:    true,
				ComposeProject: "shop",
				ComposeService: "api",
			},
		},
		history:    map[string][]float64{"abc123": []float64{10, 20, 40}},
		memHistory: map[string][]float64{"abc123": []float64{256, 512, 1024}},
	}

	view := m.renderContainers()
	for _, want := range []string{"PROJECT RESOURCE SUMMARY", "shop", "CPU95", "MEM95", "LIMITS", "40.0%", "1.0GB", "CPU:1.5 MEM:2.0GB"} {
		if !strings.Contains(view, want) {
			t.Fatalf("renderContainers() missing %q in:\n%s", want, view)
		}
	}
}

func TestRenderDetailShowsResourceSummary(t *testing.T) {
	m := Model{
		selectedID: "abc123",
		containers: []docker.ContainerInfo{
			{
				ID:             "abc123",
				Name:           "api",
				Image:          "my-api:latest",
				Status:         "running",
				CPUPerc:        40,
				MemMB:          512,
				CPULimit:       1,
				MemoryLimitMB:  1024,
				LimitsKnown:    true,
				ComposeProject: "shop",
				ComposeService: "api",
				Ports:          "8080:80",
			},
		},
		history:    map[string][]float64{"abc123": []float64{10, 20, 40}},
		memHistory: map[string][]float64{"abc123": []float64{256, 512, 768}},
	}

	view := m.renderDetail()
	for _, want := range []string{"Project", "shop", "Service", "api", "CPU", "Memory", "avg", "p95", "peak", "trend", "Limits"} {
		if !strings.Contains(view, want) {
			t.Fatalf("renderDetail() missing %q in:\n%s", want, view)
		}
	}
}
