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
		Logs:       docker.DiskUsageCategory{Total: 6, ReclaimMB: 734},
	}

	rows := diskUsageRows(du)
	if len(rows) != 5 {
		t.Fatalf("diskUsageRows() returned %d rows, want 5", len(rows))
	}

	wantKeys := []string{"images", "containers", "volumes", "buildcache", "logs"}
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

func newTestModel() Model {
	return Model{
		docker:     docker.NewDemoClient(),
		history:    make(map[string][]float64),
		memHistory: make(map[string][]float64),
	}
}

func TestDataMsgDropsStaleResults(t *testing.T) {
	m := newTestModel()
	m.fetchSeq = 2

	newer := dataMsg{seq: 2, containers: []docker.ContainerInfo{
		{ID: "a", Status: "running", CPUPerc: 50, MemMB: 200, StatsOK: true},
	}}
	older := dataMsg{seq: 1, containers: []docker.ContainerInfo{
		{ID: "a", Status: "running", CPUPerc: 10, MemMB: 100, StatsOK: true},
	}}

	next, _ := m.Update(newer)
	m = next.(Model)
	next, _ = m.Update(older)
	m = next.(Model)

	if got := m.containers[0].CPUPerc; got != 50 {
		t.Errorf("stale result overwrote newer data: CPUPerc = %v, want 50", got)
	}
	if h := m.history["a"]; len(h) != 1 || h[0] != 50 {
		t.Errorf("history = %v, want [50] (stale sample must not be appended)", h)
	}
	if m.fetchInFlight() {
		t.Error("fetchInFlight() = true after the latest fetch was applied")
	}
}

func TestDataMsgSkipsFailedStatsSamples(t *testing.T) {
	m := newTestModel()
	m.fetchSeq = 1

	next, _ := m.Update(dataMsg{seq: 1, containers: []docker.ContainerInfo{
		{ID: "ok", Status: "running", CPUPerc: 30, MemMB: 64, StatsOK: true},
		{ID: "failed", Status: "running"}, // stats call failed: zero values, StatsOK=false
	}})
	m = next.(Model)

	if h := m.history["ok"]; len(h) != 1 {
		t.Errorf("history[ok] = %v, want one sample", h)
	}
	if h, ok := m.history["failed"]; ok {
		t.Errorf("history[failed] = %v, want no entry for a failed sample", h)
	}
	if h, ok := m.memHistory["failed"]; ok {
		t.Errorf("memHistory[failed] = %v, want no entry for a failed sample", h)
	}
}

func TestTickSkipsFetchWhileInFlight(t *testing.T) {
	m := newTestModel()
	m.fetchSeq = 3
	m.appliedSeq = 2 // fetch #3 still running

	next, cmd := m.Update(tickMsg{})
	m = next.(Model)
	if m.fetchSeq != 3 {
		t.Errorf("fetchSeq = %d, want 3 (tick must not issue a new fetch while one is in flight)", m.fetchSeq)
	}
	if cmd == nil {
		t.Error("tick returned nil cmd, want the next tick to be scheduled")
	}

	m.appliedSeq = 3
	next, _ = m.Update(tickMsg{})
	m = next.(Model)
	if m.fetchSeq != 4 {
		t.Errorf("fetchSeq = %d, want 4 once the previous fetch completed", m.fetchSeq)
	}
}
