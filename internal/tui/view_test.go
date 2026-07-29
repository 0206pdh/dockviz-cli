package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestRenderDiskUsageDistinguishesZeroUnknownAndOutsidePrune(t *testing.T) {
	m := Model{
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
